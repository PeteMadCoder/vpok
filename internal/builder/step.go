package builder

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/PeteMadCoder/vpok/internal/manifest"
)

// ExecuteStep runs an individual build step against the staged layer directory.
func ExecuteStep(ctx context.Context, step manifest.BuildStep, contextDir string, stageDir string, env map[string]string) error {
	switch step.Action {
	case "copy":
		return ExecuteCopy(step, contextDir, stageDir)
	case "run":
		return executeRun(ctx, step, stageDir, env)
	case "env":
		return executeEnv(step, env)
	default:
		return fmt.Errorf("unsupported build step action: %q", step.Action)
	}
}

func ExecuteCopy(step manifest.BuildStep, contextDir string, stageDir string) error {
	srcPath := filepath.Join(contextDir, step.From)
	destPath := filepath.Join(stageDir, strings.TrimPrefix(step.To, "/"))

	info, err := os.Stat(srcPath)
	if err != nil {
		return fmt.Errorf("copy source %q not found: %w", step.From, err)
	}

	if info.IsDir() {
		return copyDir(srcPath, destPath, step.Mode)
	}
	return copyFile(srcPath, destPath, step.Mode, info.Mode())
}

func copyFile(src, dest string, overrideMode string, srcMode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return fmt.Errorf("failed to create destination directory: %w", err)
	}

	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	mode := srcMode
	if overrideMode != "" {
		parsed, err := strconv.ParseUint(overrideMode, 8, 32)
		if err != nil {
			return fmt.Errorf("invalid mode %q: %w", overrideMode, err)
		}
		mode = os.FileMode(parsed)
	}

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return nil
}

func copyDir(src, dest string, overrideMode string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)

		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target, overrideMode, info.Mode())
	})
}

func executeRun(ctx context.Context, step manifest.BuildStep, stageDir string, env map[string]string) error {
	cmd := exec.CommandContext(ctx, "/bin/sh", "-c", step.Command)
	cmd.Dir = stageDir

	// Assemble environment variables
	envSlice := os.Environ()
	for k, v := range env {
		envSlice = append(envSlice, fmt.Sprintf("%s=%s", k, v))
	}
	cmd.Env = envSlice

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command %q failed (%w): %s", step.Command, err, string(output))
	}
	return nil
}

func executeEnv(step manifest.BuildStep, env map[string]string) error {
	for k, v := range step.Vars {
		env[k] = v
	}
	return nil
}
