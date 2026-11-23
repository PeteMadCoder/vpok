package engine

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/PeteMadCoder/vpok/internal/manifest"
	"github.com/PeteMadCoder/vpok/internal/runtime"
	"github.com/PeteMadCoder/vpok/internal/store"
)

// Mock Instance implementation for testing
type mockInstance struct {
	id          string
	state       runtime.State
	pid         int
	startCalled bool
	stopCalled  bool
}

func (m *mockInstance) ID() string   { return m.id }
func (m *mockInstance) State() runtime.State { return m.state }
func (m *mockInstance) PID() int     { return m.pid }
func (m *mockInstance) Start(ctx context.Context) error {
	m.startCalled = true
	m.state = runtime.StateRunning
	return nil
}
func (m *mockInstance) Stop(ctx context.Context, grace time.Duration) error {
	m.stopCalled = true
	m.state = runtime.StateStopped
	return nil
}
func (m *mockInstance) Wait(ctx context.Context) (runtime.ExitStatus, error) {
	return runtime.ExitStatus{ExitCode: 0}, nil
}
func (m *mockInstance) Logs() (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("mock logs")), nil
}

// Mock Runtime implementation for testing
type mockRuntime struct {
	createdConfigs []runtime.VMConfig
	instances      map[string]*mockInstance
	createErr      error
}

func newMockRuntime() *mockRuntime {
	return &mockRuntime{
		instances: make(map[string]*mockInstance),
	}
}

func (m *mockRuntime) Create(ctx context.Context, cfg runtime.VMConfig) (runtime.Instance, error) {
	if m.createErr != nil {
		return nil, m.createErr
	}
	m.createdConfigs = append(m.createdConfigs, cfg)
	inst := &mockInstance{
		id:    cfg.ID,
		state: runtime.StateCreated,
		pid:   1234,
	}
	m.instances[cfg.ID] = inst
	return inst, nil
}

func (m *mockRuntime) Get(ctx context.Context, id string) (runtime.Instance, error) {
	inst, exists := m.instances[id]
	if !exists {
		return nil, errors.New("instance not found")
	}
	return inst, nil
}

func (m *mockRuntime) List(ctx context.Context) ([]runtime.InstanceSummary, error) {
	var list []runtime.InstanceSummary
	for _, inst := range m.instances {
		list = append(list, runtime.InstanceSummary{
			ID:    inst.id,
			State: inst.state,
			PID:   inst.pid,
		})
	}
	return list, nil
}

func (m *mockRuntime) Delete(ctx context.Context, id string) error {
	if _, exists := m.instances[id]; !exists {
		return errors.New("instance not found")
	}
	delete(m.instances, id)
	return nil
}

