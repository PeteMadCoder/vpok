package builder

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/PeteMadCoder/vpok/internal/manifest"
)

func TestExecuteStep_Copy(t *testing.T) {
	ctxDir := t.TempDir()
	stageDir := t.TempDir()

	// Prepare context files
	srcFile := filepath.Join(ctxDir, "app.bin")
	if err := os.WriteFile(srcFile, []byte("binary payload"), 0644); err != nil {
		t.Fatalf("failed to write app.bin: %v", err)
	}

	step := manifest.BuildStep{
		Action: "copy",
		From:   "app.bin",
		To:     "/opt/bin/app",
		Mode:   "0755",
	}

	env := make(map[string]string)
	if err := ExecuteStep(context.Background(), step, ctxDir, stageDir, env); err != nil {
		t.Fatalf("ExecuteStep copy failed: %v", err)
	}

	destFile := filepath.Join(stageDir, "opt", "bin", "app")
	content, err := os.ReadFile(destFile)
	if err != nil {
		t.Fatalf("failed to read copied file: %v", err)
	}
	if string(content) != "binary payload" {
		t.Errorf("copied content mismatch: got %q", string(content))
	}

	info, err := os.Stat(destFile)
	if err != nil {
		t.Fatalf("failed to stat destination: %v", err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("expected permissions 0755, got %v", info.Mode().Perm())
	}
}

func TestExecuteStep_Run(t *testing.T) {
	ctxDir := t.TempDir()
	stageDir := t.TempDir()

	step := manifest.BuildStep{
		Action:  "run",
		Command: "echo $CUSTOM_VAR > generated.txt",
	}

	env := map[string]string{
		"CUSTOM_VAR": "vpok-build-value",
	}

	if err := ExecuteStep(context.Background(), step, ctxDir, stageDir, env); err != nil {
		t.Fatalf("ExecuteStep run failed: %v", err)
	}

	outputFile := filepath.Join(stageDir, "generated.txt")
	content, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("failed to read generated output: %v", err)
	}
	if string(content) != "vpok-build-value\n" {
		t.Errorf("unexpected command output: %q", string(content))
	}
}

func TestExecuteStep_Env(t *testing.T) {
	env := make(map[string]string)
	step := manifest.BuildStep{
		Action: "env",
		Vars: map[string]string{
			"GOOS":   "linux",
			"GOARCH": "amd64",
		},
	}

	if err := ExecuteStep(context.Background(), step, "", "", env); err != nil {
		t.Fatalf("ExecuteStep env failed: %v", err)
	}

	if env["GOOS"] != "linux" || env["GOARCH"] != "amd64" {
		t.Errorf("unexpected env map: %+v", env)
	}
}
