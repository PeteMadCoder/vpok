package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/PeteMadCoder/vpok/internal/agent"
)

func TestAgentMain_CLIExecution(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")
	secretsDir := filepath.Join(tempDir, "secrets")

	cfg := agent.Config{
		Entrypoint: []string{"sh", "-c", "exit 0"},
		Secrets: map[string]string{
			"app-token": "secret123",
		},
		GracePeriod: "1s",
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "run", "main.go", "--config", configPath, "--secrets-dir", secretsDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("agent process failed: %v, output: %s", err, string(output))
	}

	secretFile := filepath.Join(secretsDir, "app-token")
	val, err := os.ReadFile(secretFile)
	if err != nil {
		t.Fatalf("failed to read written secret: %v", err)
	}
	if string(val) != "secret123" {
		t.Errorf("expected secret content 'secret123', got %q", string(val))
	}
}
