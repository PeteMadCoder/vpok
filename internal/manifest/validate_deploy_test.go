package manifest

import (
	"strings"
	"testing"
)

func TestValidateDeploy_ValidSpecs(t *testing.T) {
	minimal, err := LoadDeploy("testdata/deploy_minimal.toml")
	if err != nil {
		t.Fatalf("failed to load deploy_minimal.toml: %v", err)
	}
	if err := ValidateDeploy(minimal); err != nil {
		t.Fatalf("expected minimal deploy spec to be valid, got: %v", err)
	}

	complete, err := LoadDeploy("testdata/deploy_complete.toml")
	if err != nil {
		t.Fatalf("failed to load deploy_complete.toml: %v", err)
	}
	if err := ValidateDeploy(complete); err != nil {
		t.Fatalf("expected complete deploy spec to be valid, got: %v", err)
	}
}

func TestValidateDeploy_Failures(t *testing.T) {
	baseValid := func() *DeploySpec {
		spec, err := LoadDeploy("testdata/deploy_minimal.toml")
		if err != nil {
			t.Fatalf("failed to load base valid deploy spec: %v", err)
		}
		return spec
	}

	tests := []struct {
		name      string
		mutate    func(s *DeploySpec)
		errSubstr string
	}{
		{
			name:      "nil deploy spec",
			mutate:    nil,
			errSubstr: "cannot be nil",
		},
		{
			name: "invalid apiVersion",
			mutate: func(s *DeploySpec) {
				s.APIVersion = "vpok.io/v2"
			},
			errSubstr: "unsupported apiVersion",
		},
		{
			name: "invalid kind",
			mutate: func(s *DeploySpec) {
				s.Kind = "Build"
			},
			errSubstr: "unsupported kind",
		},
		{
			name: "empty name",
			mutate: func(s *DeploySpec) {
				s.Metadata.Name = ""
			},
			errSubstr: "metadata.name is required",
		},
		{
			name: "missing package source",
			mutate: func(s *DeploySpec) {
				s.Package.Source = ""
			},
			errSubstr: "package.source is required",
		},
		{
			name: "missing version and digest",
			mutate: func(s *DeploySpec) {
				s.Package.Version = ""
				s.Package.Digest = ""
			},
			errSubstr: "at least one of package.version or package.digest must be set",
		},
		{
			name: "invalid memory",
			mutate: func(s *DeploySpec) {
				s.Resources.Memory = "not-a-size"
			},
			errSubstr: "invalid resources.memory",
		},
		{
			name: "non-positive cpus",
			mutate: func(s *DeploySpec) {
				s.Resources.CPUs = 0
			},
			errSubstr: "resources.cpus must be positive",
		},
		{
			name: "duplicate storage name",
			mutate: func(s *DeploySpec) {
				s.Storage = []DeployStorage{
					{Name: "data", GuestPath: "/data1", Type: "volume", Size: "1GiB"},
					{Name: "data", GuestPath: "/data2", Type: "volume", Size: "1GiB"},
				}
			},
			errSubstr: "duplicate storage name",
		},
		{
			name: "duplicate storage guestPath",
			mutate: func(s *DeploySpec) {
				s.Storage = []DeployStorage{
					{Name: "data1", GuestPath: "/data", Type: "volume", Size: "1GiB"},
					{Name: "data2", GuestPath: "/data", Type: "volume", Size: "1GiB"},
				}
			},
			errSubstr: "duplicate storage guestPath",
		},
		{
			name: "volume storage missing size",
			mutate: func(s *DeploySpec) {
				s.Storage = []DeployStorage{
					{Name: "data", GuestPath: "/data", Type: "volume"},
				}
			},
			errSubstr: "volume requires 'size'",
		},
		{
			name: "hostPath storage with escaping path",
			mutate: func(s *DeploySpec) {
				s.Storage = []DeployStorage{
					{Name: "data", GuestPath: "/data", Type: "hostPath", HostPath: "$HOME/../secret", Mode: "ro"},
				}
			},
			errSubstr: "cannot contain '..' segments",
		},
		{
			name: "invalid network mode",
			mutate: func(s *DeploySpec) {
				s.Network = &DeployNetwork{Mode: "custom"}
			},
			errSubstr: "invalid network.mode",
		},
		{
			name: "network allow with both host and cidr",
			mutate: func(s *DeploySpec) {
				s.Network = &DeployNetwork{
					Mode: "nat",
					Allow: []DeployNetworkAllow{
						{Host: "example.org", CIDR: "10.0.0.0/8", Port: 80},
					},
				}
			},
			errSubstr: "must specify exactly one of 'host' or 'cidr'",
		},
		{
			name: "invalid secret source prefix",
			mutate: func(s *DeploySpec) {
				s.Secrets = []DeploySecret{
					{Name: "key", Source: "vault:secret/key"},
				}
			},
			errSubstr: "must start with 'env:' or 'file:'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var spec *DeploySpec
			if tc.mutate != nil {
				spec = baseValid()
				tc.mutate(spec)
			}
			err := ValidateDeploy(spec)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tc.errSubstr)
			}
			if !strings.Contains(err.Error(), tc.errSubstr) {
				t.Fatalf("expected error containing %q, got: %v", tc.errSubstr, err)
			}
		})
	}
}

