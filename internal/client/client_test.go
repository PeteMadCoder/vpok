package client

import (
	"context"
	"errors"
	"net"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"github.com/PeteMadCoder/vpok/internal/daemon"
	"github.com/PeteMadCoder/vpok/internal/manifest"
	"github.com/PeteMadCoder/vpok/internal/store"
)

func setupTestDaemon(t *testing.T) (*Client, func()) {
	t.Helper()
	tempDir := t.TempDir()
	socketPath := filepath.Join(tempDir, "vpokd.sock")
	storePath := filepath.Join(tempDir, "store")

	casStore, err := store.NewCASStore(storePath)
	if err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}

	srv := daemon.NewServer(casStore, socketPath, "0.1.0-client-test")

	go func() {
		if err := srv.Start(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			t.Logf("server stopped: %v", err)
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
		t.Fatalf("timed out connecting to server socket: %s", socketPath)
	}

	c := New(socketPath)

	cleanup := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutdownCtx)
	}

	return c, cleanup
}

func TestClientPing(t *testing.T) {
	c, cleanup := setupTestDaemon(t)
	defer cleanup()

	resp, err := c.Ping(context.Background())
	if err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got %q", resp.Status)
	}
	if resp.Version != "0.1.0-client-test" {
		t.Errorf("expected version '0.1.0-client-test', got %q", resp.Version)
	}
}

func TestClientManifestOperations(t *testing.T) {
	c, cleanup := setupTestDaemon(t)
	defer cleanup()

	ctx := context.Background()

	testManifest := manifest.Manifest{
		APIVersion: "vpok.io/v1",
		Kind:       "Manifest",
		Metadata: manifest.BuildMetadata{
			Name:        "client-pkg",
			Version:     "2.0.0",
			Publisher:   "client.example.com",
			Description: "Client test package",
		},
		Base: manifest.Base{
			Distribution: "alpine",
			Release:      "3.20",
			Architecture: "amd64",
		},
		EntryPoint: manifest.EntryPoint{
			Command: []string{"/bin/sh"},
		},
		Requires: manifest.Requires{
			Resources: manifest.RequireResources{
				Memory: "128MiB",
				CPUs:   2,
			},
		},
	}

	// 1. Put Manifest
	digestStr, err := c.PutManifest(ctx, &testManifest)
	if err != nil {
		t.Fatalf("PutManifest failed: %v", err)
	}
	if digestStr == "" {
		t.Fatal("expected non-empty digest string")
	}

	// 2. List Manifests
	list, err := c.ListManifests(ctx)
	if err != nil {
		t.Fatalf("ListManifests failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(list))
	}
	if list[0].Name != "client-pkg" || list[0].Version != "2.0.0" {
		t.Errorf("unexpected list entry: %+v", list[0])
	}

	// 3. Get Manifest
	m, err := c.GetManifest(ctx, store.Digest(digestStr))
	if err != nil {
		t.Fatalf("GetManifest failed: %v", err)
	}
	if m.Metadata.Name != "client-pkg" || m.Metadata.Publisher != "client.example.com" {
		t.Errorf("unexpected retrieved manifest: %+v", m.Metadata)
	}

	// 4. Get Non-existent Manifest
	_, err = c.GetManifest(ctx, "sha256:0000000000000000000000000000000000000000000000000000000000000000")
	if err == nil {
		t.Error("expected error for non-existent manifest, got nil")
	}
}

func TestClientConnectionFailure(t *testing.T) {
	c := New("/tmp/nonexistent-socket-12345.sock")

	_, err := c.Ping(context.Background())
	if err == nil {
		t.Error("expected error when connecting to non-existent socket, got nil")
	}
}
