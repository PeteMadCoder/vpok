package main

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunKeygen(t *testing.T) {
	tempDir := t.TempDir()
	outDir := filepath.Join(tempDir, "keys")

	err := runKeygen([]string{"-out", outDir})
	if err != nil {
		t.Fatalf("runKeygen failed: %v", err)
	}

	privPath := filepath.Join(outDir, "publisher.key")
	pubPath := filepath.Join(outDir, "publisher.pub")

	privBytes, err := os.ReadFile(privPath)
	if err != nil {
		t.Fatalf("failed to read private key: %v", err)
	}

	pubBytes, err := os.ReadFile(pubPath)
	if err != nil {
		t.Fatalf("failed to read public key: %v", err)
	}

	privHex := string(privBytes)
	if len(privHex) < 128 { // 64 bytes in hex is 128 chars
		t.Fatalf("unexpected private key length: %d", len(privHex))
	}

	pubHex := string(pubBytes)
	if len(pubHex) < 64 { // 32 bytes in hex is 64 chars
		t.Fatalf("unexpected public key length: %d", len(pubHex))
	}

	privDecoded, err := hex.DecodeString(privHex[:128])
	if err != nil {
		t.Fatalf("failed to decode private key hex: %v", err)
	}
	if len(privDecoded) != 64 {
		t.Fatalf("expected 64 private key bytes, got %d", len(privDecoded))
	}

	pubDecoded, err := hex.DecodeString(pubHex[:64])
	if err != nil {
		t.Fatalf("failed to decode public key hex: %v", err)
	}
	if len(pubDecoded) != 32 {
		t.Fatalf("expected 32 public key bytes, got %d", len(pubDecoded))
	}

	info, err := os.Stat(privPath)
	if err != nil {
		t.Fatalf("failed to stat private key: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0600 {
		t.Errorf("expected private key permission 0600, got %o", perm)
	}
}

func TestRunValidate(t *testing.T) {
	tempDir := t.TempDir()

	validBuildSpec := `apiVersion = "vpok.io/v1"
kind = "Build"

[metadata]
name = "test-pkg"
version = "1.0.0"
publisher = "example.com"

[base]
distribution = "alpine"
release = "3.20"
architecture = "amd64"

[entrypoint]
command = ["/bin/sh"]

[requires.resources]
memory = "64MiB"
cpus = 1
`
	validBuildPath := filepath.Join(tempDir, "vpok.build.toml")
	if err := os.WriteFile(validBuildPath, []byte(validBuildSpec), 0644); err != nil {
		t.Fatalf("failed to write build spec: %v", err)
	}

	invalidBuildSpec := `apiVersion = "vpok.io/v1"
kind = "Build"
`
	invalidBuildPath := filepath.Join(tempDir, "invalid.build.toml")
	if err := os.WriteFile(invalidBuildPath, []byte(invalidBuildSpec), 0644); err != nil {
		t.Fatalf("failed to write invalid build spec: %v", err)
	}

	validDeploySpec := `apiVersion = "vpok.io/v1"
kind = "Deploy"

[metadata]
name = "test-deploy"

[package]
source = "example.com/test-pkg"
version = "1.0.0"

[resources]
memory = "128MiB"
cpus = 2
`
	validDeployPath := filepath.Join(tempDir, "vpok.deploy.toml")
	if err := os.WriteFile(validDeployPath, []byte(validDeploySpec), 0644); err != nil {
		t.Fatalf("failed to write deploy spec: %v", err)
	}

	t.Run("valid build file with flag", func(t *testing.T) {
		err := runValidate([]string{"--build", validBuildPath})
		if err != nil {
			t.Errorf("expected validation success, got %v", err)
		}
	})

	t.Run("valid build file positional", func(t *testing.T) {
		err := runValidate([]string{validBuildPath})
		if err != nil {
			t.Errorf("expected validation success, got %v", err)
		}
	})

	t.Run("invalid build file", func(t *testing.T) {
		err := runValidate([]string{"--build", invalidBuildPath})
		if err == nil {
			t.Error("expected validation error, got nil")
		}
	})

	t.Run("valid deploy file with flag", func(t *testing.T) {
		err := runValidate([]string{"--deploy", validDeployPath})
		if err != nil {
			t.Errorf("expected validation success, got %v", err)
		}
	})

	t.Run("valid deploy file positional", func(t *testing.T) {
		err := runValidate([]string{validDeployPath})
		if err != nil {
			t.Errorf("expected validation success, got %v", err)
		}
	})

	t.Run("missing arguments", func(t *testing.T) {
		err := runValidate([]string{})
		if err == nil {
			t.Error("expected error for missing arguments, got nil")
		}
	})
}

func TestRunBuildAndInspect(t *testing.T) {
	tempDir := t.TempDir()
	storeDir := filepath.Join(tempDir, "store")
	contextDir := filepath.Join(tempDir, "context")
	if err := os.MkdirAll(contextDir, 0755); err != nil {
		t.Fatalf("failed to create context directory: %v", err)
	}

	appFile := filepath.Join(contextDir, "hello.txt")
	if err := os.WriteFile(appFile, []byte("hello world\n"), 0644); err != nil {
		t.Fatalf("failed to create context file: %v", err)
	}

	buildSpec := `apiVersion = "vpok.io/v1"
kind = "Build"

[metadata]
name = "cli-test-pkg"
version = "0.1.0"
publisher = "example.com"

[base]
distribution = "alpine"
release = "3.20"
architecture = "amd64"

[[build.steps]]
action = "copy"
from = "hello.txt"
to = "/app/hello.txt"

[entrypoint]
command = ["/bin/cat", "/app/hello.txt"]

[requires.resources]
memory = "64MiB"
cpus = 1
`
	buildPath := filepath.Join(tempDir, "vpok.build.toml")
	if err := os.WriteFile(buildPath, []byte(buildSpec), 0644); err != nil {
		t.Fatalf("failed to write build spec: %v", err)
	}

	keysDir := filepath.Join(tempDir, "keys")
	if err := runKeygen([]string{"-out", keysDir}); err != nil {
		t.Fatalf("keygen failed: %v", err)
	}
	keyPath := filepath.Join(keysDir, "publisher.key")

	t.Run("build with signing key", func(t *testing.T) {
		err := runBuild([]string{
			"-f", buildPath,
			"-C", contextDir,
			"-store", storeDir,
			"-key", keyPath,
		})
		if err != nil {
			t.Fatalf("runBuild failed: %v", err)
		}
	})

	t.Run("build unsigned", func(t *testing.T) {
		err := runBuild([]string{
			"-f", buildPath,
			"-C", contextDir,
			"-store", storeDir,
		})
		if err != nil {
			t.Fatalf("runBuild unsigned failed: %v", err)
		}
	})

	t.Run("inspect success", func(t *testing.T) {
		var foundDigest string
		err := filepath.Walk(filepath.Join(storeDir, "manifests"), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() {
				// manifest file is stored at .../<alg>/<prefix>/<rest>
				rel, err := filepath.Rel(filepath.Join(storeDir, "manifests", "sha256"), path)
				if err == nil {
					parts := strings.Split(rel, string(filepath.Separator))
					if len(parts) == 2 {
						foundDigest = "sha256:" + parts[0] + parts[1]
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("failed to walk manifests dir: %v", err)
		}
		if foundDigest == "" {
			t.Fatal("no manifest found in store")
		}

		err = runInspect([]string{"-store", storeDir, foundDigest})
		if err != nil {
			t.Fatalf("runInspect failed on existing manifest: %v", err)
		}
	})

	t.Run("inspect missing digest argument", func(t *testing.T) {
		err := runInspect([]string{"-store", storeDir})
		if err == nil {
			t.Error("expected error for missing digest argument, got nil")
		}
	})

	t.Run("inspect non-existent digest", func(t *testing.T) {
		err := runInspect([]string{"-store", storeDir, "sha256:0000000000000000000000000000000000000000000000000000000000000000"})
		if err == nil {
			t.Error("expected error for non-existent digest, got nil")
		}
	})
}

