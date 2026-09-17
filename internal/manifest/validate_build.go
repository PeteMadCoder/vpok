package manifest

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var (
	nameRegex   = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`)
	semverRegex = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)
)

// ValidateBuild validates a BuildSpec againts all v1 specification rules.
func ValidateBuild(spec *BuildSpec) error {
	if spec == nil {
		return errors.New("build spec cannot be nil")
	}

	if spec.APIVersion != "vpok.io/v1" {
		return fmt.Errorf("unsupported apiVersion %q, expected %q", spec.APIVersion, "vpok.io/v1")
	}

	if spec.Kind != "Build" {
		return fmt.Errorf("unsupported kind %q, expected %q", spec.Kind, "Build")
	}

	// Metadata validation
	if err := validateBuildMetadata(&spec.Metadata); err != nil {
		return fmt.Errorf("metadata: %w", err)
	}

	// Base validation
	if err := validateBase(&spec.Base); err != nil {
		return fmt.Errorf("base: %w", err)
	}

	// Build steps validation
	if spec.Build != nil {
		if err := validateBuildSteps(spec.Build.Steps); err != nil {
			return fmt.Errorf("build.steps: %w", err)
		}
	}

	// Entrypoint validation
	if len(spec.EntryPoint.Command) == 0 || strings.TrimSpace(spec.EntryPoint.Command[0]) == "" {
		return errors.New("entrypoint.command must not be empty")
	}

	// Requirements validation
	if err := validateBuildRequirements(&spec.Requires); err != nil {
		return fmt.Errorf("requires: %w", err)
	}

	// Service validation
	if spec.Service != nil {
		if err := validateBuildService(spec.Service); err != nil {
			return fmt.Errorf("service: %w", err)
		}
	}

	return nil
}

func validateBuildMetadata(m *BuildMetadata) error {
	if m.Name == "" {
		return errors.New("name is required")
	}
	if len(m.Name) > 63 {
		return fmt.Errorf("name %q exceeds 63 characters", m.Name)
	}
	if !nameRegex.MatchString(m.Name) {
		return fmt.Errorf("name %q must match DNS-safe pattern [a-z0-9]([a-z0-9-]*[a-z0-9])?", m.Name)
	}
	if m.Version == "" {
		return errors.New("version is required")
	}
	if !semverRegex.MatchString(m.Version) {
		return fmt.Errorf("version %q must be a valid semantic version (MAJOR.MINOR.PATCH)", m.Version)
	}
	if m.Publisher == "" {
		return errors.New("publisher is required")
	}
	return nil
}

func validateBase(b *Base) error {
	if b.Distribution != "alpine" {
		return fmt.Errorf("unsupported distribution %q (only 'alpine' is supported in v1)", b.Distribution)
	}
	if b.Release == "" {
		return errors.New("release is required")
	}
	if b.Architecture != "amd64" && b.Architecture != "arm64" {
		return fmt.Errorf("unsupported architecture %q (only 'amd64' and 'arm64' are supported", b.Architecture)
	}

	return nil
}

func validateBuildSteps(steps []BuildStep) error {
	for i, step := range steps {
		switch step.Action {
		case "run":
			if step.Command == "" {
				return fmt.Errorf("step %d: 'run' action requires a command", i)
			}
			if step.From != "" || step.To != "" || step.Mode != "" || len(step.Vars) > 0 {
				return fmt.Errorf("step %d: 'run' action has illegal fields", i)
			}

		case "copy":
			if step.From == "" {
				return fmt.Errorf("step %d: 'copy' action requires 'from'", i)
			}
			if step.To == "" || !filepath.IsAbs(step.To) {
				return fmt.Errorf("step %d: 'copy' action requires absolute 'to' path", i)
			}
			cleanedFrom := filepath.Clean(step.From)
			if strings.HasPrefix(cleanedFrom, "..") || filepath.IsAbs(cleanedFrom) {
				return fmt.Errorf("step %d: 'copy.from' %q escapes build context", i, step.From)
			}
			if step.Command != "" || len(step.Vars) > 0 {
				return fmt.Errorf("step %d: 'copy' action has illegal fields", i)
			}

		case "env":
			if len(step.Vars) == 0 {
				return fmt.Errorf("step %d: 'env' action requires 'vars' map", i)
			}
			if step.Command != "" || step.From != "" || step.To != "" || step.Mode != "" {
				return fmt.Errorf("step %d: 'env' action has illegal fields", i)
			}
		default:
			return fmt.Errorf("step %d: unknown action %q", i, step.Action)
		}
	}

	return nil
}

func validateBuildRequirements(r *Requires) error {
	if r.Resources.Memory == "" {
		return errors.New("resources.memory is required")
	}
	if _, err := ParseBytes(r.Resources.Memory); err != nil {
		return fmt.Errorf("invalid resources.memory: %w", err)
	}
	if r.Resources.CPUs <= 0 {
		return fmt.Errorf("resources.cpus must be positive, got %d", r.Resources.CPUs)
	}

	for i, d := range r.DataDirs {
		if !filepath.IsAbs(d.Path) {
			return fmt.Errorf("dataDirs[%d].path %q must be an absolute path", i, d.Path)
		}
	}

	if r.Devices != nil && r.Devices.GPU {
		return errors.New("devices.gpu is not supported in v1")
	}
	if r.Display != nil && r.Display.Enabled {
		return errors.New("display.enabled is not supported in v1")
	}
	if r.Audio != nil && r.Audio.Enabled {
		return errors.New("audio.enabled is not supported in v1")
	}

	return nil
}

func validateBuildService(s *BuildService) error {
	portNames := make(map[string]bool)
	containerPorts := make(map[int]bool)

	for i, p := range s.Ports {
		if p.Name == "" || !nameRegex.MatchString(p.Name) {
			return fmt.Errorf("ports[%d].name %q must be DNS-safe", i, p.Name)
		}
		if portNames[p.Name] {
			return fmt.Errorf("duplicate service port name %q", p.Name)
		}
		portNames[p.Name] = true

		if p.ContainerPort < 1 || p.ContainerPort > 65535 {
			return fmt.Errorf("invalid containerPort %d for port %q", p.ContainerPort, p.Name)
		}
		if containerPorts[p.ContainerPort] {
			return fmt.Errorf("duplicate containerPort %d", p.ContainerPort)
		}
		containerPorts[p.ContainerPort] = true

		proto := strings.ToLower(p.Protocol)
		if proto != "tcp" && proto != "udp" {
			return fmt.Errorf("invalid protocol %q for port %q (must be 'tcp' or 'udp')", p.Protocol, p.Name)
		}
	}

	if s.Health != nil {
		if s.Health.Type != "http" && s.Health.Type != "tcp" {
			return fmt.Errorf("service.health.type must be 'http' or 'tcp', got %q", s.Health.Type)
		}
		if s.Health.Type == "http" && s.Health.Path == "" {
			return errors.New("service.health.path is required for http health checks")
		}
		if !containerPorts[s.Health.Port] {
			return fmt.Errorf("service.health.port %d does not match any declared service port", s.Health.Port)
		}
	}

	if s.Readiness != nil {
		if s.Readiness.Type != "http" && s.Readiness.Type != "tcp" {
			return fmt.Errorf("service.readiness.type must be 'http' or 'tcp', got %q", s.Readiness.Type)
		}
		if s.Readiness.Type == "http" && s.Readiness.Path == "" {
			return errors.New("service.readiness.path is required for http readiness checks")
		}
		if !containerPorts[s.Readiness.Port] {
			return fmt.Errorf("service.readiness.port %d does not match any declared service port", s.Readiness.Port)
		}
	}

	return nil
}

// ParseBytes converts size strings like "64MiB", "2BiB", "100MB" to byte counts.
func ParseBytes(s string) (int64, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, errors.New("empty byte size")
	}

	units := []struct {
		suffix string
		factor int64
	}{
		{"TiB", 1024 * 1024 * 1024 * 1024},
		{"GiB", 1024 * 1024 * 1024},
		{"MiB", 1024 * 1024},
		{"KiB", 1024},
		{"TB", 1000 * 1000 * 1000 * 1000},
		{"GB", 1000 * 1000 * 1000},
		{"MB", 1000 * 1000},
		{"KB", 1000},
		{"B", 1},
	}

	for _, u := range units {
		if strings.HasSuffix(s, u.suffix) {
			numPart := strings.TrimSpace(strings.TrimSuffix(s, u.suffix))
			val, err := strconv.ParseInt(numPart, 10, 64)
			if err != nil || val <= 0 {
				return 0, fmt.Errorf("invalid number %q in byte size %q", numPart, s)
			}
			return val * u.factor, nil
		}
	}

	val, err := strconv.ParseInt(s, 10, 64)
	if err != nil || val <= 0 {
		return 0, fmt.Errorf("invalid byte size %q", s)
	}
	return val, nil
}
