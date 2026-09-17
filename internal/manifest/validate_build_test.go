package manifest

import (
	"strings"
	"testing"
)

func TestValidateBuild_ValidSpecs(t *testing.T) {
	minimal, err := LoadBuild("testdata/build_minimal.toml")
	if err != nil {
		t.Fatalf("failed to load build_minimal.toml: %v", err)
	}
	if err := ValidateBuild(minimal); err != nil {
		t.Fatalf("expected minimal spec to be valid, got: %v", err)
	}

	complete, err := LoadBuild("testdata/build_complete.toml")
	if err != nil {
		t.Fatalf("failed to load build_complete.toml: %v", err)
	}
	if err := ValidateBuild(complete); err != nil {
		t.Fatalf("expected complete spec to be valid, got: %v", err)
	}
}

func TestValidateBuild_Failures(t *testing.T) {
	baseValid := func() *BuildSpec {
		spec, err := LoadBuild("testdata/build_minimal.toml")
		if err != nil {
			t.Fatalf("failed to load base valid spec: %v", err)
		}
		return spec
	}

	tests := []struct {
		name      string
		mutate    func(s *BuildSpec)
		errSubstr string
	}{
		{
			name:      "nil spec",
			mutate:    nil,
			errSubstr: "cannot be nil",
		},
		{
			name: "invalid apiVersion",
			mutate: func(s *BuildSpec) {
				s.APIVersion = "vpok.io/v2"
			},
			errSubstr: "unsupported apiVersion",
		},
		{
			name: "invalid kind",
			mutate: func(s *BuildSpec) {
				s.Kind = "Deployment"
			},
			errSubstr: "unsupported kind",
		},
		{
			name: "empty name",
			mutate: func(s *BuildSpec) {
				s.Metadata.Name = ""
			},
			errSubstr: "name is required",
		},
		{
			name: "uppercase name",
			mutate: func(s *BuildSpec) {
				s.Metadata.Name = "My-App"
			},
			errSubstr: "DNS-safe pattern",
		},
		{
			name: "name too long",
			mutate: func(s *BuildSpec) {
				s.Metadata.Name = strings.Repeat("a", 64)
			},
			errSubstr: "exceeds 63 characters",
		},
		{
			name: "invalid semver",
			mutate: func(s *BuildSpec) {
				s.Metadata.Version = "1.0"
			},
			errSubstr: "valid semantic version",
		},
		{
			name: "empty publisher",
			mutate: func(s *BuildSpec) {
				s.Metadata.Publisher = ""
			},
			errSubstr: "publisher is required",
		},
		{
			name: "unsupported distribution",
			mutate: func(s *BuildSpec) {
				s.Base.Distribution = "ubuntu"
			},
			errSubstr: "unsupported distribution",
		},
		{
			name: "unsupported architecture",
			mutate: func(s *BuildSpec) {
				s.Base.Architecture = "riscv64"
			},
			errSubstr: "unsupported architecture",
		},
		{
			name: "empty entrypoint command",
			mutate: func(s *BuildSpec) {
				s.EntryPoint.Command = []string{}
			},
			errSubstr: "entrypoint.command must not be empty",
		},
		{
			name: "run step without command",
			mutate: func(s *BuildSpec) {
				s.Build = &BuildConfig{
					Steps: []BuildStep{
						{Action: "run", Command: ""},
					},
				}
			},
			errSubstr: "'run' action requires a command",
		},
		{
			name: "copy step relative to path",
			mutate: func(s *BuildSpec) {
				s.Build = &BuildConfig{
					Steps: []BuildStep{
						{Action: "copy", From: "./server", To: "relative/path"},
					},
				}
			},
			errSubstr: "requires absolute 'to' path",
		},
		{
			name: "copy step escaping build context",
			mutate: func(s *BuildSpec) {
				s.Build = &BuildConfig{
					Steps: []BuildStep{
						{Action: "copy", From: "../outside", To: "/app/outside"},
					},
				}
			},
			errSubstr: "escapes build context",
		},
		{
			name: "unknown build step action",
			mutate: func(s *BuildSpec) {
				s.Build = &BuildConfig{
					Steps: []BuildStep{
						{Action: "install"},
					},
				}
			},
			errSubstr: "unknown action",
		},
		{
			name: "invalid memory format",
			mutate: func(s *BuildSpec) {
				s.Requires.Resources.Memory = "invalid"
			},
			errSubstr: "invalid resources.memory",
		},
		{
			name: "non-positive cpus",
			mutate: func(s *BuildSpec) {
				s.Requires.Resources.CPUs = 0
			},
			errSubstr: "resources.cpus must be positive",
		},
		{
			name: "relative dataDir path",
			mutate: func(s *BuildSpec) {
				s.Requires.DataDirs = []DataDirs{{Path: "var/lib/data"}}
			},
			errSubstr: "must be an absolute path",
		},
		{
			name: "gpu enabled rejected in v1",
			mutate: func(s *BuildSpec) {
				s.Requires.Devices = &RequireDevices{GPU: true}
			},
			errSubstr: "devices.gpu is not supported in v1",
		},
		{
			name: "display enabled rejected in v1",
			mutate: func(s *BuildSpec) {
				s.Requires.Display = &RequireDisplay{Enabled: true}
			},
			errSubstr: "display.enabled is not supported in v1",
		},
		{
			name: "audio enabled rejected in v1",
			mutate: func(s *BuildSpec) {
				s.Requires.Audio = &RequireAudio{Enabled: true}
			},
			errSubstr: "audio.enabled is not supported in v1",
		},
		{
			name: "duplicate service port names",
			mutate: func(s *BuildSpec) {
				s.Service = &BuildService{
					Ports: []ServicePort{
						{Name: "http", ContainerPort: 8080, Protocol: "tcp"},
						{Name: "http", ContainerPort: 8081, Protocol: "tcp"},
					},
				}
			},
			errSubstr: "duplicate service port name",
		},
		{
			name: "duplicate container ports",
			mutate: func(s *BuildSpec) {
				s.Service = &BuildService{
					Ports: []ServicePort{
						{Name: "http", ContainerPort: 8080, Protocol: "tcp"},
						{Name: "api", ContainerPort: 8080, Protocol: "tcp"},
					},
				}
			},
			errSubstr: "duplicate containerPort",
		},
		{
			name: "health check port not declared",
			mutate: func(s *BuildSpec) {
				s.Service = &BuildService{
					Ports: []ServicePort{
						{Name: "http", ContainerPort: 8080, Protocol: "tcp"},
					},
					Health: &HealthCheck{
						Type: "http",
						Path: "/healthz",
						Port: 9090,
					},
				}
			},
			errSubstr: "does not match any declared service port",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var spec *BuildSpec
			if tc.mutate != nil {
				spec = baseValid()
				tc.mutate(spec)
			}
			err := ValidateBuild(spec)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.errSubstr)
			}
			if !strings.Contains(err.Error(), tc.errSubstr) {
				t.Fatalf("expected error containing %q, got: %v", tc.errSubstr, err)
			}
		})
	}
}

func TestParseBytes(t *testing.T) {
	valid := []struct {
		input    string
		expected int64
	}{
		{"64MiB", 64 * 1024 * 1024},
		{"2GiB", 2 * 1024 * 1024 * 1024},
		{"1TiB", 1024 * 1024 * 1024 * 1024},
		{"512KiB", 512 * 1024},
		{"100MB", 100 * 1000 * 1000},
		{"1GB", 1000 * 1000 * 1000},
		{"2048", 2048},
	}

	for _, tc := range valid {
		got, err := ParseBytes(tc.input)
		if err != nil {
			t.Errorf("ParseBytes(%q) unexpected error: %v", tc.input, err)
		}
		if got != tc.expected {
			t.Errorf("ParseBytes(%q) = %d, want %d", tc.input, got, tc.expected)
		}
	}

	invalid := []string{"", "   ", "abc", "64Xib", "-10MiB", "0GiB"}
	for _, in := range invalid {
		if _, err := ParseBytes(in); err == nil {
			t.Errorf("ParseBytes(%q) expected error, got nil", in)
		}
	}
}
