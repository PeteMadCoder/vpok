package runtime

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// QEMURuntime implements the Runtime interface using QEMU / KVM
type QEMURuntime struct {
	binaryPath string
	workDir    string
	mu         sync.RWMutex
	instances  map[string]*QEMUInstance
}

// NewQEMURuntime initializes a QEMURuntime. If binaryPath is empty, it looks up qemu-system-x86_64.
func NewQEMURuntime(binaryPath, workDir string) (*QEMURuntime, error) {
	if binaryPath == "" {
		var err error
		binaryPath, err = exec.LookPath("qemu-system-x86_64")
		if err != nil {
			// Fallback to plain binary name if not found in path at initialization time
			binaryPath = "qemu-system-x86_64"
		}
	}

	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create runtime work directory %s: %w", workDir, err)
	}

	return &QEMURuntime{
		binaryPath: binaryPath,
		workDir:    workDir,
		instances:  make(map[string]*QEMUInstance),
	}, nil
}

// Create prepares and registers a new VM instance.
func (r *QEMURuntime) Create(ctx context.Context, cfg VMConfig) (Instance, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if cfg.ID == "" {
		return nil, fmt.Errorf("instance ID cannot be empty")
	}

	if _, exists := r.instances[cfg.ID]; exists {
		return nil, fmt.Errorf("instance %s already exists", cfg.ID)
	}

	instDir := filepath.Join(r.workDir, cfg.ID)
	if err := os.MkdirAll(instDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create instance dir: %w", err)
	}

	if cfg.LogFile == "" {
		cfg.LogFile = filepath.Join(instDir, "console.log")
	}

	inst := &QEMUInstance{
		config:     cfg,
		binaryPath: r.binaryPath,
		instDir:    instDir,
		state:      StateCreated,
	}

	r.instances[cfg.ID] = inst
	return inst, nil
}

// Get retrieves an instance by ID.
func (r *QEMURuntime) Get(ctx context.Context, id string) (Instance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	inst, exists := r.instances[id]
	if !exists {
		return nil, fmt.Errorf("instance %s not found", id)
	}
	return inst, nil
}

// List returns summaries for all tracked instances.
func (r *QEMURuntime) List(ctx context.Context) ([]InstanceSummary, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	summaries := make([]InstanceSummary, 0, len(r.instances))
	for _, inst := range r.instances {
		inst.mu.RLock()
		summaries = append(summaries, InstanceSummary{
			ID:        inst.config.ID,
			State:     inst.state,
			PID:       inst.pid,
			StartTime: inst.startTime,
			MemoryMB:  inst.config.MemoryMB,
			VCPUs:     inst.config.VCPUs,
		})
		inst.mu.RUnlock()
	}
	return summaries, nil
}

// Delete removes an instance from tracking.
func (r *QEMURuntime) Delete(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	inst, exists := r.instances[id]
	if !exists {
		return fmt.Errorf("instance %s not found", id)
	}

	inst.mu.RLock()
	running := (inst.state == StateRunning)
	inst.mu.RUnlock()

	if running {
		return fmt.Errorf("cannot delete running instance %s", id)
	}

	delete(r.instances, id)
	_ = os.RemoveAll(inst.instDir)
	return nil
}

// QUMUInstance manages a single QEMU VM process.
type QEMUInstance struct {
	config     VMConfig
	binaryPath string
	instDir    string
	cmd        *exec.Cmd
	pid        int
	state      State
	startTime  time.Time
	mu         sync.RWMutex
	waitChan   chan ExitStatus
}

func (inst *QEMUInstance) ID() string {
	return inst.config.ID
}

func (inst *QEMUInstance) State() State {
	inst.mu.RLock()
	defer inst.mu.RUnlock()
	return inst.state
}

func (inst *QEMUInstance) PID() int {
	inst.mu.RLock()
	defer inst.mu.RUnlock()
	return inst.pid
}

