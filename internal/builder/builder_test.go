package builder

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/PeteMadCoder/vpok/internal/manifest"
	"github.com/PeteMadCoder/vpok/internal/store"
)

func TestBuilder_BuildEndToEnd(t *testing.T) {
	rootStore := t.TempDir()
	casStore, err := store.NewCASStore(rootStore)
	if err != nil {
		t.Fatalf("failed to create CAS store: %v", err)
	}

	pubKey, privKey, err := manifest.GenerateKeyPair()
	if err != nil {
		t.Fatalf("failed to generate key pair: %v", err)
	}

	b := NewBuilder(casStore, privKey)

	// Prepare build context
	ctxDir := t.TempDir()
	serverBin := filepath.Join(ctxDir, "server")
	if err := os.WriteFile(serverBin, []byte("#!/bin/sh\necho server running\n"), 0755); err != nil {
		t.Fatalf("failed to write server binary: %v", err)
	}

	spec := &manifest.BuildSpec{
		APIVersion: "vpok.io/v1",
		Kind:       "Build",
		Metadata: manifest.BuildMetadata{
			Name:      "test-service",
			Version:   "1.0.0",
			Publisher: "example.org",
		},
		Base: manifest.Base{
			Distribution: "alpine",
			Release:      "3.20",
			Architecture: "amd64",
		},
		Build: &manifest.BuildConfig{
			Steps: []manifest.BuildStep{
				{
					Action: "copy",
					From:   "server",
					To:     "/opt/server/server",
					Mode:   "0755",
				},
				{
					Action: "env",
					Vars:   map[string]string{"SERVICE_PORT": "8080"},
				},
			},
		},
		EntryPoint: manifest.EntryPoint{
			Command:    []string{"/opt/server/server"},
			WorkingDir: "/opt/server",
		},
		Requires: manifest.Requires{
			Resources: manifest.RequireResources{
				Memory: "128MiB",
				CPUs:   1,
			},
			DataDirs: []manifest.DataDirs{
				{Path: "/var/lib/data"},
			},
			Network: manifest.RequireNetwork{
				Outbound: false,
			},
		},
		Reproducible: &manifest.Reproducible{
			Enabled: true,
		},
	}

	ctx := context.Background()
	m, manifestDigest, err := b.Build(ctx, spec, ctxDir)
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	if m == nil {
		t.Fatalf("expected non-nil manifest")
	}

	if len(m.Layers) != 1 {
		t.Fatalf("expected 1 layer, got %d", len(m.Layers))
	}

	// Verify layer exists in CAS
	layerDigest := store.Digest(m.Layers[0].Digest)
	hasBlob, err := casStore.HasBlob(ctx, layerDigest)
	if err != nil || !hasBlob {
		t.Fatalf("layer blob %s not found in store: %v", layerDigest, err)
	}

	// Verify manifest exists in CAS
	storedManifest, err := casStore.GetManifest(ctx, manifestDigest)
	if err != nil {
		t.Fatalf("failed to retrieve stored manifest: %v", err)
	}

	if storedManifest.Metadata.Name != "test-service" {
		t.Errorf("stored manifest name mismatch: %s", storedManifest.Metadata.Name)
	}

	// Verify cryptographic signature
	if err := manifest.VerifyManifest(storedManifest, "example.org", pubKey); err != nil {
		t.Fatalf("manifest signature verification failed: %v", err)
	}
}

func TestBuilder_ValidationFailure(t *testing.T) {
	rootStore := t.TempDir()
	casStore, err := store.NewCASStore(rootStore)
	if err != nil {
		t.Fatalf("failed to create CAS store: %v", err)
	}

	b := NewBuilder(casStore, nil)

	invalidSpec := &manifest.BuildSpec{
		APIVersion: "vpok.io/v1",
		Kind:       "Build",
		Metadata: manifest.BuildMetadata{
			Name: "Invalid Name With Spaces",
		},
	}

	_, _, err = b.Build(context.Background(), invalidSpec, t.TempDir())
	if err == nil {
		t.Fatal("expected build to fail validation with invalid spec")
	}
}
