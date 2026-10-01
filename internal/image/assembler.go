package image

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/PeteMadCoder/vpok/internal/agent"
	"github.com/PeteMadCoder/vpok/internal/manifest"
	"github.com/PeteMadCoder/vpok/internal/store"
)

// AssemblyOptions defines parameters required to construct a guest rootfs and initrd.
type AssemblyOptions struct {
	Store           store.Store
	Manifest        *manifest.Manifest
	DeploySpec      *manifest.DeploySpec
	AgentBinaryPath string // Host path to vpok-agent binary
	WorkDir         string // Directory to hold staging rootfs and output initrd
}

// AssemblyResult holds the paths to generated boot artifacts
type AssemblyResult struct {
	RootfsDir  string
	InitrdPath string
	ConfigPath string
}

// Assemble compiles layers, provisions the in-guest agent, writes the agent configuration,
// and produces the bootable initrd.img.
func Assemble(ctx context.Context, opts AssemblyOptions) (*AssemblyResult, error) {
	if opts.Manifest == nil {
		return nil, fmt.Errorf("manifest is required")
	}
	if opts.DeploySpec == nil {
		return nil, fmt.Errorf("deploy spec is required")
	}
	if opts.Store == nil {
		return nil, fmt.Errorf("store is required")
	}

	if opts.WorkDir == "" {
		temp, err := os.MkdirTemp("", "vpok-assemble-*")
		if err != nil {
			return nil, fmt.Errorf("failed to create temporary work directory: %w", err)
		}
		opts.WorkDir = temp
	}

	rootfsDir := filepath.Join(opts.WorkDir, "rootfs")
	if err := os.MkdirAll(rootfsDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create rootfs directory: %w", err)
	}

	// 1. Extract package layers
	var layerDigests []string
	for _, l := range opts.Manifest.Layers {
		layerDigests = append(layerDigests, l.Digest)
	}
	if err := ExtractLayers(ctx, opts.Store, layerDigests, rootfsDir); err != nil {
		return nil, fmt.Errorf("failed to extract package layers: %w", err)
	}

	// 2. Create standard system directories
	standardDirs := []string{"/proc", "/sys", "/dev", "/tmp", "/run", "/run/vpok", "/bin", "/sbin", "/etc"}
	for _, d := range standardDirs {
		if err := os.MkdirAll(filepath.Join(rootfsDir, d), 0755); err != nil {
			return nil, fmt.Errorf("failed to create dir %s: %w", d, err)
		}
	}

	// Create mount point directories for storage volumes
	for _, s := range opts.DeploySpec.Storage {
		if err := os.MkdirAll(filepath.Join(rootfsDir, s.GuestPath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create mount point %s: %w", s.GuestPath, err)
		}
	}

	// 3. Inject vpok-agent binary if available
	if opts.AgentBinaryPath != "" {
		destAgent := filepath.Join(rootfsDir, "bin", "vpok-agent")
		if err := copyExecutable(opts.AgentBinaryPath, destAgent); err != nil {
			return nil, fmt.Errorf("failed to inject vpok-agent: %w", err)
		}
		// Create /init symlink pointing to /bin/vpok-agent for direct initrd boot
		initSymlink := filepath.Join(rootfsDir, "init")
		_ = os.Remove(initSymlink)
		if err := os.Symlink("/bin/vpok-agent", initSymlink); err != nil {
			return nil, fmt.Errorf("failed to create /init symlink: %w", err)
		}
	}

	// 4. Construct and write agent config
	agentCfg := buildAgentConfig(opts.Manifest, opts.DeploySpec)
	configPath := filepath.Join(rootfsDir, "run", "vpok", "config.json")
	cfgBytes, err := json.MarshalIndent(agentCfg, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("failed to marshal agent config: %w", err)
	}

	if err := os.WriteFile(configPath, cfgBytes, 0644); err != nil {
		return nil, fmt.Errorf("failed to write %s: %w", configPath, err)
	}

	// 5. Build initrd.img
	initrdPath := filepath.Join(opts.WorkDir, "initrd.img")
	if err := BuildInitrd(rootfsDir, initrdPath); err != nil {
		return nil, fmt.Errorf("failed to build initrd: %w", err)
	}

	return &AssemblyResult{
		RootfsDir:  rootfsDir,
		InitrdPath: initrdPath,
		ConfigPath: configPath,
	}, nil
}

func buildAgentConfig(m *manifest.Manifest, d *manifest.DeploySpec) *agent.Config {
	var mounts []agent.VolumeMount
	for i, s := range d.Storage {
		mountTag := fmt.Sprintf("fsdev%d", i)
		ro := (s.Mode == "ro")
		opts := "trans=virtio,version=9p2000.L"
		if ro {
			opts += ",ro"
		}
		mounts = append(mounts, agent.VolumeMount{
			Tag:       mountTag,
			GuestPath: s.GuestPath,
			FSType:    "9p",
			Options:   opts,
			ReadOnly:  ro,
		})
	}

	cfg := &agent.Config{
		Hostname:   d.Metadata.Name,
		Entrypoint: m.EntryPoint.Command,
		WorkingDir: m.EntryPoint.WorkingDir,
		Env:        d.Env,
		Secrets:    make(map[string]string),
		Mounts:     mounts,
	}

	if d.Shutdown != nil && d.Shutdown.GracePeriod != "" {
		cfg.GracePeriod = d.Shutdown.GracePeriod
	}

	if m.Service != nil && m.Service.Health != nil {
		cfg.HealthCheck = &agent.HealthCheckConfig{
			Type:     m.Service.Health.Type,
			Port:     m.Service.Health.Port,
			Path:     m.Service.Health.Path,
			Interval: m.Service.Health.Interval,
			Timeout:  m.Service.Health.Timeout,
			Failures: m.Service.Health.Failures,
		}
	}

	return cfg
}

func copyExecutable(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return nil
}