func TestValidateCompatibility(t *testing.T) {
	baseDeploy := func() *DeploySpec {
		return &DeploySpec{
			APIVersion: "vpok.io/v1",
			Kind:       "Deploy",
			Metadata: DeployMetadata{
				Name: "app-inst",
			},
			Package: DeployPackage{
				Source:  "registry.vpok.io/apps/my-app",
				Version: "1.0.0",
			},
			Resources: DeployResources{
				Memory: "512MiB",
				CPUs:   2,
			},
			Storage: []DeployStorage{
				{
					Name:      "data",
					GuestPath: "/var/lib/app",
					Type:      "volume",
					Size:      "1GiB",
				},
			},
			Network: &DeployNetwork{
				Mode: "none",
			},
		}
	}

	baseManifest := func() *Manifest {
		return &Manifest{
			APIVersion: "vpok.io/v1",
			Kind:       "Manifest",
			Metadata: BuildMetadata{
				Name:      "my-app",
				Version:   "1.0.0",
				Publisher: "example.org",
			},
			Base: Base{
				Distribution: "alpine",
				Release:      "3.20",
				Architecture: "amd64",
			},
			EntryPoint: EntryPoint{
				Command: []string{"/bin/app"},
			},
			Requires: Requires{
				Resources: RequireResources{
					Memory: "256MiB",
					CPUs:   1,
				},
				DataDirs: []DataDirs{
					{Path: "/var/lib/app"},
				},
				Network: RequireNetwork{
					Outbound: false,
				},
			},
		}
	}

	t.Run("valid compatibility", func(t *testing.T) {
		deploy := baseDeploy()
		manifest := baseManifest()
		if err := ValidateCompatibility(deploy, manifest); err != nil {
			t.Fatalf("expected compatible, got: %v", err)
		}
	})

	t.Run("missing required dataDir mount", func(t *testing.T) {
		deploy := baseDeploy()
		deploy.Storage = nil
		manifest := baseManifest()
		err := ValidateCompatibility(deploy, manifest)
		if err == nil || !strings.Contains(err.Error(), "missing required storage mount for dataDir") {
			t.Fatalf("expected missing storage mount error, got: %v", err)
		}
	})

	t.Run("insufficient memory granted", func(t *testing.T) {
		deploy := baseDeploy()
		deploy.Resources.Memory = "128MiB"
		manifest := baseManifest()
		err := ValidateCompatibility(deploy, manifest)
		if err == nil || !strings.Contains(err.Error(), "insufficient memory granted") {
			t.Fatalf("expected insufficient memory error, got: %v", err)
		}
	})

	t.Run("insufficient cpus granted", func(t *testing.T) {
		deploy := baseDeploy()
		deploy.Resources.CPUs = 1
		manifest := baseManifest()
		manifest.Requires.Resources.CPUs = 2
		err := ValidateCompatibility(deploy, manifest)
		if err == nil || !strings.Contains(err.Error(), "insufficient cpus granted") {
			t.Fatalf("expected insufficient cpus error, got: %v", err)
		}
	})

	t.Run("network mode mismatch when outbound false", func(t *testing.T) {
		deploy := baseDeploy()
		deploy.Network = &DeployNetwork{Mode: "nat"}
		manifest := baseManifest()
		manifest.Requires.Network.Outbound = false
		err := ValidateCompatibility(deploy, manifest)
		if err == nil || !strings.Contains(err.Error(), "package requires outbound=false") {
			t.Fatalf("expected network mismatch error, got: %v", err)
		}
	})

	t.Run("network mode mismatch when outbound true", func(t *testing.T) {
		deploy := baseDeploy()
		deploy.Network = &DeployNetwork{Mode: "none"}
		manifest := baseManifest()
		manifest.Requires.Network.Outbound = true
		err := ValidateCompatibility(deploy, manifest)
		if err == nil || !strings.Contains(err.Error(), "package requires outbound=true") {
			t.Fatalf("expected network mismatch error, got: %v", err)
		}
	})

	t.Run("device granted without requirement", func(t *testing.T) {
		deploy := baseDeploy()
		deploy.Devices = &DeployDevices{Webcam: true}
		manifest := baseManifest()
		manifest.Requires.Devices = nil
		err := ValidateCompatibility(deploy, manifest)
		if err == nil || !strings.Contains(err.Error(), "deploy grants devices but package manifest declares no device requirements") {
			t.Fatalf("expected device grant error, got: %v", err)
		}
	})

	t.Run("service publish port not declared", func(t *testing.T) {
		deploy := baseDeploy()
		deploy.Service = &DeployService{
			Publish: []DeployServicePublish{
				{Name: "metrics", HostPort: 9090},
			},
		}
		manifest := baseManifest()
		manifest.Service = &BuildService{
			Ports: []ServicePort{
				{Name: "http", ContainerPort: 8080, Protocol: "tcp"},
			},
		}
		err := ValidateCompatibility(deploy, manifest)
		if err == nil || !strings.Contains(err.Error(), "which is not declared in package service ports") {
			t.Fatalf("expected undeclared published port error, got: %v", err)
		}
	})
}
