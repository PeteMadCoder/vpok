package agent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// ProcessResult contains the exit details of the supervised workload.
type ProcessResult struct {
	ExitCode int
	Err      error
}

// Supervisor manages the child workload lifecycle.
type Supervisor struct {
	config      *Config
	cmd         *exec.Cmd
	stdout      io.Writer
	stderr      io.Writer
	gracePeriod time.Duration
}

// NewSupervisor creates a new Supervisor instance.
func NewSupervisor(cfg *Config, stdout, stderr io.Writer) *Supervisor {
	if stdout == nil {
		stdout = os.Stdout
	}
	if stderr == nil {
		stderr = os.Stderr
	}

	return &Supervisor{
		config:      cfg,
		stdout:      stdout,
		stderr:      stderr,
		gracePeriod: cfg.ParseGracePeriod(),
	}
}

// Start launches the child process asynchronously and returns a channel that emits
// the ProcessResult when the process terminates.
func (s *Supervisor) Start(ctx context.Context) (<-chan ProcessResult, error) {
	if len(s.config.Entrypoint) == 0 {
		return nil, fmt.Errorf("empty entrypoint command")
	}

	cmd := exec.CommandContext(ctx, s.config.Entrypoint[0], s.config.Entrypoint[1:]...)
	if s.config.WorkingDir != "" {
		cmd.Dir = s.config.WorkingDir
	}

	// Prepare environment: inherit host/guest environment plus custom overrides
	cmd.Env = os.Environ()
	for k, v := range s.config.Env {
		cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
	}

	cmd.Stdout = s.stdout
	cmd.Stderr = s.stderr

	// Put the process into its own process group so signals propagate to child forks
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start entrypoint: %w", err)
	}

	s.cmd = cmd
	done := make(chan ProcessResult, 1)

	go func() {
		err := cmd.Wait()
		exitCode := 0
		if err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = 1
			}
		}
		done <- ProcessResult{
			ExitCode: exitCode,
			Err:      err,
		}
		close(done)
	}()

	return done, nil
}

// Stop attempts a graceful shutdown by sending SIGTERM to the child's process group.
// If the process does not terminate within the configured grace period, it sends SIGKILL.
func (s *Supervisor) Stop() error {
	if s.cmd == nil || s.cmd.Process == nil {
		return nil
	}

	pid := s.cmd.Process.Pid

	// Send SIGTERM to the process group (negative PID)
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		pgid = pid
	}

	if err := syscall.Kill(-pgid, syscall.SIGTERM); err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
	}

	// Wait up to gracePeriod for process exit
	timer := time.NewTimer(s.gracePeriod)
	defer timer.Stop()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-timer.C:
			// Grace period expired, force kill
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			return nil
		case <-ticker.C:
			// Check if process has expired
			if err := syscall.Kill(-pgid, 0); errors.Is(err, syscall.ESRCH) {
				return nil
			}
		}
	}
}
