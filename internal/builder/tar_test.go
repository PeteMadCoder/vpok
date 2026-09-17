package builder

import (
	"archive/tar"
	"bytes"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestCreateReproducibleLayerTar(t *testing.T) {
	srcDir := t.TempDir()

	// Create test directory tree
	if err := os.MkdirAll(filepath.Join(srcDir, "etc"), 0755); err != nil {
		t.Fatalf("failed to create etc dir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(srcDir, "usr", "bin"), 0755); err != nil {
		t.Fatalf("failed to create usr/bin dir: %v", err)
	}

	file1 := filepath.Join(srcDir, "etc", "config.txt")
	if err := os.WriteFile(file1, []byte("port = 8080\n"), 0644); err != nil {
		t.Fatalf("failed to write config.txt: %v", err)
	}

	file2 := filepath.Join(srcDir, "usr", "bin", "hello")
	if err := os.WriteFile(file2, []byte("#!/bin/sh\necho hello\n"), 0755); err != nil {
		t.Fatalf("failed to write hello: %v", err)
	}

	// Create symlink
	symlink := filepath.Join(srcDir, "usr", "bin", "greeting")
	if err := os.Symlink("hello", symlink); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	// Generate two separate tar streams to verify determinism
	var buf1, buf2 bytes.Buffer
	if err := CreateReproducibleLayerTar(srcDir, &buf1); err != nil {
		t.Fatalf("first tar generation failed: %v", err)
	}
	if err := CreateReproducibleLayerTar(srcDir, &buf2); err != nil {
		t.Fatalf("second tar generation failed: %v", err)
	}

	if !bytes.Equal(buf1.Bytes(), buf2.Bytes()) {
		t.Fatalf("tar archives are not bit-for-bit identical across runs")
	}

	// Inspect the tar contents and normalized headers
	tr := tar.NewReader(&buf1)
	seen := make(map[string]bool)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("failed to read tar header: %v", err)
		}

		seen[hdr.Name] = true

		if !hdr.ModTime.Equal(EpochTime) {
			t.Errorf("header %q ModTime got %v, want %v", hdr.Name, hdr.ModTime, EpochTime)
		}
		if hdr.Uid != 0 || hdr.Gid != 0 || hdr.Uname != "root" || hdr.Gname != "root" {
			t.Errorf("header %q ownership not normalized to root: Uid=%d, Gid=%d, Uname=%s, Gname=%s",
				hdr.Name, hdr.Uid, hdr.Gid, hdr.Uname, hdr.Gname)
		}
	}

	expectedEntries := []string{"etc", "etc/config.txt", "usr", "usr/bin", "usr/bin/hello", "usr/bin/greeting"}
	for _, expected := range expectedEntries {
		if !seen[expected] {
			t.Errorf("missing expected tar entry %q", expected)
		}
	}
}
