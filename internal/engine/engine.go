package engine

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/PeteMadCoder/vpok/internal/image"
	"github.com/PeteMadCoder/vpok/internal/manifest"
	"github.com/PeteMadCoder/vpok/internal/runtime"
	"github.com/PeteMadCoder/vpok/internal/store"
)

const (
	defaultCmdline = "console=ttyS0 root=/dev/ram0 panic=-1 quiet"
	defaultMemMB   = 256
	defaultVCPUs   = 1
)

// Config configures the Workload Execution Engine.
type Config struct {
	Store           store.Store
	Runtime         runtime.Runtime
	KernelPath      string // Optional: explicit path to guest kernel (vmlinuz/vmlinux)
	AgentBinaryPath string // Optional: explicit path to host vpok-agent binary
	WorkDir         string // Base working directory for instances and state (default: ~/.vpok/runtime)
}

// Engine coordinates image assembly and hypervisor execution.
type Engine struct {
	store           store.Store
	runtime         runtime.Runtime
	kernelPath      string
	agentBinaryPath string
	workDir         string
}

// LaunchOptions provides optional overrides when launching a workload.
type LaunchOptions struct {
	InstanceID string // Optional: custom instance ID (auto-generated if omitted)
	Cmdline    string // Optional: kernel cmdline override
}

// LaunchResult constains metadata and handles for a successfully launched workload
type LaunchResult struct {
	ID             string
	Instance       runtime.Instance
	AssemblyResult *image.AssemblyResult
	VMConfig       runtime.VMConfig
}

// New creates and initializes a new Workload Execution Engine.
func New(cfg Config) (*Engine, error) {
	if cfg.Store == nil {
		return nil, errors.New("store is required")
	}
	if cfg.Runtime == nil {
		return nil, errors.New("runtime backend is required")
	}

	workDir := cfg.WorkDir
	if workDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			workDir = filepath.Join(".", ".vpok", "runtime")
		} else {
			workDir = filepath.Join(home, ".vpok", "runtime")
		}
	}

	if err := os.MkdirAll(workDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create engine work directory %s: %w", workDir, err)
	}

	return &Engine{
		store:           cfg.Store,
		runtime:         cfg.Runtime,
		kernelPath:      cfg.KernelPath,
		agentBinaryPath: cfg.AgentBinaryPath,
		workDir:         workDir,
	}, nil
}

