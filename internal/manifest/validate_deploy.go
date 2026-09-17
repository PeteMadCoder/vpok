package manifest

import (
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
)

// ValidateDeploy validates standalone DeploySpec fields and constraints.
func ValidateDeploy(spec *DeploySpec) error {
	if spec == nil {
		return errors.New("deploy spec cannot be nil")
	}

	if spec.APIVersion != "vpok.io/v1" {
		return fmt.Errorf("unsupported apiVersion %q, expected %q", spec.APIVersion, "vpok.io/v1")
	}

	if spec.Kind != "Deploy" {
		return fmt.Errorf("unsupported kind %q, expected %q", spec.Kind, "Deploy")
	}

	// Metadata
	if spec.Metadata.Name == "" {
		return errors.New("metadata.name is required")
	}
	if len(spec.Metadata.Name) > 63 {
		return fmt.Errorf("metadata.name %q exceeds 63 characters", spec.Metadata.Name)
	}
	if !nameRegex.MatchString(spec.Metadata.Name) {
		return fmt.Errorf("metadata.name %q must match DNS-safe pattern", spec.Metadata.Name)
	}

	// Package reference
	if spec.Package.Source == "" {
		return errors.New("package.source is required")
	}
	if spec.Package.Version == "" && spec.Package.Digest == "" {
		return errors.New("at least one of package.version or package.digest must be set")
	}
	if spec.Package.Verify != nil && spec.Package.Verify.Signature != "" {
		sig := spec.Package.Verify.Signature
		if sig != "required" && sig != "preferred" && sig != "off" {
			return fmt.Errorf("invalid package.verify.signature %q (must be 'required', 'preferred', or 'off')", sig)
		}
	}

	// Resources
	if spec.Resources.Memory == "" {
		return errors.New("resources.memory is required")
	}
	if _, err := ParseBytes(spec.Resources.Memory); err != nil {
		return fmt.Errorf("invalid resources.memory: %w", err)
	}
	if spec.Resources.CPUs <= 0 {
		return fmt.Errorf("resources.cpus must be positive, got %d", spec.Resources.CPUs)
	}
	if spec.Resources.PIDs < 0 {
		return errors.New("resources.pids must not be negative")
	}

	// Storage
	storageNames := make(map[string]bool)
	guestPaths := make(map[string]bool)
	for i, s := range spec.Storage {
		if s.Name == "" {
			return fmt.Errorf("storage[%d].name is required", i)
		}
		if storageNames[s.Name] {
			return fmt.Errorf("duplicate storage name %q", s.Name)
		}
		storageNames[s.Name] = true

		if s.GuestPath == "" || !filepath.IsAbs(s.GuestPath) {
			return fmt.Errorf("storage[%d].guestPath %q must be an absolute path", i, s.GuestPath)
		}
		if guestPaths[s.GuestPath] {
			return fmt.Errorf("duplicate storage guestPath %q", s.GuestPath)
		}
		guestPaths[s.GuestPath] = true

		switch s.Type {
		case "volume":
			if s.Size == "" {
				return fmt.Errorf("storage[%d] volume requires 'size'", i)
			}
			if _, err := ParseBytes(s.Size); err != nil {
				return fmt.Errorf("storage[%d] invalid volume size: %w", i, err)
			}
			if s.HostPath != "" || s.Mode != "" {
				return fmt.Errorf("storage[%d] volume cannot have hostPath or mode", i)
			}
		case "hostPath":
			if s.HostPath == "" {
				return fmt.Errorf("storage[%d] hostPath requires 'hostPath'", i)
			}
			if strings.Contains(s.HostPath, "..") {
				return fmt.Errorf("storage[%d] hostPath cannot contain '..' segments", i)
			}
			if s.Mode != "ro" && s.Mode != "rw" {
				return fmt.Errorf("storage[%d] mode must be 'ro' or 'rw', got %q", i, s.Mode)
			}
			if s.Size != "" || s.Persistent != nil {
				return fmt.Errorf("storage[%d] hostPath cannot have size or persistent flags", i)
			}
		default:
			return fmt.Errorf("storage[%d] unknown type %q (must be 'volume' or 'hostPath')", i, s.Type)
		}
	}

	// Network
	if spec.Network != nil {
		mode := spec.Network.Mode
		if mode != "none" && mode != "nat" && mode != "bridge" {
			return fmt.Errorf("invalid network.mode %q (must be 'none', 'nat', or 'bridge')", mode)
		}
		for i, a := range spec.Network.Allow {
			hasHost := a.Host != ""
			hasCIDR := a.CIDR != ""
			if (hasHost && hasCIDR) || (!hasHost && !hasCIDR) {
				return fmt.Errorf("network.allow[%d] must specify exactly one of 'host' or 'cidr'", i)
			}
			if hasCIDR {
				if _, _, err := net.ParseCIDR(a.CIDR); err != nil {
					return fmt.Errorf("network.allow[%d] invalid CIDR %q: %w", i, a.CIDR, err)
				}
			}
			if a.Port < 1 || a.Port > 65535 {
				return fmt.Errorf("network.allow[%d] invalid port %d", i, a.Port)
			}
		}
	}

	// Devices, Display, Audio
	if spec.Devices != nil && spec.Devices.GPU {
		return errors.New("devices.gpu is not supported in v1")
	}
	if spec.Display != nil && spec.Display.Enabled {
		return errors.New("display.enabled is not supported in v1")
	}
	if spec.Audio != nil && spec.Audio.Enabled {
		return errors.New("audio.enabled is not supported in v1")
	}

	// Secrets
	secretNames := make(map[string]bool)
	for i, sec := range spec.Secrets {
		if sec.Name == "" {
			return fmt.Errorf("secrets[%d].name is required", i)
		}
		if secretNames[sec.Name] {
			return fmt.Errorf("duplicate secret name %q", sec.Name)
		}
		secretNames[sec.Name] = true

		if !strings.HasPrefix(sec.Source, "env:") && !strings.HasPrefix(sec.Source, "file:") {
			return fmt.Errorf("secrets[%d].source %q must start with 'env:' or 'file:'", i, sec.Source)
		}
	}

	// Restart policy
	if spec.Restart != nil && spec.Restart.Policy != "" {
		p := spec.Restart.Policy
		if p != "never" && p != "on-failure" && p != "always" {
			return fmt.Errorf("invalid restart.policy %q (must be 'never', 'on-failure', or 'always')", p)
		}
	}

	// License
	if spec.License != nil {
		if spec.License.Document == "" {
			return errors.New("license.document is required when license is configured")
		}
		if spec.License.Activation == "online" && spec.License.Server == "" {
			return errors.New("license.server is required when activation is 'online'")
		}
	}

	return nil
}

