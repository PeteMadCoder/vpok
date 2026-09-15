package manifest

import (
	"strings"
	"testing"
)

func TestLoadBuildMinimal(t *testing.T) {
	spec, err := LoadBuild("testdata/build_minimal.toml")
	if err != nil {
		t.Fatalf("expected no error loading build_minimal.toml, got: %v", err)
	}

	if spec.APIVersion != "vpok.io/v1" {
		t.Errorf("expected apiVersion 'vpok.io/v1', got '%s'", spec.APIVersion)
	}
	if spec.Kind != "Build" {
		t.Errorf("expected kind 'Build', got '%s'", spec.Kind)
	}
	if spec.Metadata.Name != "hello" {
		t.Errorf("expected metadata.name 'hello', got '%s'", spec.Metadata.Name)
	}
	if spec.Metadata.Version != "0.1.0" {
		t.Errorf("expected metadata.version '0.1.0', got '%s'", spec.Metadata.Version)
	}
	if spec.Metadata.Publisher != "example.org" {
		t.Errorf("expected metadata.publisher 'example.org', got '%s'", spec.Metadata.Publisher)
	}
	if spec.Base.Distribution != "alpine" {
		t.Errorf("expected base.distribution 'alpine', got '%s'", spec.Base.Distribution)
	}
	if spec.Base.Release != "3.20" {
		t.Errorf("expected base.release '3.20', got '%s'", spec.Base.Release)
	}
	if spec.Base.Architecture != "amd64" {
		t.Errorf("expected base.architecture 'amd64', got '%s'", spec.Base.Architecture)
	}
	if len(spec.EntryPoint.Command) != 2 || spec.EntryPoint.Command[0] != "/bin/echo" || spec.EntryPoint.Command[1] != "hello" {
		t.Errorf("unexpected entrypoint.command: %v", spec.EntryPoint.Command)
	}
	if spec.Requires.Resources.Memory != "64MiB" {
		t.Errorf("expected requires.resources.memory '64MiB', got '%s'", spec.Requires.Resources.Memory)
	}
	if spec.Requires.Resources.CPUs != 1 {
		t.Errorf("expected requires.resources.cpus 1, got %d", spec.Requires.Resources.CPUs)
	}
}

