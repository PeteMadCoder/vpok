package image

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestBuildInitrd_Basic(t *testing.T) {
	tempDir := t.TempDir()
	rootfs := filepath.Join(tempDir, "rootfs")

	if err := os.MkdirAll(filepath.Join(rootfs, "bin"), 0755); err != nil {
		t.Fatalf("failed to create bin: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(rootfs, "etc"), 0755); err != nil {
		t.Fatalf("failed to create etc: %v", err)
	}

	helloPath := filepath.Join(rootfs, "bin", "hello")
	if err := os.WriteFile(helloPath, []byte("#!/bin/sh\necho hello\n"), 0755); err != nil {
		t.Fatalf("failed to write hello script: %v", err)
	}

	initSymlink := filepath.Join(rootfs, "init")
	if err := os.Symlink("/bin/hello", initSymlink); err != nil {
		t.Fatalf("failed to create symlink: %v", err)
	}

	initrdPath := filepath.Join(tempDir, "initrd.img")
	if err := BuildInitrd(rootfs, initrdPath); err != nil {
		t.Fatalf("BuildInitrd failed: %v", err)
	}

	// Verify initrd.img is a valid gzip file
	f, err := os.Open(initrdPath)
	if err != nil {
		t.Fatalf("failed to open initrd: %v", err)
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer gr.Close()

	cpioData, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("failed to read decompressed cpio data: %v", err)
	}

	// Verify CPIO magic "070701" exists in header
	if len(cpioData) < 110 {
		t.Fatalf("cpio data too short: %d bytes", len(cpioData))
	}
	if string(cpioData[:6]) != "070701" {
		t.Fatalf("expected cpio magic 070701, got %q", string(cpioData[:6]))
	}

	// Parse CPIO entries
	entries := parseCPIO(t, cpioData)
	if _, ok := entries["bin"]; !ok {
		t.Errorf("missing 'bin' directory entry in cpio archive")
	}
	if _, ok := entries["etc"]; !ok {
		t.Errorf("missing 'etc' directory entry in cpio archive")
	}
	if content, ok := entries["bin/hello"]; !ok || string(content) != "#!/bin/sh\necho hello\n" {
		t.Errorf("missing or invalid 'bin/hello' content: %q", string(content))
	}
	if target, ok := entries["init"]; !ok || string(target) != "/bin/hello" {
		t.Errorf("missing or invalid 'init' symlink target: %q", string(target))
	}
	if _, ok := entries["TRAILER!!!"]; !ok {
		t.Errorf("missing TRAILER!!! entry in cpio archive")
	}
}

func TestBuildInitrd_Reproducibility(t *testing.T) {
	tempDir := t.TempDir()
	rootfs := filepath.Join(tempDir, "rootfs")

	if err := os.MkdirAll(filepath.Join(rootfs, "data"), 0755); err != nil {
		t.Fatalf("failed to create data dir: %v", err)
	}
	for i := 0; i < 5; i++ {
		p := filepath.Join(rootfs, "data", fmt.Sprintf("file_%d.txt", i))
		if err := os.WriteFile(p, []byte(fmt.Sprintf("content_%d\n", i)), 0644); err != nil {
			t.Fatalf("failed to write file: %v", err)
		}
	}

	out1 := filepath.Join(tempDir, "initrd1.img")
	out2 := filepath.Join(tempDir, "initrd2.img")

	if err := BuildInitrd(rootfs, out1); err != nil {
		t.Fatalf("BuildInitrd run 1 failed: %v", err)
	}
	if err := BuildInitrd(rootfs, out2); err != nil {
		t.Fatalf("BuildInitrd run 2 failed: %v", err)
	}

	bytes1, err := os.ReadFile(out1)
	if err != nil {
		t.Fatalf("failed to read out1: %v", err)
	}
	bytes2, err := os.ReadFile(out2)
	if err != nil {
		t.Fatalf("failed to read out2: %v", err)
	}

	h1 := sha256.Sum256(bytes1)
	h2 := sha256.Sum256(bytes2)

	if hex.EncodeToString(h1[:]) != hex.EncodeToString(h2[:]) {
		t.Fatalf("non-deterministic initrd builds: %s != %s", hex.EncodeToString(h1[:]), hex.EncodeToString(h2[:]))
	}
}

func parseCPIO(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	entries := make(map[string][]byte)
	offset := 0

	for offset < len(data) {
		if offset+110 > len(data) {
			break
		}
		magic := string(data[offset : offset+6])
		if magic != "070701" {
			break
		}

		filesizeHex := string(data[offset+54 : offset+62])
		namesizeHex := string(data[offset+94 : offset+102])

		filesize, err := strconv.ParseInt(filesizeHex, 16, 64)
		if err != nil {
			t.Fatalf("invalid filesize hex %q: %v", filesizeHex, err)
		}
		namesize, err := strconv.ParseInt(namesizeHex, 16, 64)
		if err != nil {
			t.Fatalf("invalid namesize hex %q: %v", namesizeHex, err)
		}

		offset += 110
		if offset+int(namesize) > len(data) {
			t.Fatalf("namesize out of bounds")
		}

		rawName := data[offset : offset+int(namesize)]
		name := string(bytes.TrimRight(rawName, "\x00"))
		offset += int(namesize)

		// Name pad
		namePad := (4 - ((110 + int(namesize)) % 4)) % 4
		offset += namePad

		var content []byte
		if filesize > 0 {
			if offset+int(filesize) > len(data) {
				t.Fatalf("filesize out of bounds")
			}
			content = data[offset : offset+int(filesize)]
			offset += int(filesize)

			// Data pad
			dataPad := (4 - (int(filesize) % 4)) % 4
			offset += dataPad
		}

		entries[name] = content

		if name == "TRAILER!!!" {
			break
		}
	}

	return entries
}
