package agent

import (
	"bytes"
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLoadConfig_Valid(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.json")

	raw := `{
		"entrypoint": ["/bin/echo", "hello"],
		"workingDir": "/tmp",
		"env": {"TEST_VAR": "123"},
		"secrets": {"token": "secret-value"},
		"gracePeriod": "5s",
		"healthCheck": {
			"type": "http",
			"port": 8080,
			"path": "/healthz",
			"interval": "2s",
			"timeout": "1s",
			"failures": 2
		}
	}`

	if err := os.WriteFile(configPath, []byte(raw), 0644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	cfg, err := LoadConfig(configPath)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if len(cfg.Entrypoint) != 2 || cfg.Entrypoint[0] != "/bin/echo" || cfg.Entrypoint[1] != "hello" {
		t.Errorf("unexpected entrypoint: %v", cfg.Entrypoint)
	}
	if cfg.WorkingDir != "/tmp" {
		t.Errorf("expected workingDir /tmp, got %s", cfg.WorkingDir)
	}
	if cfg.Env["TEST_VAR"] != "123" {
		t.Errorf("expected TEST_VAR=123, got %s", cfg.Env["TEST_VAR"])
	}
	if cfg.Secrets["token"] != "secret-value" {
		t.Errorf("expected secret token=secret-value, got %s", cfg.Secrets["token"])
	}
	if d := cfg.ParseGracePeriod(); d != 5*time.Second {
		t.Errorf("expected 5s grace period, got %v", d)
	}
	if cfg.HealthCheck == nil || cfg.HealthCheck.Port != 8080 {
		t.Errorf("unexpected health check config: %+v", cfg.HealthCheck)
	}
}

func TestLoadConfig_Errors(t *testing.T) {
	t.Run("file not found", func(t *testing.T) {
		_, err := LoadConfig("/nonexistent/path/config.json")
		if err == nil {
			t.Fatal("expected error for nonexistent file, got nil")
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "invalid.json")
		_ = os.WriteFile(configPath, []byte("{invalid json"), 0644)

		_, err := LoadConfig(configPath)
		if err == nil {
			t.Fatal("expected error for invalid json, got nil")
		}
	})

	t.Run("empty entrypoint", func(t *testing.T) {
		tempDir := t.TempDir()
		configPath := filepath.Join(tempDir, "empty.json")
		_ = os.WriteFile(configPath, []byte(`{"entrypoint": []}`), 0644)

		_, err := LoadConfig(configPath)
		if err == nil {
			t.Fatal("expected error for empty entrypoint, got nil")
		}
	})
}

func TestParseGracePeriod_DefaultAndInvalid(t *testing.T) {
	cfgEmpty := &Config{GracePeriod: ""}
	if d := cfgEmpty.ParseGracePeriod(); d != 10*time.Second {
		t.Errorf("expected default 10s, got %v", d)
	}

	cfgInvalid := &Config{GracePeriod: "not-a-duration"}
	if d := cfgInvalid.ParseGracePeriod(); d != 10*time.Second {
		t.Errorf("expected fallback 10s, got %v", d)
	}
}

func TestSetupSecrets(t *testing.T) {
	t.Run("empty secrets", func(t *testing.T) {
		tempDir := t.TempDir()
		if err := SetupSecrets(filepath.Join(tempDir, "secrets"), nil); err != nil {
			t.Fatalf("expected nil for empty secrets, got %v", err)
		}
	})

	t.Run("successful write with 0400 permissions", func(t *testing.T) {
		tempDir := t.TempDir()
		secretsDir := filepath.Join(tempDir, "secrets")

		secrets := map[string]string{
			"api-key": "supersecret123",
			"cert":    "cert-data\n",
		}

		if err := SetupSecrets(secretsDir, secrets); err != nil {
			t.Fatalf("SetupSecrets failed: %v", err)
		}

		for name, val := range secrets {
			targetFile := filepath.Join(secretsDir, name)
			data, err := os.ReadFile(targetFile)
			if err != nil {
				t.Fatalf("failed to read secret file %s: %v", name, err)
			}
			if string(data) != val {
				t.Errorf("expected content %q, got %q", val, string(data))
			}

			info, err := os.Stat(targetFile)
			if err != nil {
				t.Fatalf("failed to stat secret file %s: %v", name, err)
			}
			if info.Mode().Perm() != 0400 {
				t.Errorf("expected file mode 0400, got %o", info.Mode().Perm())
			}
		}
	})

	t.Run("path traversal prevention", func(t *testing.T) {
		tempDir := t.TempDir()
		secretsDir := filepath.Join(tempDir, "secrets")

		badSecrets := map[string]string{
			"../escape": "bad",
		}
		if err := SetupSecrets(secretsDir, badSecrets); err == nil {
			t.Fatalf("expected error for path traversal secret name, got nil")
		}
	})
}

func TestSupervisor_RunSuccess(t *testing.T) {
	cfg := &Config{
		Entrypoint: []string{"sh", "-c", "echo standard-out && echo standard-err >&2"},
	}

	var stdout, stderr bytes.Buffer
	sup := NewSupervisor(cfg, &stdout, &stderr)

	ctx := context.Background()
	done, err := sup.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	res := <-done
	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}
	if !strings.Contains(stdout.String(), "standard-out") {
		t.Errorf("expected 'standard-out' in stdout, got %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "standard-err") {
		t.Errorf("expected 'standard-err' in stderr, got %q", stderr.String())
	}
}

func TestSupervisor_EnvironmentAndWorkingDir(t *testing.T) {
	tempDir := t.TempDir()
	cfg := &Config{
		Entrypoint: []string{"sh", "-c", "pwd && echo $TEST_VAL"},
		WorkingDir: tempDir,
		Env: map[string]string{
			"TEST_VAL": "custom_environment_value",
		},
	}

	var stdout bytes.Buffer
	sup := NewSupervisor(cfg, &stdout, nil)

	ctx := context.Background()
	done, err := sup.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	res := <-done
	if res.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", res.ExitCode)
	}

	out := stdout.String()
	if !strings.Contains(out, tempDir) {
		t.Errorf("expected pwd output to contain %s, got %q", tempDir, out)
	}
	if !strings.Contains(out, "custom_environment_value") {
		t.Errorf("expected env output to contain custom_environment_value, got %q", out)
	}
}

func TestSupervisor_NonZeroExit(t *testing.T) {
	cfg := &Config{
		Entrypoint: []string{"sh", "-c", "exit 42"},
	}

	sup := NewSupervisor(cfg, nil, nil)
	ctx := context.Background()
	done, err := sup.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	res := <-done
	if res.ExitCode != 42 {
		t.Errorf("expected exit code 42, got %d", res.ExitCode)
	}
}

func TestSupervisor_StopGraceful(t *testing.T) {
	cfg := &Config{
		Entrypoint:  []string{"sh", "-c", "trap 'exit 0' TERM; while true; do sleep 0.05; done"},
		GracePeriod: "1s",
	}

	sup := NewSupervisor(cfg, nil, nil)
	ctx := context.Background()
	done, err := sup.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	if err := sup.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	select {
	case res := <-done:
		if res.ExitCode != 0 {
			t.Errorf("expected graceful exit code 0, got %d", res.ExitCode)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("process did not terminate within expected time")
	}
}

func TestSupervisor_StopForceKill(t *testing.T) {
	cfg := &Config{
		// Process ignores SIGTERM
		Entrypoint:  []string{"sh", "-c", "trap '' TERM; while true; do sleep 0.05; done"},
		GracePeriod: "200ms",
	}

	sup := NewSupervisor(cfg, nil, nil)
	ctx := context.Background()
	done, err := sup.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	startStop := time.Now()
	if err := sup.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	select {
	case <-done:
		elapsed := time.Since(startStop)
		if elapsed < 150*time.Millisecond {
			t.Errorf("expected force kill to take at least gracePeriod (200ms), took %v", elapsed)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("process was not killed after gracePeriod expiration")
	}
}

func TestHealthChecker_HTTP(t *testing.T) {
	var mu sync.Mutex
	healthy := true

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if healthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()

	portStr := strings.TrimPrefix(server.URL, "http://127.0.0.1:")
	port, err := strconv.Atoi(portStr)
	if err != nil {
		t.Fatalf("failed to parse test server port: %v", err)
	}

	cfg := &HealthCheckConfig{
		Type:     "http",
		Port:     port,
		Path:     "/",
		Timeout:  "500ms",
		Interval: "50ms",
		Failures: 2,
	}

	stateChanges := make(chan bool, 10)
	checker := NewHealthChecker(cfg, func(h bool, err error) {
		stateChanges <- h
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go checker.Start(ctx)

	// Verify single probe
	if err := checker.CheckOnce(ctx); err != nil {
		t.Fatalf("expected healthy single check, got: %v", err)
	}

	// Toggle unhealthy
	mu.Lock()
	healthy = false
	mu.Unlock()

	select {
	case h := <-stateChanges:
		if h {
			t.Errorf("expected unhealthy state change")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for unhealthy state change")
	}

	// Toggle back to healthy
	mu.Lock()
	healthy = true
	mu.Unlock()

	select {
	case h := <-stateChanges:
		if !h {
			t.Errorf("expected healthy recovery state change")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for healthy recovery state change")
	}
}

func TestHealthChecker_TCP(t *testing.T) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen on TCP: %v", err)
	}
	defer l.Close()

	port := l.Addr().(*net.TCPAddr).Port

	cfg := &HealthCheckConfig{
		Type:    "tcp",
		Port:    port,
		Timeout: "500ms",
	}

	checker := NewHealthChecker(cfg, nil)
	ctx := context.Background()

	if err := checker.CheckOnce(ctx); err != nil {
		t.Fatalf("expected successful TCP health check, got: %v", err)
	}

	// Close listener and test failure
	l.Close()
	if err := checker.CheckOnce(ctx); err == nil {
		t.Fatal("expected failure on closed TCP port, got nil")
	}
}
