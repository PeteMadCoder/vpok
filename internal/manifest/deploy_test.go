package manifest

import (
	"strings"
	"testing"
)

func TestLoadDeployMinimal(t *testing.T) {
	spec, err := LoadDeploy("testdata/deploy_minimal.toml")
	if err != nil {
		t.Fatalf("expected no error loading minimal.toml, got: %v", err)
	}

	if spec.APIVersion != "vpok.io/v1" {
		t.Errorf("expected apiVersion 'vpok.io/v1', got '%s'", spec.APIVersion)
	}
	if spec.Kind != "Deploy" {
		t.Errorf("expected kind 'Deploy', got '%s'", spec.Kind)
	}
	if spec.Metadata.Name != "hello" {
		t.Errorf("expected metadata.name 'hello', got '%s'", spec.Metadata.Name)
	}
	if spec.Package.Source != "registry.vpok.io/example/hello" {
		t.Errorf("expected package.source 'registry.vpok.io/example/hello', got '%s'", spec.Package.Source)
	}
	if spec.Package.Version != "0.1.0" {
		t.Errorf("expected package.version '0.1.0', got '%s'", spec.Package.Version)
	}
	if spec.Resources.Memory != "64MiB" {
		t.Errorf("expected resources.memory '64MiB', got '%s'", spec.Resources.Memory)
	}
	if spec.Resources.CPUs != 1 {
		t.Errorf("expected resources.cpus 1, got %d", spec.Resources.CPUs)
	}
}

