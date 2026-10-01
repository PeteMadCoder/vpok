package image

import (
	"archive/tar"
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/PeteMadCoder/vpok/internal/store"
)

func TestExtractTar_Valid(t *testing.T) {
	tempDir := t.TempDir()
	destDir := filepath.Join(tempDir, "dest")

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	// Add dir
	if err := tw.WriteHeader(&tar.Header{
		Name:     "app/",
		Mode:     0755,
		Typeflag: tar.TypeDir,
	}); err != nil {
		t.Fatalf("failed to write dir header: %v", err)
	}

	// Add file
	content := []byte("hello from tar\n")
	if err := tw.WriteHeader(&tar.Header{
		Name:     "app/hello.txt",
		Mode:     0644,
		Size:     int64(len(content)),
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("failed to write file header: %v", err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatalf("failed to write file content: %v", err)
	}

	// Add symlink
	if err := tw.WriteHeader(&tar.Header{
		Name:     "app/link.txt",
		Typeflag: tar.TypeSymlink,
		Linkname: "hello.txt",
	}); err != nil {
		t.Fatalf("failed to write symlink header: %v", err)
	}

	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}

	if err := ExtractTar(&buf, destDir); err != nil {
		t.Fatalf("ExtractTar failed: %v", err)
	}

	readBytes, err := os.ReadFile(filepath.Join(destDir, "app", "hello.txt"))
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}
	if string(readBytes) != "hello from tar\n" {
		t.Errorf("extracted content mismatch: got %q", string(readBytes))
	}

	linkTarget, err := os.Readlink(filepath.Join(destDir, "app", "link.txt"))
	if err != nil {
		t.Fatalf("failed to read extracted symlink: %v", err)
	}
	if linkTarget != "hello.txt" {
		t.Errorf("expected symlink target 'hello.txt', got %q", linkTarget)
	}
}

func TestExtractTar_PathTraversal(t *testing.T) {
	tempDir := t.TempDir()
	destDir := filepath.Join(tempDir, "dest")

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)

	if err := tw.WriteHeader(&tar.Header{
		Name:     "../escape.txt",
		Mode:     0644,
		Size:     4,
		Typeflag: tar.TypeReg,
	}); err != nil {
		t.Fatalf("failed to write header: %v", err)
	}
	tw.Write([]byte("test"))
	tw.Close()

	if err := ExtractTar(&buf, destDir); err == nil {
		t.Fatal("expected error for path escaping root, got nil")
	}
}

func TestExtractLayers(t *testing.T) {
	tempDir := t.TempDir()
	storeDir := filepath.Join(tempDir, "store")
	destDir := filepath.Join(tempDir, "rootfs")

	casStore, err := store.NewCASStore(storeDir)
	if err != nil {
		t.Fatalf("failed to create CAS store: %v", err)
	}

	// Layer 1: /bin/tool
	var layer1Buf bytes.Buffer
	tw1 := tar.NewWriter(&layer1Buf)
	tw1.WriteHeader(&tar.Header{
		Name:     "bin/tool",
		Mode:     0755,
		Size:     4,
		Typeflag: tar.TypeReg,
	})
	tw1.Write([]byte("tool"))
	tw1.Close()

	digest1, _, err := casStore.PutBlob(context.Background(), &layer1Buf)
	if err != nil {
		t.Fatalf("failed to put layer 1 blob: %v", err)
	}

	// Layer 2: /etc/config
	var layer2Buf bytes.Buffer
	tw2 := tar.NewWriter(&layer2Buf)
	tw2.WriteHeader(&tar.Header{
		Name:     "etc/config",
		Mode:     0644,
		Size:     3,
		Typeflag: tar.TypeReg,
	})
	tw2.Write([]byte("cfg"))
	tw2.Close()

	digest2, _, err := casStore.PutBlob(context.Background(), &layer2Buf)
	if err != nil {
		t.Fatalf("failed to put layer 2 blob: %v", err)
	}

	err = ExtractLayers(context.Background(), casStore, []string{digest1.String(), digest2.String()}, destDir)
	if err != nil {
		t.Fatalf("ExtractLayers failed: %v", err)
	}

	toolBytes, err := os.ReadFile(filepath.Join(destDir, "bin", "tool"))
	if err != nil || string(toolBytes) != "tool" {
		t.Errorf("layer 1 file extraction failed: %v, content: %q", err, string(toolBytes))
	}

	cfgBytes, err := os.ReadFile(filepath.Join(destDir, "etc", "config"))
	if err != nil || string(cfgBytes) != "cfg" {
		t.Errorf("layer 2 file extraction failed: %v, content: %q", err, string(cfgBytes))
	}
}
