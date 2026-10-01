package image

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/PeteMadCoder/vpok/internal/agent"
	"github.com/PeteMadCoder/vpok/internal/manifest"
	"github.com/PeteMadCoder/vpok/internal/store"
)

func TestAssemble_Complete(t *testing.T) {
	tempDir := t.TempDir()
	storeDir := filepath.Join(tempDir, "store")
	workDir := filepath.Join(tempDir, "work")

	casStore, err := store.NewCASStore(storeDir)
	if err != nil {
		t.Fatalf("failed to initialize store: %v", err)
	}

	// Create package layer with an application binary
	var layerBuf bytes.Buffer
	tw := tar.NewWriter(&layerBuf)
	appContent := []byte("#!/bin/sh\necho 'running app'\n")
	tw.WriteHeader(&tar.Header{
		Name:     "opt/app/bin/server",
		Mode:     0755,
		Size:     int64(len(appContent)),
		Typeflag: tar.TypeReg,
	})
	tw.Write(appContent)
	tw.Close()

	layerDigest, layerSize, err := casStore.PutBlob(context.Background(), &layerBuf)
	if err != nil {
		t.Fatalf("failed to put layer blob: %v", err)
	}

	// Mock host vpok-agent binary
	mockAgentPath := filepath.Join(tempDir, "mock-vpok-agent")
	if err := os.WriteFile(mockAgentPath, []byte("mock agent binary"), 0755); err != nil {
		t.Fatalf("failed to create mock agent: %v", err)
	}

	pkgManifest := &manifest.Manifest{
		APIVersion: "vpok.io/v1",
		Kind:       "Manifest",
		Metadata: manifest.BuildMetadata{
			Name:      "test-pkg",
			Version:   "1.0.0",
			Publisher: "example.org",
		},
		Base: manifest.Base{
			Distribution: "alpine",
			Release:      "3.20",
			Architecture: "amd64",
		},
		Layers: []manifest.LayerRef{
			{
				Digest:    layerDigest.String(),
				Size:      layerSize,
				MediaType: "application/vnd.vpok.layer.v1.tar",
			},
		},
		EntryPoint: manifest.EntryPoint{
			Command:    []string{"/opt/app/bin/server"},
			WorkingDir: "/opt/app",
		},
		Requires: manifest.Requires{
			Resources: manifest.RequireResources{
				Memory: "128MiB",
				CPUs:   1,
			},
			DataDirs: []manifest.DataDirs{
				{Path: "/var/lib/data"},
			},
		},
		Service: &manifest.BuildService{
			Ports: []manifest.ServicePort{
				{Name: "http", ContainerPort: 8080, Protocol: "tcp"},
			},
			Health: &manifest.HealthCheck{
				Type:     "http",
				Path:     "/healthz",
				Port:     8080,
				Interval: "5s",
				Timeout:  "1s",
				Failures: 2,
			},
		},
	}

	deploySpec := &manifest.DeploySpec{
		APIVersion: "vpok.io/v1",
		Kind:       "Deploy",
		Metadata: manifest.DeployMetadata{
			Name: "test-deployment",
		},
		Package: manifest.DeployPackage{
			Source:  "example.org/test-pkg",
			Version: "1.0.0",
		},
		Resources: manifest.DeployResources{
			Memory: "256MiB",
			CPUs:   1,
		},
		Storage: []manifest.DeployStorage{
			{
				Name:      "data",
				GuestPath: "/var/lib/data",
				Type:      "volume",
				Size:      "1GiB",
			},
		},
		Env: map[string]string{
			"PORT": "8080",
			"MODE": "production",
		},
		Shutdown: &manifest.DeployShutdown{
			GracePeriod: "15s",
		},
	}

	res, err := Assemble(context.Background(), AssemblyOptions{
		Store:           casStore,
		Manifest:        pkgManifest,
		DeploySpec:      deploySpec,
		AgentBinaryPath: mockAgentPath,
		WorkDir:         workDir,
	})
	if err != nil {
		t.Fatalf("Assemble failed: %v", err)
	}

	// 1. Verify rootfs contents
	appFile := filepath.Join(res.RootfsDir, "opt", "app", "bin", "server")
	if _, err := os.Stat(appFile); err != nil {
		t.Errorf("expected package binary at %s, got: %v", appFile, err)
	}

	// 2. Verify agent binary & init symlink
	agentFile := filepath.Join(res.RootfsDir, "bin", "vpok-agent")
	if _, err := os.Stat(agentFile); err != nil {
		t.Errorf("expected agent binary at %s, got: %v", agentFile, err)
	}
	initTarget, err := os.Readlink(filepath.Join(res.RootfsDir, "init"))
	if err != nil || initTarget != "/bin/vpok-agent" {
		t.Errorf("expected /init symlink to /bin/vpok-agent, got target %q, err: %v", initTarget, err)
	}

	// 3. Verify storage mount point exists
	mountPoint := filepath.Join(res.RootfsDir, "var", "lib", "data")
	if info, err := os.Stat(mountPoint); err != nil || !info.IsDir() {
		t.Errorf("expected mountpoint dir at %s, err: %v", mountPoint, err)
	}

	// 4. Verify agent config.json
	cfg, err := agent.LoadConfig(res.ConfigPath)
	if err != nil {
		t.Fatalf("failed to load generated agent config: %v", err)
	}
	if len(cfg.Entrypoint) != 1 || cfg.Entrypoint[0] != "/opt/app/bin/server" {
		t.Errorf("unexpected entrypoint: %v", cfg.Entrypoint)
	}
	if cfg.WorkingDir != "/opt/app" {
		t.Errorf("unexpected workingDir: %s", cfg.WorkingDir)
	}
	if cfg.Env["PORT"] != "8080" || cfg.Env["MODE"] != "production" {
		t.Errorf("unexpected env: %v", cfg.Env)
	}
	if cfg.GracePeriod != "15s" {
		t.Errorf("unexpected gracePeriod: %s", cfg.GracePeriod)
	}
	if cfg.HealthCheck == nil || cfg.HealthCheck.Path != "/healthz" || cfg.HealthCheck.Port != 8080 {
		t.Errorf("unexpected health check: %+v", cfg.HealthCheck)
	}

	// 5. Verify initrd.img exists and is non-empty
	info, err := os.Stat(res.InitrdPath)
	if err != nil || info.Size() == 0 {
		t.Errorf("initrd.img was not generated or is empty: %v", err)
	}
}

func TestAssemble_ValidationErrors(t *testing.T) {
	tempDir := t.TempDir()
	storeDir := filepath.Join(tempDir, "store")
	casStore, err := store.NewCASStore(storeDir)
	if err != nil {
		t.Fatalf("failed to init store: %v", err)
	}

	t.Run("missing manifest", func(t *testing.T) {
		_, err := Assemble(context.Background(), AssemblyOptions{
			Store:      casStore,
			DeploySpec: &manifest.DeploySpec{},
		})
		if err == nil {
			t.Error("expected error for missing manifest, got nil")
		}
	})

	t.Run("missing deploy spec", func(t *testing.T) {
		_, err := Assemble(context.Background(), AssemblyOptions{
			Store:    casStore,
			Manifest: &manifest.Manifest{},
		})
		if err == nil {
			t.Error("expected error for missing deploy spec, got nil")
		}
	})

	t.Run("missing store", func(t *testing.T) {
		_, err := Assemble(context.Background(), AssemblyOptions{
			Manifest:   &manifest.Manifest{},
			DeploySpec: &manifest.DeploySpec{},
		})
		if err == nil {
			t.Error("expected error for missing store, got nil")
		}
	})
}