func TestNew_Validation(t *testing.T) {
	tempDir := t.TempDir()
	casStore, err := store.NewCASStore(filepath.Join(tempDir, "store"))
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	rt := newMockRuntime()

	// Missing store
	if _, err := New(Config{Runtime: rt}); err == nil {
		t.Errorf("expected error when store is nil, got nil")
	}

	// Missing runtime
	if _, err := New(Config{Store: casStore}); err == nil {
		t.Errorf("expected error when runtime is nil, got nil")
	}

	// Custom work dir
	customWorkDir := filepath.Join(tempDir, "custom-work")
	eng, err := New(Config{
		Store:   casStore,
		Runtime: rt,
		WorkDir: customWorkDir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if eng.workDir != customWorkDir {
		t.Errorf("expected workDir %s, got %s", customWorkDir, eng.workDir)
	}
	if _, err := os.Stat(customWorkDir); os.IsNotExist(err) {
		t.Errorf("workDir was not created")
	}
}

func TestEngine_ParseMemoryMB(t *testing.T) {
	tests := []struct {
		input    string
		expected int
		wantErr  bool
	}{
		{"", defaultMemMB, false},
		{"64MiB", 64, false},
		{"1GiB", 1024, false},
		{"512MiB", 512, false},
		{"512MB", 488, false},
		{"invalid-size", 0, true},
	}

	for _, tt := range tests {
		got, err := parseMemoryMB(tt.input)
		if tt.wantErr {
			if err == nil {
				t.Errorf("parseMemoryMB(%q) expected error, got nil", tt.input)
			}
		} else {
			if err != nil {
				t.Errorf("parseMemoryMB(%q) unexpected error: %v", tt.input, err)
			}
			if got != tt.expected {
				t.Errorf("parseMemoryMB(%q) = %d, expected %d", tt.input, got, tt.expected)
			}
		}
	}
}

func TestEngine_ResolveKernel(t *testing.T) {
	tempDir := t.TempDir()
	casStore, _ := store.NewCASStore(filepath.Join(tempDir, "store"))
	rt := newMockRuntime()

	dummyKernel := filepath.Join(tempDir, "vmlinuz-dummy")
	if err := os.WriteFile(dummyKernel, []byte("fake kernel"), 0644); err != nil {
		t.Fatalf("failed to write dummy kernel: %v", err)
	}

	// 1. Explicit kernel that exists
	eng, _ := New(Config{
		Store:      casStore,
		Runtime:    rt,
		KernelPath: dummyKernel,
		WorkDir:    tempDir,
	})
	resolved, err := eng.resolveKernel()
	if err != nil || resolved != dummyKernel {
		t.Errorf("expected %s, got %s (err: %v)", dummyKernel, resolved, err)
	}

	// 2. Explicit kernel that does NOT exist
	engNonExistent, _ := New(Config{
		Store:      casStore,
		Runtime:    rt,
		KernelPath: filepath.Join(tempDir, "nonexistent"),
		WorkDir:    tempDir,
	})
	if _, err := engNonExistent.resolveKernel(); err == nil {
		t.Errorf("expected error for nonexistent kernel path, got nil")
	}

	// 3. Via environment variable VPOK_KERNEL
	t.Setenv("VPOK_KERNEL", dummyKernel)
	engEnv, _ := New(Config{
		Store:   casStore,
		Runtime: rt,
		WorkDir: tempDir,
	})
	resolvedEnv, err := engEnv.resolveKernel()
	if err != nil || resolvedEnv != dummyKernel {
		t.Errorf("expected %s from env, got %s (err: %v)", dummyKernel, resolvedEnv, err)
	}
}

func TestEngine_ResolveAgentBinary(t *testing.T) {
	tempDir := t.TempDir()
	casStore, _ := store.NewCASStore(filepath.Join(tempDir, "store"))
	rt := newMockRuntime()

	dummyAgent := filepath.Join(tempDir, "mock-agent")
	if err := os.WriteFile(dummyAgent, []byte("fake agent"), 0755); err != nil {
		t.Fatalf("failed to write dummy agent: %v", err)
	}

	// 1. Explicit agent that exists
	eng, _ := New(Config{
		Store:           casStore,
		Runtime:         rt,
		AgentBinaryPath: dummyAgent,
		WorkDir:         tempDir,
	})
	resolved, err := eng.resolveAgentBinary()
	if err != nil || resolved != dummyAgent {
		t.Errorf("expected %s, got %s (err: %v)", dummyAgent, resolved, err)
	}

	// 2. Explicit agent that does NOT exist
	engNonExistent, _ := New(Config{
		Store:           casStore,
		Runtime:         rt,
		AgentBinaryPath: filepath.Join(tempDir, "nonexistent"),
		WorkDir:         tempDir,
	})
	if _, err := engNonExistent.resolveAgentBinary(); err == nil {
		t.Errorf("expected error for nonexistent agent binary, got nil")
	}

	// 3. Via environment variable VPOK_AGENT_PATH
	t.Setenv("VPOK_AGENT_PATH", dummyAgent)
	engEnv, _ := New(Config{
		Store:   casStore,
		Runtime: rt,
		WorkDir: tempDir,
	})
	resolvedEnv, err := engEnv.resolveAgentBinary()
	if err != nil || resolvedEnv != dummyAgent {
		t.Errorf("expected %s from env, got %s (err: %v)", dummyAgent, resolvedEnv, err)
	}
}

func TestEngine_ResolveStorage(t *testing.T) {
	tempDir := t.TempDir()
	casStore, _ := store.NewCASStore(filepath.Join(tempDir, "store"))
	rt := newMockRuntime()

	eng, _ := New(Config{
		Store:   casStore,
		Runtime: rt,
		WorkDir: tempDir,
	})

	hostMountDir := filepath.Join(tempDir, "host-data")
	persistentTrue := true
	persistentFalse := false

	deploy := &manifest.DeploySpec{
		Storage: []manifest.DeployStorage{
			{
				Name:      "data-host",
				GuestPath: "/var/host",
				Type:      "hostPath",
				HostPath:  hostMountDir,
				Mode:      "ro",
			},
			{
				Name:       "data-vol-persist",
				GuestPath:  "/var/persist",
				Type:       "volume",
				Persistent: &persistentTrue,
				Mode:       "rw",
			},
			{
				Name:       "data-vol-ephemeral",
				GuestPath:  "/var/temp",
				Type:       "volume",
				Persistent: &persistentFalse,
				Mode:       "rw",
			},
		},
	}

	instanceDir := filepath.Join(tempDir, "inst-1")
	sharedDirs, err := eng.resolveStorage(deploy, instanceDir)
	if err != nil {
		t.Fatalf("resolveStorage failed: %v", err)
	}

	if len(sharedDirs) != 3 {
		t.Fatalf("expected 3 sharedDirs, got %d", len(sharedDirs))
	}

	// 1. hostPath
	if sharedDirs[0].Tag != "fsdev0" || sharedDirs[0].HostPath != hostMountDir || !sharedDirs[0].ReadOnly {
		t.Errorf("unexpected sharedDir[0]: %+v", sharedDirs[0])
	}
	// 2. persistent volume
	expectedPersistDir := filepath.Join(tempDir, "volumes", "data-vol-persist")
	if sharedDirs[1].Tag != "fsdev1" || sharedDirs[1].HostPath != expectedPersistDir || sharedDirs[1].ReadOnly {
		t.Errorf("unexpected sharedDir[1]: %+v", sharedDirs[1])
	}
	// 3. ephemeral volume
	expectedEphemeralDir := filepath.Join(instanceDir, "volumes", "data-vol-ephemeral")
	if sharedDirs[2].Tag != "fsdev2" || sharedDirs[2].HostPath != expectedEphemeralDir || sharedDirs[2].ReadOnly {
		t.Errorf("unexpected sharedDir[2]: %+v", sharedDirs[2])
	}

	// Test unsupported storage type
	invalidDeploy := &manifest.DeploySpec{
		Storage: []manifest.DeployStorage{
			{Name: "bad", Type: "unknown_type"},
		},
	}
	if _, err := eng.resolveStorage(invalidDeploy, instanceDir); err == nil {
		t.Errorf("expected error for unsupported storage type, got nil")
	}
}

func TestEngine_Run_Success(t *testing.T) {
	tempDir := t.TempDir()
	storeDir := filepath.Join(tempDir, "store")
	casStore, err := store.NewCASStore(storeDir)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	// 1. Put layer blob into store
	var layerBuf bytes.Buffer
	tw := tar.NewWriter(&layerBuf)
	content := []byte("#!/bin/sh\necho hello\n")
	tw.WriteHeader(&tar.Header{
		Name:     "bin/app",
		Mode:     0755,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	})
	tw.Write(content)
	tw.Close()

	layerDigest, layerSize, err := casStore.PutBlob(ctx, &layerBuf)
	if err != nil {
		t.Fatalf("failed to put blob: %v", err)
	}

	// 2. Setup mock kernel and agent binaries
	mockKernel := filepath.Join(tempDir, "vmlinuz")
	if err := os.WriteFile(mockKernel, []byte("fake kernel"), 0644); err != nil {
		t.Fatalf("failed to write kernel: %v", err)
	}
	mockAgent := filepath.Join(tempDir, "vpok-agent")
	if err := os.WriteFile(mockAgent, []byte("fake agent"), 0755); err != nil {
		t.Fatalf("failed to write agent: %v", err)
	}

	rt := newMockRuntime()
	eng, err := New(Config{
		Store:           casStore,
		Runtime:         rt,
		KernelPath:      mockKernel,
		AgentBinaryPath: mockAgent,
		WorkDir:         filepath.Join(tempDir, "engine-work"),
	})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	// 3. Prepare Package Manifest and Deploy Spec
	pkg := &manifest.Manifest{
		APIVersion: "vpok.io/v1",
		Kind:       "Manifest",
		Metadata: manifest.BuildMetadata{
			Name:      "test-service",
			Version:   "1.0.0",
			Publisher: "test.org",
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
		BuildSpec: manifest.BuildSpec{
			APIVersion: "vpok.io/v1",
			Kind:       "Build",
			Metadata: manifest.BuildMetadata{
				Name:      "test-service",
				Version:   "1.0.0",
				Publisher: "test.org",
			},
			Base: manifest.Base{
				Distribution: "alpine",
				Release:      "3.20",
				Architecture: "amd64",
			},
			EntryPoint: manifest.EntryPoint{
				Command: []string{"/bin/app"},
			},
			Requires: manifest.Requires{
				Resources: manifest.RequireResources{
					Memory: "128MiB",
					CPUs:   1,
				},
				DataDirs: []manifest.DataDirs{
					{Path: "/var/data"},
				},
			},
		},
		EntryPoint: manifest.EntryPoint{
			Command: []string{"/bin/app"},
		},
		Requires: manifest.Requires{
			Resources: manifest.RequireResources{
				Memory: "128MiB",
				CPUs:   1,
			},
			DataDirs: []manifest.DataDirs{
				{Path: "/var/data"},
			},
		},
	}

	deploy := &manifest.DeploySpec{
		APIVersion: "vpok.io/v1",
		Kind:       "Deploy",
		Metadata: manifest.DeployMetadata{
			Name: "test-service",
		},
		Package: manifest.DeployPackage{
			Source:  "registry.vpok.io/test-service",
			Version: "1.0.0",
		},
		Resources: manifest.DeployResources{
			Memory: "256MiB",
			CPUs:   2,
		},
		Storage: []manifest.DeployStorage{
			{
				Name:      "data",
				GuestPath: "/var/data",
				Type:      "hostPath",
				HostPath:  filepath.Join(tempDir, "shared-data"),
				Mode:      "rw",
			},
		},
	}

	// 4. Launch workload
	opts := LaunchOptions{
		InstanceID: "custom-instance-01",
		Cmdline:    "console=ttyS0 quiet",
	}

	res, err := eng.Run(ctx, deploy, pkg, opts)
	if err != nil {
		t.Fatalf("eng.Run failed: %v", err)
	}

	if res.ID != "custom-instance-01" {
		t.Errorf("expected ID custom-instance-01, got %s", res.ID)
	}
	if res.Instance == nil {
		t.Fatal("expected non-nil instance")
	}
	if res.Instance.State() != runtime.StateRunning {
		t.Errorf("expected state running, got %s", res.Instance.State())
	}
	if res.VMConfig.MemoryMB != 256 {
		t.Errorf("expected MemoryMB 256, got %d", res.VMConfig.MemoryMB)
	}
	if res.VMConfig.VCPUs != 2 {
		t.Errorf("expected VCPUs 2, got %d", res.VMConfig.VCPUs)
	}
	if res.VMConfig.KernelPath != mockKernel {
		t.Errorf("expected KernelPath %s, got %s", mockKernel, res.VMConfig.KernelPath)
	}
	if res.VMConfig.Cmdline != "console=ttyS0 quiet" {
		t.Errorf("expected Cmdline 'console=ttyS0 quiet', got %s", res.VMConfig.Cmdline)
	}

	// Verify initrd artifact exists
	if _, err := os.Stat(res.AssemblyResult.InitrdPath); os.IsNotExist(err) {
		t.Errorf("initrd artifact was not generated at %s", res.AssemblyResult.InitrdPath)
	}

	// 5. Lifecycle operations
	// Get
	gotInst, err := eng.Get(ctx, res.ID)
	if err != nil || gotInst.ID() != res.ID {
		t.Errorf("eng.Get failed: %v", err)
	}

	// List
	list, err := eng.List(ctx)
	if err != nil || len(list) != 1 || list[0].ID != res.ID {
		t.Errorf("eng.List failed or unexpected summary: %v, %+v", err, list)
	}

	// Stop
	if err := eng.Stop(ctx, res.ID, 5*time.Second); err != nil {
		t.Errorf("eng.Stop failed: %v", err)
	}
	if gotInst.State() != runtime.StateStopped {
		t.Errorf("expected instance stopped after Stop(), got %s", gotInst.State())
	}

	// Delete
	if err := eng.Delete(ctx, res.ID); err != nil {
		t.Errorf("eng.Delete failed: %v", err)
	}
	if _, err := eng.Get(ctx, res.ID); err == nil {
		t.Errorf("expected error getting deleted instance, got nil")
	}
}

func TestEngine_Run_ValidationFailure(t *testing.T) {
	tempDir := t.TempDir()
	casStore, _ := store.NewCASStore(filepath.Join(tempDir, "store"))
	rt := newMockRuntime()

	eng, _ := New(Config{
		Store:      casStore,
		Runtime:    rt,
		KernelPath: filepath.Join(tempDir, "kernel"),
		WorkDir:    tempDir,
	})

	ctx := context.Background()

	// Incompatible: Build requires 512MiB, but Deploy provides 128MiB
	pkg := &manifest.Manifest{
		APIVersion: "vpok.io/v1",
		Kind:       "Manifest",
		Metadata: manifest.BuildMetadata{
			Name:      "test-service",
			Version:   "1.0.0",
			Publisher: "test.org",
		},
		Base: manifest.Base{
			Distribution: "alpine",
			Release:      "3.20",
			Architecture: "amd64",
		},
		BuildSpec: manifest.BuildSpec{
			APIVersion: "vpok.io/v1",
			Kind:       "Build",
			Metadata: manifest.BuildMetadata{
				Name:      "test-service",
				Version:   "1.0.0",
				Publisher: "test.org",
			},
			Base: manifest.Base{
				Distribution: "alpine",
				Release:      "3.20",
				Architecture: "amd64",
			},
			EntryPoint: manifest.EntryPoint{
				Command: []string{"/bin/app"},
			},
			Requires: manifest.Requires{
				Resources: manifest.RequireResources{
					Memory: "512MiB",
					CPUs:   2,
				},
			},
		},
	}

	deploy := &manifest.DeploySpec{
		APIVersion: "vpok.io/v1",
		Kind:       "Deploy",
		Metadata: manifest.DeployMetadata{
			Name: "test-service",
		},
		Package: manifest.DeployPackage{
			Source:  "registry.vpok.io/test-service",
			Version: "1.0.0",
		},
		Resources: manifest.DeployResources{
			Memory: "128MiB",
			CPUs:   1,
		},
	}

	_, err := eng.Run(ctx, deploy, pkg, LaunchOptions{})
	if err == nil {
		t.Fatal("expected compatibility failure error, got nil")
	}
	if !strings.Contains(err.Error(), "memory grant") {
		t.Errorf("expected error message to mention memory grant, got: %v", err)
	}
}
