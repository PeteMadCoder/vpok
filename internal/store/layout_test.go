package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLayoutPaths(t *testing.T) {
	root := t.TempDir()
	layout := NewLayout(root)

	if err := layout.EnsureDirs(); err != nil {
		t.Fatalf("EnsureDirs failed: %v", err)
	}

	hexStr := strings.Repeat("e", 64)
	d := Digest("sha256:" + hexStr)

	blobPath, err := layout.BlobPath(d)
	if err != nil {
		t.Fatalf("BlobPath failed: %v", err)
	}
	expectedBlobPath := filepath.Join(root, "blobs", "sha256", "ee", strings.Repeat("e", 62))
	if blobPath != expectedBlobPath {
		t.Errorf("BlobPath got %q, want %q", blobPath, expectedBlobPath)
	}

	manifestPath, err := layout.ManifestPath(d)
	if err != nil {
		t.Fatalf("ManifestPath failed: %v", err)
	}
	expectedManifestPath := filepath.Join(root, "manifests", "sha256", "ee", strings.Repeat("e", 62))
	if manifestPath != expectedManifestPath {
		t.Errorf("ManifestPath got %q, want %q", manifestPath, expectedManifestPath)
	}

	// Invalid digest
	if _, err := layout.BlobPath(Digest("invalid")); err == nil {
		t.Error("expected error for invalid digest on BlobPath")
	}
	if _, err := layout.ManifestPath(Digest("invalid")); err == nil {
		t.Error("expected error for invalid digest on ManifestPath")
	}

	// Verify directories created
	for _, dir := range []string{layout.BlobsDir(), layout.ManifestsDir(), layout.TmpDir()} {
		info, err := os.Stat(dir)
		if err != nil {
			t.Errorf("expected directory %q to exist: %v", dir, err)
		} else if !info.IsDir() {
			t.Errorf("expected %q to be a directory", dir)
		}
	}
}