func TestLoadBuildComplete(t *testing.T) {
	spec, err := LoadBuild("testdata/build_complete.toml")
	if err != nil {
		t.Fatalf("expected no error loading build_complete.toml, got: %v", err)
	}

	// Metadata
	if spec.Metadata.Name != "image-tool" {
		t.Errorf("unexpected name: %s", spec.Metadata.Name)
	}
	if spec.Metadata.Version != "1.4.0" {
		t.Errorf("unexpected version: %s", spec.Metadata.Version)
	}
	if spec.Metadata.Publisher != "example.org" {
		t.Errorf("unexpected publisher: %s", spec.Metadata.Publisher)
	}
	if spec.Metadata.Description != "Image processing CLI" {
		t.Errorf("unexpected description: %s", spec.Metadata.Description)
	}
	if spec.Metadata.License != "Apache-2.0" {
		t.Errorf("unexpected license: %s", spec.Metadata.License)
	}
	if spec.Metadata.Homepage != "https://example.org/image-tool" {
		t.Errorf("unexpected homepage: %s", spec.Metadata.Homepage)
	}

	// Base
	if spec.Base.Distribution != "alpine" || spec.Base.Release != "3.20" || spec.Base.Architecture != "amd64" {
		t.Errorf("unexpected base values: %+v", spec.Base)
	}

	// Build steps
	if spec.Build == nil {
		t.Fatalf("expected build steps to be set")
	}
	if len(spec.Build.Steps) != 4 {
		t.Fatalf("expected 4 build steps, got %d", len(spec.Build.Steps))
	}
	if spec.Build.Steps[0].Action != "run" || spec.Build.Steps[0].Command != "apk add --no-cache imagemagick" {
		t.Errorf("unexpected build.steps[0]: %+v", spec.Build.Steps[0])
	}
	if spec.Build.Steps[1].Action != "copy" || spec.Build.Steps[1].From != "./bin/image-tool" || spec.Build.Steps[1].To != "/opt/image-tool/bin/image-tool" || spec.Build.Steps[1].Mode != "0755" {
		t.Errorf("unexpected build.steps[1]: %+v", spec.Build.Steps[1])
	}
	if spec.Build.Steps[2].Action != "run" || spec.Build.Steps[2].Command != "strip /opt/image-tool/bin/image-tool" {
		t.Errorf("unexpected build.steps[2]: %+v", spec.Build.Steps[2])
	}
	if spec.Build.Steps[3].Action != "env" || spec.Build.Steps[3].Vars["IMAGE_TOOL_HOME"] != "/opt/image-tool" {
		t.Errorf("unexpected build.steps[3]: %+v", spec.Build.Steps[3])
	}

	// Entrypoint
	if len(spec.EntryPoint.Command) != 1 || spec.EntryPoint.Command[0] != "/opt/image-tool/bin/image-tool" {
		t.Errorf("unexpected entrypoint.command: %v", spec.EntryPoint.Command)
	}
	if spec.EntryPoint.WorkingDir != "/opt/image-tool" {
		t.Errorf("unexpected entrypoint.workingDir: %s", spec.EntryPoint.WorkingDir)
	}

	// Requires — resources
	if spec.Requires.Resources.Memory != "512MiB" || spec.Requires.Resources.CPUs != 1 {
		t.Errorf("unexpected requires.resources: %+v", spec.Requires.Resources)
	}

	// Requires — dataDirs
	if len(spec.Requires.DataDirs) != 1 || spec.Requires.DataDirs[0].Path != "/var/lib/image-tool" {
		t.Errorf("unexpected requires.dataDirs: %+v", spec.Requires.DataDirs)
	}

	// Requires — network
	if spec.Requires.Network.Outbound {
		t.Errorf("expected requires.network.outbound to be false")
	}

	// Requires — devices, display, audio
	if spec.Requires.Devices == nil || spec.Requires.Devices.Webcam || spec.Requires.Devices.Microphone || spec.Requires.Devices.GPU {
		t.Errorf("expected all requires.devices to be false")
	}
	if spec.Requires.Display == nil || spec.Requires.Display.Enabled {
		t.Errorf("expected requires.display.enabled to be false")
	}
	if spec.Requires.Audio == nil || spec.Requires.Audio.Enabled {
		t.Errorf("expected requires.audio.enabled to be false")
	}

	// Service — ports
	if spec.Service == nil {
		t.Fatalf("expected service to be set")
	}
	if len(spec.Service.Ports) != 1 {
		t.Fatalf("expected 1 service port, got %d", len(spec.Service.Ports))
	}
	if spec.Service.Ports[0].Name != "http" || spec.Service.Ports[0].ContainerPort != 8080 || spec.Service.Ports[0].Protocol != "tcp" {
		t.Errorf("unexpected service.ports[0]: %+v", spec.Service.Ports[0])
	}

	// Service — health
	if spec.Service.Health == nil {
		t.Fatalf("expected service.health to be set")
	}
	if spec.Service.Health.Type != "http" || spec.Service.Health.Path != "/healthz" || spec.Service.Health.Port != 8080 {
		t.Errorf("unexpected service.health: %+v", spec.Service.Health)
	}
	if spec.Service.Health.Interval != "10s" || spec.Service.Health.Timeout != "2s" || spec.Service.Health.Failures != 3 {
		t.Errorf("unexpected service.health timing: %+v", spec.Service.Health)
	}

	// Service — readiness
	if spec.Service.Readiness == nil {
		t.Fatalf("expected service.readiness to be set")
	}
	if spec.Service.Readiness.Type != "http" || spec.Service.Readiness.Path != "/ready" || spec.Service.Readiness.Port != 8080 || spec.Service.Readiness.Interval != "5s" {
		t.Errorf("unexpected service.readiness: %+v", spec.Service.Readiness)
	}

	// Reproducible
	if spec.Reproducible == nil {
		t.Fatalf("expected reproducible to be set")
	}
	if !spec.Reproducible.Enabled {
		t.Errorf("expected reproducible.enabled to be true")
	}
	if !spec.Reproducible.SBOM {
		t.Errorf("expected reproducible.sbom to be true")
	}
}

func TestBuildRejectUnknownFields(t *testing.T) {
	_, err := LoadBuild("testdata/build_unknown_field.toml")
	if err == nil {
		t.Fatalf("expected error for unknown fields, got nil")
	}
	if !strings.Contains(err.Error(), "unknown fields") {
		t.Errorf("expected error to mention 'unknown fields', got: %v", err)
	}
}

func TestBuildInvalidSyntax(t *testing.T) {
	_, err := LoadBuild("testdata/build_invalid_syntax.toml")
	if err == nil {
		t.Fatalf("expected decode error for invalid syntax, got nil")
	}
}

func TestLoadBuildString(t *testing.T) {
	valid := `
apiVersion = "vpok.io/v1"
kind = "Build"

[metadata]
name = "in-memory"
version = "1.0.0"
publisher = "example.org"

[base]
distribution = "alpine"
release = "3.20"
architecture = "amd64"

[entrypoint]
command = ["/bin/true"]

[requires.resources]
memory = "32MiB"
cpus = 1
`
	spec, err := LoadBuildString(valid)
	if err != nil {
		t.Fatalf("expected no error parsing valid string, got: %v", err)
	}
	if spec.Metadata.Name != "in-memory" {
		t.Errorf("expected name 'in-memory', got '%s'", spec.Metadata.Name)
	}

	invalid := `
apiVersion = "vpok.io/v1"
kind = "Build"
unknownKey = "reject-me"
`
	_, err = LoadBuildString(invalid)
	if err == nil {
		t.Fatalf("expected error parsing string with unknown keys, got nil")
	}
}