// Run orchestrates image assembly, VM configuration, and hypervisor boot.
func (e *Engine) Run(ctx context.Context, deploy *manifest.DeploySpec, pkg *manifest.Manifest, opts LaunchOptions) (*LaunchResult, error) {
	if deploy == nil {
		return nil, errors.New("deploy spec is required")
	}
	if pkg == nil {
		return nil, errors.New("package manifest is required")
	}

	// 1. Verify deployment spec compatibility against publisher requirements
	if err := manifest.ValidateCompatibilityWithBuild(deploy, &pkg.BuildSpec); err != nil {
		return nil, fmt.Errorf("deploy specification fails package requirements: %w", err)
	}

	// 2. Resolve instance ID
	instanceID := opts.InstanceID
	if instanceID == "" {
		randomSuffix, err := randomHex(4)
		if err != nil {
			randomSuffix = fmt.Sprintf("%d", time.Now().UnixNano()%10000)
		}
		instanceID = fmt.Sprintf("%s-%s", deploy.Metadata.Name, randomSuffix)
	}

	instanceDir := filepath.Join(e.workDir, "instances", instanceID)
	if err := os.MkdirAll(instanceDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create instance dir: %w", err)
	}

	// 3. Locate guest kernel and agent binary
	kernelPath, err := e.resolveKernel()
	if err != nil {
		return nil, err
	}

	agentPath, err := e.resolveAgentBinary()
	if err != nil {
		return nil, err
	}

	// 4. Assemble rootfs and build bootable initrd.img
	assemblyOpts := image.AssemblyOptions{
		Store:           e.store,
		Manifest:        pkg,
		DeploySpec:      deploy,
		AgentBinaryPath: agentPath,
		WorkDir:         filepath.Join(instanceDir, "image"),
	}

	assemblyRes, err := image.Assemble(ctx, assemblyOpts)
	if err != nil {
		return nil, fmt.Errorf("image assembly failed: %w", err)
	}

	// 5. Parse and assign computing resources
	memMB, err := parseMemoryMB(deploy.Resources.Memory)
	if err != nil {
		memMB = defaultMemMB
	}

	vcpus := deploy.Resources.CPUs
	if vcpus <= 0 {
		vcpus = defaultVCPUs
	}

	// 6. Map storage declarations to shared 9p directories
	sharedDirs, err := e.resolveStorage(deploy, instanceDir)
	if err != nil {
		return nil, fmt.Errorf("failed to configure storage: %w", err)
	}

	cmdline := opts.Cmdline
	if cmdline == "" {
		cmdline = defaultCmdline
	}

	logFile := filepath.Join(instanceDir, "console.log")

	// 7. Construct hypervisor configuration
	vmConfig := runtime.VMConfig{
		ID:          instanceID,
		MemoryMB:    memMB,
		VCPUs:       vcpus,
		KernelPath:  kernelPath,
		InitrdPath:  assemblyRes.InitrdPath,
		Cmdline:     cmdline,
		SharedDirs:  sharedDirs,
		LogFile:     logFile,
		AgentConfig: nil, // Embedded directly into roofts by image.Assemble
	}

	// 8. Create and start VM instance
	inst, err := e.runtime.Create(ctx, vmConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to register VM instance: %w", err)
	}

	if err := inst.Start(ctx); err != nil {
		return nil, fmt.Errorf("failed to boot VM instance: %w", err)
	}

	return &LaunchResult{
		ID:             instanceID,
		Instance:       inst,
		AssemblyResult: assemblyRes,
		VMConfig:       vmConfig,
	}, nil
}

// Stop initiates an orderly shutdown of an active workload.
func (e *Engine) Stop(ctx context.Context, instanceID string, gracePeriod time.Duration) error {
	inst, err := e.runtime.Get(ctx, instanceID)
	if err != nil {
		return err
	}
	return inst.Stop(ctx, gracePeriod)
}

// Get retrieves an instance by ID.
func (e *Engine) Get(ctx context.Context, instanceID string) (runtime.Instance, error) {
	return e.runtime.Get(ctx, instanceID)
}

// List returns a list of all tracked instances.
func (e *Engine) List(ctx context.Context) ([]runtime.InstanceSummary, error) {
	return e.runtime.List(ctx)
}

// Delete stops (if needed) and cleans up an instance.
func (e *Engine) Delete(ctx context.Context, instanceID string) error {
	return e.runtime.Delete(ctx, instanceID)
}

// resolveKernel discovers the guest kernel path from config, env or host locations.
func (e *Engine) resolveKernel() (string, error) {
	if e.kernelPath != "" {
		if _, err := os.Stat(e.kernelPath); err == nil {
			return e.kernelPath, nil
		}
		return "", fmt.Errorf("specified kernel not found: %s", e.kernelPath)
	}

	if envPath := os.Getenv("VPOK_KERNEL"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath, nil
		}
		return "", fmt.Errorf("VPOK_KERNEL path not found: %s", envPath)
	}

	// Probe host default kernel candidates
	candidates := []string{
		"/boot/vmlinuz",
		"/boot/vmlinux",
	}

	// Check kernel matching current running host kernel release
	if releaseBytes, err :=
		os.ReadFile("/proc/sys/kernel/osrelease"); err == nil {
		rel := strings.TrimSpace(string(releaseBytes))
		candidates = append([]string{
			fmt.Sprintf("/boot/vmlinuz-%s", rel),
			fmt.Sprintf("/boot/vmlinux-%s", rel),
		}, candidates...)
	}

	// Check user local vpok cache
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home,
			".vpok", "kernel", "vmlinuz"))
	}

	// Match glob patterns for any installed kernels
	if matches, err := filepath.Glob("/boot/vmlinuz-*"); err == nil && len(matches) > 0 {
		candidates = append(candidates, matches...)
	}

	for _, c := range candidates {
		if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
			return c, nil
		}
	}

	return "", errors.New("guest kernel not found; please specify a kernel via EngineConfig or VPOK_KERNEL env var")
}

