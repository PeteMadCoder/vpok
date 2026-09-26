package runtime

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestQEMURuntime_Create_Get_List_Delete(t *testing.T) {
	tempDir := t.TempDir()
	rt, err := NewQEMURuntime("fake-qemu", tempDir)
	if err != nil {
		t.Fatalf("failed to initialize runtime: %v", err)
	}

	ctx := context.Background()

	// 1. Create instance
	cfg := VMConfig{
		ID:       "test-vm-1",
		MemoryMB: 512,
		VCPUs:    2,
	}

	inst, err := rt.Create(ctx, cfg)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}

	if inst.ID() != "test-vm-1" {
		t.Errorf("expected ID test-vm-1, got %s", inst.ID())
	}
	if inst.State() != StateCreated {
		t.Errorf("expected state created, got %s", inst.State())
	}

	// Duplicate create should fail
	if _, err := rt.Create(ctx, cfg); err == nil {
		t.Fatalf("expected error on duplicate create, got nil")
	}

	// 2. Get instance
	retrieved, err := rt.Get(ctx, "test-vm-1")
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}
	if retrieved.ID() != "test-vm-1" {
		t.Errorf("expected ID test-vm-1, got %s", retrieved.ID())
	}

	if _, err := rt.Get(ctx, "nonexistent"); err == nil {
		t.Fatalf("expected error for nonexistent ID, got nil")
	}

	// 3. List instances
	list, err := rt.List(ctx)
	if err != nil {
		t.Fatalf("List failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 instance, got %d", len(list))
	}
	if list[0].ID != "test-vm-1" || list[0].MemoryMB != 512 || list[0].VCPUs != 2 {
		t.Errorf("unexpected summary: %+v", list[0])
	}

	// 4. Delete instance
	if err := rt.Delete(ctx, "test-vm-1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	listAfter, err := rt.List(ctx)
	if err != nil {
		t.Fatalf("List after delete failed: %v", err)
	}
	if len(listAfter) != 0 {
		t.Errorf("expected 0 instances after delete, got %d", len(listAfter))
	}
}

func TestQEMUInstance_BuildArgs(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "console.log")

	cfg := VMConfig{
		ID:         "vm-args-test",
		MemoryMB:   1024,
		VCPUs:      4,
		KernelPath: "/boot/vmlinuz",
		InitrdPath: "/boot/initrd.img",
		Cmdline:    "console=ttyS0 root=/dev/vda",
		LogFile:    logPath,
		RootDrive: &Drive{
			Path:     "/store/rootfs.raw",
			ReadOnly: true,
			Format:   "raw",
		},
		Drives: []Drive{
			{
				Path:     "/store/data.qcow2",
				ReadOnly: false,
				Format:   "qcow2",
			},
		},
		SharedDirs: []SharedDir{
			{
				Tag:      "vpok_share",
				HostPath: "/host/share",
				ReadOnly: true,
			},
		},
	}

	inst := &QEMUInstance{
		config:     cfg,
		binaryPath: "qemu-system-x86_64",
		instDir:    tempDir,
		state:      StateCreated,
	}

	args, err := inst.BuildArgs()
	if err != nil {
		t.Fatalf("BuildArgs failed: %v", err)
	}

	argsStr := strings.Join(args, " ")

	// Verify required flags
	expectedSubstrings := []string{
		"-m 1024",
		"-smp 4",
		"-kernel /boot/vmlinuz",
		"-initrd /boot/initrd.img",
		"-append console=ttyS0 root=/dev/vda",
		"-drive file=/store/rootfs.raw,if=virtio,format=raw,readonly=on",
		"-drive file=/store/data.qcow2,if=virtio,format=qcow2",
		"-fsdev local,security_model=none,id=fsdev0,path=/host/share,readonly=on",
		"-device virtio-9p-pci,fsdev=fsdev0,mount_tag=vpok_share",
		"-serial file:" + logPath,
	}

	for _, sub := range expectedSubstrings {
		if !strings.Contains(argsStr, sub) {
			t.Errorf("expected args to contain %q, but got: %s", sub, argsStr)
		}
	}
}

func TestQEMUInstance_ExecutionLifecycle(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "console.log")

	// Use 'sh' as a mock hypervisor process
	// It writes output to log file via standard redirection in script and handles TERM
	mockScript := filepath.Join(tempDir, "mock_qemu.sh")
	scriptContent := `#!/bin/sh
trap 'exit 0' TERM
echo "MOCK VM STARTED" >> "` + logPath + `"
while true; do
	sleep 0.05
done
`
	if err := os.WriteFile(mockScript, []byte(scriptContent), 0755); err != nil {
		t.Fatalf("failed to write mock script: %v", err)
	}

	cfg := VMConfig{
		ID:       "vm-lifecycle",
		MemoryMB: 256,
		VCPUs:    1,
		LogFile:  logPath,
	}

	inst := &QEMUInstance{
		config:     cfg,
		binaryPath: mockScript,
		instDir:    tempDir,
		state:      StateCreated,
	}

	ctx := context.Background()

	// 1. Start instance
	if err := inst.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if inst.State() != StateRunning {
		t.Errorf("expected state running, got %s", inst.State())
	}
	if inst.PID() <= 0 {
		t.Errorf("expected positive PID, got %d", inst.PID())
	}

	// 2. Allow process to log
	time.Sleep(100 * time.Millisecond)

	logReader, err := inst.Logs()
	if err != nil {
		t.Fatalf("Logs failed: %v", err)
	}
	logBytes, err := io.ReadAll(logReader)
	_ = logReader.Close()
	if err != nil {
		t.Fatalf("failed to read logs: %v", err)
	}

	if !strings.Contains(string(logBytes), "MOCK VM STARTED") {
		t.Errorf("expected log to contain 'MOCK VM STARTED', got %q", string(logBytes))
	}

	// 3. Stop instance gracefully
	if err := inst.Stop(ctx, 1*time.Second); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	status, err := inst.Wait(ctx)
	if err != nil {
		t.Fatalf("Wait failed: %v", err)
	}

	if status.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", status.ExitCode)
	}
	if inst.State() != StateStopped {
		t.Errorf("expected state stopped, got %s", inst.State())
	}
}