func TestLoadDeployComplete(t *testing.T) {
	spec, err := LoadDeploy("testdata/deploy_complete.toml")
	if err != nil {
		t.Fatalf("expected no error loading complete.toml, got: %v", err)
	}

	// Metadata
	if spec.Metadata.Name != "image-tool" {
		t.Errorf("unexpected name: %s", spec.Metadata.Name)
	}
	if spec.Metadata.Description != "Alice's image tool" {
		t.Errorf("unexpected description: %s", spec.Metadata.Description)
	}

	// Package
	if spec.Package.Source != "registry.vpok.io/example/image-tool" {
		t.Errorf("unexpected package source: %s", spec.Package.Source)
	}
	if spec.Package.Version != "1.4.0" {
		t.Errorf("unexpected package version: %s", spec.Package.Version)
	}
	if spec.Package.Digest == "" {
		t.Errorf("expected package digest to be set")
	}
	if spec.Package.Verify == nil || spec.Package.Verify.Signature != "required" {
		t.Errorf("expected package.verify.signature 'required'")
	}
	if spec.Package.Verify.Publisher != "example.org" {
		t.Errorf("expected package.verify.publisher 'example.org'")
	}

	// Resources
	if spec.Resources.Memory != "1GiB" || spec.Resources.CPUs != 2 || spec.Resources.PIDs != 256 || spec.Resources.Disk != "4GiB" {
		t.Errorf("unexpected resources values: %+v", spec.Resources)
	}

	// Storage
	if len(spec.Storage) != 3 {
		t.Fatalf("expected 3 storage entries, got %d", len(spec.Storage))
	}
	if spec.Storage[0].Name != "data" || spec.Storage[0].Type != "volume" {
		t.Errorf("unexpected storage[0]: %+v", spec.Storage[0])
	}
	if spec.Storage[0].Persistent == nil || !*spec.Storage[0].Persistent {
		t.Errorf("expected storage[0].persistent to be true")
	}
	if spec.Storage[1].Name != "pictures" || spec.Storage[1].Type != "hostPath" || spec.Storage[1].Mode != "ro" {
		t.Errorf("unexpected storage[1]: %+v", spec.Storage[1])
	}

	// Network
	if spec.Network == nil || spec.Network.Mode != "nat" {
		t.Fatalf("unexpected network mode: %+v", spec.Network)
	}
	if len(spec.Network.DNS) != 2 {
		t.Errorf("expected 2 DNS entries, got %d", len(spec.Network.DNS))
	}
	if len(spec.Network.Allow) != 2 {
		t.Fatalf("expected 2 network allow rules, got %d", len(spec.Network.Allow))
	}
	if spec.Network.Allow[0].Host != "updates.example.org" || spec.Network.Allow[0].Port != 443 {
		t.Errorf("unexpected allow[0]: %+v", spec.Network.Allow[0])
	}
	if spec.Network.Allow[1].CIDR != "10.0.0.0/8" || spec.Network.Allow[1].Port != 5432 {
		t.Errorf("unexpected allow[1]: %+v", spec.Network.Allow[1])
	}

	// Devices, Display, Audio
	if spec.Devices == nil || spec.Devices.Webcam || spec.Devices.Microphone || spec.Devices.GPU {
		t.Errorf("expected all devices to be false")
	}
	if spec.Display == nil || spec.Display.Enabled || spec.Display.Protocol != "wayland" {
		t.Errorf("unexpected display values: %+v", spec.Display)
	}
	if spec.Audio == nil || spec.Audio.Enabled {
		t.Errorf("expected audio to be disabled")
	}

	// Secrets
	if len(spec.Secrets) != 1 || spec.Secrets[0].Name != "api-key" || spec.Secrets[0].Source != "env:IMAGE_TOOL_API_KEY" {
		t.Errorf("unexpected secrets: %+v", spec.Secrets)
	}

	// Env
	if spec.Env["LOG_LEVEL"] != "info" || spec.Env["TZ"] != "Europe/Lisbon" {
		t.Errorf("unexpected env: %+v", spec.Env)
	}

	// Restart & Shutdown
	if spec.Restart == nil || spec.Restart.Policy != "on-failure" || spec.Restart.MaxAttempts != 3 || spec.Restart.Backoff != "5s" {
		t.Errorf("unexpected restart settings: %+v", spec.Restart)
	}
	if spec.Shutdown == nil || spec.Shutdown.GracePeriod != "10s" {
		t.Errorf("unexpected shutdown settings: %+v", spec.Shutdown)
	}

	// Service
	if spec.Service == nil || len(spec.Service.Publish) != 1 {
		t.Fatalf("expected service with 1 publish rule")
	}
	if spec.Service.Publish[0].Name != "http" || spec.Service.Publish[0].HostPort != 8080 {
		t.Errorf("unexpected service publish: %+v", spec.Service.Publish[0])
	}
	if spec.Service.Update == nil || spec.Service.Update.Strategy != "recreate" {
		t.Errorf("unexpected service update strategy: %+v", spec.Service.Update)
	}
	if spec.Service.Update.RollbackOnFailure == nil || !*spec.Service.Update.RollbackOnFailure {
		t.Errorf("expected service.update.rollbackOnFailure to be true")
	}

	// License
	if spec.License == nil {
		t.Fatalf("expected license to be set")
	}
	if spec.License.Document != "/etc/vpok/license.jwt" || spec.License.Activation != "offline" || spec.License.Server != "https://license.example.org" {
		t.Errorf("unexpected license: %+v", spec.License)
	}
}

func TestDeployRejectUnknownFields(t *testing.T) {
	_, err := LoadDeploy("testdata/deploy_unknown_field.toml")
	if err == nil {
		t.Fatalf("expected error for unknown fields, got nil")
	}
	if !strings.Contains(err.Error(), "unknown fields") {
		t.Errorf("expected error to mention 'unknown fields', got: %v", err)
	}
}

func TestDeployInvalidSyntax(t *testing.T) {
	_, err := LoadDeploy("testdata/deploy_invalid_syntax.toml")
	if err == nil {
		t.Fatalf("expected decode error for invalid syntax, got nil")
	}
}

func TestLoadDeployString(t *testing.T) {
	valid := `
apiVersion = "vpok.io/v1"
kind = "Deploy"

[metadata]
name = "in-memory"

[package]
source = "registry.vpok.io/example/test"
version = "1.0.0"

[resources]
memory = "128MiB"
cpus = 1
	`
	spec, err := LoadDeployString(valid)
	if err != nil {
		t.Fatalf("expected name 'in-memory', got '%s'", spec.Metadata.Name)
	}

	invalid := `
apiVersion = "vpok.io/v1"
kind = "Deploy"
unknownKey = "reject-me"
	`
	_, err = LoadDeployString(invalid)
	if err == nil {
		t.Fatalf("expected error parsing string with unknown keys, got nil")
	}
}