// BuildArgs constructs the QEMU command line flags.
func (inst *QEMUInstance) BuildArgs() ([]string, error) {
	cfg := inst.config
	args := []string{
		"-nodefaults",
		"-no-user-config",
		"-nographic",
		"-m", strconv.Itoa(cfg.MemoryMB),
		"-smp", strconv.Itoa(cfg.VCPUs),
	}

	// Check if KVM is available
	if _, err := os.Stat("/dev/kvm"); err == nil {
		args = append(args, "-enable-kvm", "-cpu", "host")
	} else {
		args = append(args, "-cpu", "max")
	}

	// Kernel and Initrd
	if cfg.KernelPath != "" {
		args = append(args, "-kernel", cfg.KernelPath)
	}
	if cfg.InitrdPath != "" {
		args = append(args, "-initrd", cfg.InitrdPath)
	}
	if cfg.Cmdline != "" {
		args = append(args, "-append", cfg.Cmdline)
	}

	// Root drive
	if cfg.RootDrive != nil {
		driveArg := fmt.Sprintf("file=%s,if=virtio,format=%s", cfg.RootDrive.Path, cfg.RootDrive.Format)
		if cfg.RootDrive.ReadOnly {
			driveArg += ",readonly=on"
		}
		args = append(args, "-drive", driveArg)
	}

	// Additional drives
	for _, d := range cfg.Drives {
		driveArg := fmt.Sprintf("file=%s,if=virtio,format=%s", d.Path, d.Format)
		if d.ReadOnly {
			driveArg += ",readonly=on"
		}
		args = append(args, "-drive", driveArg)
	}

	// Shared directories (virtio-9p)
	for i, sd := range cfg.SharedDirs {
		fsDevID := fmt.Sprintf("fsdev%d", i)
		readonlyOpt := "readonly=on"
		if !sd.ReadOnly {
			readonlyOpt = "readonly=off"
		}
		fsDevArg := fmt.Sprintf("local,security_model=none,id=%s,path=%s,%s", fsDevID, sd.HostPath, readonlyOpt)
		deviceArg := fmt.Sprintf("virtio-9p-pci,fsdev=%s,mount_tag=%s", fsDevID, sd.Tag)
		args = append(args, "-fsdev", fsDevArg, "-device", deviceArg)
	}

	// Serial console logging
	args = append(args, "-serial", fmt.Sprintf("file:%s", cfg.LogFile))

	return args, nil
}

// Start spawns the QEMU process.
func (inst *QEMUInstance) Start(ctx context.Context) error {
	inst.mu.Lock()
	defer inst.mu.Unlock()

	if inst.state == StateRunning {
		return fmt.Errorf("instance %s is already running", inst.config.ID)
	}

	args, err := inst.BuildArgs()
	if err != nil {
		return fmt.Errorf("failed to build QEMU args: %w", err)
	}

	cmd := exec.CommandContext(ctx, inst.binaryPath, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		inst.state = StateFailed
		return fmt.Errorf("failed to start QEMU: %w", err)
	}

	inst.cmd = cmd
	inst.pid = cmd.Process.Pid
	inst.state = StateRunning
	inst.startTime = time.Now()
	inst.waitChan = make(chan ExitStatus, 1)

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

		inst.mu.Lock()
		if exitCode == 0 {
			inst.state = StateStopped
		} else {
			inst.state = StateFailed
		}
		inst.mu.Unlock()

		inst.waitChan <- ExitStatus{
			ExitCode: exitCode,
			Err:      err,
		}
		close(inst.waitChan)
	}()

	return nil
}

// Stop initiates termination of the QEMU VM.
func (inst *QEMUInstance) Stop(ctx context.Context, gracePeriod time.Duration) error {
	inst.mu.RLock()
	if inst.state != StateRunning || inst.cmd == nil || inst.cmd.Process == nil {
		inst.mu.RUnlock()
		return nil
	}
	pid := inst.pid
	inst.mu.RUnlock()

	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		pgid = pid
	}

	// Send SIGTERM
	_ = syscall.Kill(-pgid, syscall.SIGTERM)

	timer := time.NewTimer(gracePeriod)
	defer timer.Stop()

	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			return ctx.Err()
		case <-timer.C:
			_ = syscall.Kill(-pgid, syscall.SIGKILL)
			return nil
		case <-ticker.C:
			if err := syscall.Kill(-pgid, 0); errors.Is(err, syscall.ESRCH) {
				return nil
			}
		}
	}
}

// Wait block until the instance stops
func (inst *QEMUInstance) Wait(ctx context.Context) (ExitStatus, error) {
	inst.mu.RLock()
	ch := inst.waitChan
	inst.mu.RUnlock()

	if ch == nil {
		return ExitStatus{}, fmt.Errorf("instance not started")
	}

	select {
	case <-ctx.Done():
		return ExitStatus{}, ctx.Err()
	case res := <-ch:
		return res, nil
	}
}

// Logs returns a stream of the console log file
func (inst *QEMUInstance) Logs() (io.ReadCloser, error) {
	inst.mu.RLock()
	logFile := inst.config.LogFile
	inst.mu.RUnlock()

	f, err := os.Open(logFile)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file %s: %w", logFile, err)
	}
	return f, nil
}