// ValidateCompatibility validates that a DeploySpec satisfies all requirements of a package Manifest.
func ValidateCompatibility(deploy *DeploySpec, pkg *Manifest) error {
	if deploy == nil {
		return errors.New("deploy spec cannot be nil")
	}
	if pkg == nil {
		return errors.New("package manifest cannot be nil")
	}

	if err := ValidateDeploy(deploy); err != nil {
		return fmt.Errorf("invalid deploy spec: %w", err)
	}

	// 1. Data directories: Every required dataDir must be mounted in deploy.storage
	mountedPaths := make(map[string]bool)
	for _, s := range deploy.Storage {
		mountedPaths[s.GuestPath] = true
	}
	for _, reqDir := range pkg.Requires.DataDirs {
		if !mountedPaths[reqDir.Path] {
			return fmt.Errorf("missing required storage mount for dataDir %q", reqDir.Path)
		}
	}

	// 2. Memory grant: deploy >= manifest requirement
	deployMem, err := ParseBytes(deploy.Resources.Memory)
	if err != nil {
		return fmt.Errorf("invalid deploy memory: %w", err)
	}
	reqMem, err := ParseBytes(pkg.Requires.Resources.Memory)
	if err != nil {
		return fmt.Errorf("invalid manifest required memory: %w", err)
	}
	if deployMem < reqMem {
		return fmt.Errorf("insufficient memory granted: deploy grants %s (%d bytes), manifest requires %s (%d bytes)",
			deploy.Resources.Memory, deployMem, pkg.Requires.Resources.Memory, reqMem)
	}

	// 3. CPU grant: deploy >= manifest requirement
	if deploy.Resources.CPUs < pkg.Requires.Resources.CPUs {
		return fmt.Errorf("insufficient cpus granted: deploy grants %d, manifest requires %d",
			deploy.Resources.CPUs, pkg.Requires.Resources.CPUs)
	}

	// 4. Network compatibility
	deployNetMode := "none"
	if deploy.Network != nil && deploy.Network.Mode != "" {
		deployNetMode = deploy.Network.Mode
	}

	if !pkg.Requires.Network.Outbound {
		if deployNetMode != "none" {
			return fmt.Errorf("package requires outbound=false; deploy network mode must be 'none', got %q", deployNetMode)
		}
	} else {
		if deployNetMode != "nat" && deployNetMode != "bridge" {
			return fmt.Errorf("package requires outbound=true; deploy network mode must be 'nat' or 'bridge', got %q", deployNetMode)
		}
	}

	// 5. Device grants: deploy can only grant what manifest declares
	if deploy.Devices != nil {
		if pkg.Requires.Devices == nil {
			if deploy.Devices.Webcam || deploy.Devices.Microphone || deploy.Devices.GPU {
				return errors.New("deploy grants devices but package manifest declares no device requirements")
			}
		} else {
			if deploy.Devices.Webcam && !pkg.Requires.Devices.Webcam {
				return errors.New("deploy grants webcam, but package does not declare webcam requirement")
			}
			if deploy.Devices.Microphone && !pkg.Requires.Devices.Microphone {
				return errors.New("deploy grants microphone, but package does not declare microphone requirement")
			}
		}
	}

	// 6. Display grant
	deployDisplay := deploy.Display != nil && deploy.Display.Enabled
	pkgDisplay := pkg.Requires.Display != nil && pkg.Requires.Display.Enabled
	if deployDisplay && !pkgDisplay {
		return errors.New("deploy enables display, but package does not declare display requirement")
	}

	// 7. Audio grant
	deployAudio := deploy.Audio != nil && deploy.Audio.Enabled
	pkgAudio := pkg.Requires.Audio != nil && pkg.Requires.Audio.Enabled
	if deployAudio && !pkgAudio {
		return errors.New("deploy enables audio, but package does not declare audio requirement")
	}

	// 8. Service port publishing: every published port must exist in the manifest
	if deploy.Service != nil && len(deploy.Service.Publish) > 0 {
		if pkg.Service == nil || len(pkg.Service.Ports) == 0 {
			return errors.New("deploy configures service port publishing, but package declares no service ports")
		}
		declaredPorts := make(map[string]bool)
		for _, p := range pkg.Service.Ports {
			declaredPorts[p.Name] = true
		}
		for _, pub := range deploy.Service.Publish {
			if !declaredPorts[pub.Name] {
				return fmt.Errorf("deploy publishes port %q which is not declared in package service ports", pub.Name)
			}
		}
	}

	return nil
}
