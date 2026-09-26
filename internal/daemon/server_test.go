package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/PeteMadCoder/vpok/internal/manifest"
	"github.com/PeteMadCoder/vpok/internal/store"
)

func setupTestServer(t *testing.T) (*Server, string, *store.CASStore) {
	t.Helper()
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "vpokd.sock")
	storePath := filepath.Join(tempDir, "store")

	casStore, err := store.NewCASStore(storePath)
	if err != nil {
		t.Fatalf("failed to create CAS store: %v", err)
	}

	srv := NewServer(casStore, socketPath, "0.1.0-test")

	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Logf("server stopped with error: %v", err)
		}
	}()

	var connected bool
	for i := 0; i < 50; i++ {
		conn, err := net.Dial("unix", socketPath)
		if err == nil {
			conn.Close()
			connected = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if !connected {
		t.Fatalf("timed out waiting for server socket at %s", socketPath)
	}

	t.Cleanup(func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	})

	return srv, socketPath, casStore
}

func newTestHTTPClient(socketPath string) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", socketPath)
			},
		},
		Timeout: 5 * time.Second,
	}
}

func TestServerPing(t *testing.T) {
	_, socketPath, _ := setupTestServer(t)
	client := newTestHTTPClient(socketPath)

	resp, err := client.Get("http://unix/v1/ping")
	if err != nil {
		t.Fatalf("GET /v1/ping failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	var pingResp PingResponse
	if err := json.NewDecoder(resp.Body).Decode(&pingResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if pingResp.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", pingResp.Status)
	}
	if pingResp.Version != "0.1.0-test" {
		t.Errorf("expected version '0.1.0-test', got %q", pingResp.Version)
	}
}

func TestServerManifestLifecycle(t *testing.T) {
	_, socketPath, casStore := setupTestServer(t)
	client := newTestHTTPClient(socketPath)

	testManifest := manifest.Manifest{
		APIVersion: "vpok.io/v1",
		Kind:       "Manifest",
		Metadata: manifest.BuildMetadata{
			Name:        "demo-service",
			Version:     "1.0.0",
			Publisher:   "example.com",
			Description: "A demo workload",
		},
		Base: manifest.Base{
			Distribution: "alpine",
			Release:      "3.20",
			Architecture: "amd64",
		},
		EntryPoint: manifest.EntryPoint{
			Command: []string{"/bin/echo", "hello"},
		},
		Requires: manifest.Requires{
			Resources: manifest.RequireResources{
				Memory: "64MiB",
				CPUs:   1,
			},
		},
	}

	var digestStr string

	t.Run("post manifest", func(t *testing.T) {
		data, err := json.Marshal(testManifest)
		if err != nil {
			t.Fatalf("failed to marshal manifest: %v", err)
		}

		resp, err := client.Post("http://unix/v1/manifests", "application/json", bytes.NewReader(data))
		if err != nil {
			t.Fatalf("POST /v1/manifests failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusCreated {
			body, _ := io.ReadAll(resp.Body)
			t.Fatalf("expected status 201, got %d: %s", resp.StatusCode, string(body))
		}

		var result map[string]string
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		digestStr = result["digest"]
		if digestStr == "" {
			t.Fatal("expected non-empty digest in response")
		}
	})

	t.Run("list manifests", func(t *testing.T) {
		resp, err := client.Get("http://unix/v1/manifests")
		if err != nil {
			t.Fatalf("GET /v1/manifests failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}

		var summaries []ManifestSummary
		if err := json.NewDecoder(resp.Body).Decode(&summaries); err != nil {
			t.Fatalf("failed to decode summaries: %v", err)
		}

		if len(summaries) != 1 {
			t.Fatalf("expected 1 manifest summary, got %d", len(summaries))
		}

		item := summaries[0]
		if item.Name != "demo-service" || item.Version != "1.0.0" || item.Publisher != "example.com" {
			t.Errorf("unexpected summary content: %+v", item)
		}
		if item.Digest != digestStr {
			t.Errorf("expected digest %s, got %s", digestStr, item.Digest)
		}
	})

	t.Run("get manifest by digest", func(t *testing.T) {
		url := fmt.Sprintf("http://unix/v1/manifests/%s", digestStr)
		resp, err := client.Get(url)
		if err != nil {
			t.Fatalf("GET manifest by digest failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected status 200, got %d", resp.StatusCode)
		}

		var m manifest.Manifest
		if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
			t.Fatalf("failed to decode retrieved manifest: %v", err)
		}

		if m.Metadata.Name != "demo-service" || m.Metadata.Version != "1.0.0" {
			t.Errorf("unexpected manifest content: %+v", m.Metadata)
		}
	})

	t.Run("get manifest not found", func(t *testing.T) {
		url := "http://unix/v1/manifests/sha256:0000000000000000000000000000000000000000000000000000000000000000"
		resp, err := client.Get(url)
		if err != nil {
			t.Fatalf("GET non-existent manifest failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("expected status 404, got %d", resp.StatusCode)
		}
	})

	t.Run("get manifest invalid digest format", func(t *testing.T) {
		resp, err := client.Get("http://unix/v1/manifests/invalid-digest")
		if err != nil {
			t.Fatalf("GET invalid digest failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("expected status 400, got %d", resp.StatusCode)
		}
	})

	t.Run("method not allowed on ping", func(t *testing.T) {
		resp, err := client.Post("http://unix/v1/ping", "application/json", bytes.NewReader([]byte("{}")))
		if err != nil {
			t.Fatalf("POST /v1/ping failed: %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Fatalf("expected status 405, got %d", resp.StatusCode)
		}
	})

	_ = casStore
}
