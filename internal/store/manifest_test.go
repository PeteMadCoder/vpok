package store

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/PeteMadCoder/vpok/internal/manifest"
)

func TestManifestRoundtrip(t *testing.T) {
	root := t.TempDir()
	store, err := NewCASStore(root)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	m := &manifest.Manifest{
		APIVersion: "vpok.io/v1",
		Kind:       "Manifest",
		Metadata: manifest.BuildMetadata{
			Name:      "test-app",
			Version:   "1.0.0",
			Publisher: "example.org",
		},
		Base: manifest.Base{
			Distribution: "alpine",
			Release:      "3.20",
			Architecture: "amd64",
		},
		EntryPoint: manifest.EntryPoint{
			Command: []string{"/app/entrypoint"},
		},
		Requires: manifest.Requires{
			Resources: manifest.RequireResources{
				Memory: "128MiB",
				CPUs:   1,
			},
		},
	}

	digest, err := store.PutManifest(ctx, m)
	if err != nil {
		t.Fatalf("PutManifest failed: %v", err)
	}

	retrieved, err := store.GetManifest(ctx, digest)
	if err != nil {
		t.Fatalf("GetManifest failed: %v", err)
	}

	if retrieved.Metadata.Name != m.Metadata.Name {
		t.Errorf("manifest name mismatch: got %q, want %q", retrieved.Metadata.Name, m.Metadata.Name)
	}
	if retrieved.Metadata.Version != m.Metadata.Version {
		t.Errorf("manifest version mismatch: got %q, want %q", retrieved.Metadata.Version, m.Metadata.Version)
	}
}

func TestManifestVerifyOnReadCorruption(t *testing.T) {
	root := t.TempDir()
	store, err := NewCASStore(root)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	m := &manifest.Manifest{
		APIVersion: "vpok.io/v1",
		Kind:       "Manifest",
		Metadata: manifest.BuildMetadata{
			Name: "corruptible",
		},
	}

	digest, err := store.PutManifest(ctx, m)
	if err != nil {
		t.Fatalf("PutManifest failed: %v", err)
	}

	path, err := store.layout.ManifestPath(digest)
	if err != nil {
		t.Fatalf("ManifestPath failed: %v", err)
	}

	if err := os.WriteFile(path, []byte(`{"apiVersion":"tampered"}`), 0644); err != nil {
		t.Fatalf("failed to tamper manifest: %v", err)
	}

	_, err = store.GetManifest(ctx, digest)
	if !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("expected ErrDigestMismatch on corrupted manifest, got: %v", err)
	}
}

func TestManifestNotFoundAndNil(t *testing.T) {
	root := t.TempDir()
	store, err := NewCASStore(root)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	if _, err := store.PutManifest(ctx, nil); err == nil {
		t.Error("expected error for nil manifest")
	}

	nonExistent := FromBytes([]byte("non-existent-manifest"))
	if _, err := store.GetManifest(ctx, nonExistent); !errors.Is(err, ErrManifestNotFound) {
		t.Errorf("expected ErrManifestNotFound, got: %v", err)
	}
}