// resolveAgentBinary locates the vpok-agent binary.
func (e *Engine) resolveAgentBinary() (string, error) {
	if e.agentBinaryPath != "" {
		if _, err := os.Stat(e.agentBinaryPath); err == nil {
			return e.agentBinaryPath, nil
		}
		return "", fmt.Errorf("specified vpok-agent not found: %s", e.agentBinaryPath)
	}

	if envPath := os.Getenv("VPOK_AGENT_PATH"); envPath != "" {
		if _, err := os.Stat(envPath); err == nil {
			return envPath, nil
		}
		return "", fmt.Errorf("VPOK_AGENT_PATH not found: %s", envPath)
	}

	// Check current PATH
	if p, err := exec.LookPath("vpok-agent"); err == nil {
		return p, nil
	}

	// Check sibling directory to current executable
	if self, err := os.Executable(); err == nil {
		sibling := filepath.Join(filepath.Dir(self), "vpok-agent")
		if _, err := os.Stat(sibling); err == nil {
			return sibling, nil
		}
	}

	// Check standard user directory ~/.vpok/bin/vpok-agent
	if home, err := os.UserHomeDir(); err == nil {
		userBin := filepath.Join(home, ".vpok", "bin", "vpok-agent")
		if _, err := os.Stat(userBin); err == nil {
			return userBin, nil
		}
	}

	return "", errors.New("vpok-agent binary not found; please ensure it is built and in PATH, or specify via VPOK_AGENT_PATH")
}

// resolveStorage maps DeploySpec storage entries to 9p SharedDir objects.
func (e *Engine) resolveStorage(deploy *manifest.DeploySpec, instanceDir string) ([]runtime.SharedDir, error) {
	var sharedDirs []runtime.SharedDir

	for i, s := range deploy.Storage {
		mountTag := fmt.Sprintf("fsdev%d", i)
		var hostDir string

		switch s.Type {
		case "hostPath":
			expanded := expandUser(s.HostPath)
			abs, err := filepath.Abs(expanded)
			if err != nil {
				return nil, fmt.Errorf("invalid host path %s: %w", s.HostPath, err)
			}
			hostDir = abs

		case "volume":
			isPersistent := (s.Persistent != nil && *s.Persistent)
			if isPersistent {
				hostDir = filepath.Join(e.workDir, "volumes", s.Name)
			} else {
				hostDir = filepath.Join(instanceDir, "volumes", s.Name)
			}

		default:
			return nil, fmt.Errorf("unsupported storage type: %s", s.Type)
		}

		if err := os.MkdirAll(hostDir, 0755); err != nil {
			return nil, fmt.Errorf("failed to create host storage directory %s: %w", hostDir, err)
		}

		sharedDirs = append(sharedDirs, runtime.SharedDir{
			Tag:      mountTag,
			HostPath: hostDir,
			ReadOnly: (s.Mode == "ro"),
		})
	}

	return sharedDirs, nil
}

func parseMemoryMB(memStr string) (int, error) {
	if memStr == "" {
		return defaultMemMB, nil
	}
	bytes, err := manifest.ParseBytes(memStr)
	if err != nil {
		return 0, err
	}
	mb := int(bytes / (1024 * 1024))
	if mb <= 0 {
		mb = defaultMemMB
	}
	return mb, nil
}

func expandUser(path string) string {
	if !strings.HasPrefix(path, "~/") && path != "~" {
		return os.ExpandEnv(path)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return os.ExpandEnv(path)
	}
	if path == "~" {
		return home
	}
	return filepath.Join(home, path[2:])
}

func randomHex(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
