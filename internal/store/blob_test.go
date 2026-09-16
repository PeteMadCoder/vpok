package store

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"io"
	"os"
	"sync"
	"testing"
)

func TestBlobRoundtrip(t *testing.T) {
	root := t.TempDir()
	store, err := NewCASStore(root)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	content := []byte("virtual package orchestration kit layer blob data")

	digest, size, err := store.PutBlob(ctx, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PutBlob failed: %v", err)
	}

	if size != int64(len(content)) {
		t.Fatalf("PutBlob returned size %d, want %d", size, len(content))
	}

	has, err := store.HasBlob(ctx, digest)
	if err != nil {
		t.Fatalf("HasBlob failed: %v", err)
	}
	if !has {
		t.Fatalf("expected HasBlob to return true")
	}

	rc, err := store.GetBlob(ctx, digest)
	if err != nil {
		t.Fatalf("GetBlob failed: %v", err)
	}
	defer rc.Close()

	readBack, err := io.ReadAll(rc)
	if err != nil {
		t.Fatalf("failed to read blob: %v", err)
	}

	if !bytes.Equal(readBack, content) {
		t.Fatalf("read back content does not match original")
	}
}

func TestBlobPropertyRoundtrip(t *testing.T) {
	root := t.TempDir()
	store, err := NewCASStore(root)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()

	// Run property tests across varying payload sizes
	sizes := []int{0, 1, 15, 64, 1024, 64*1024 + 7, 256 * 1024}
	for _, sz := range sizes {
		data := make([]byte, sz)
		if sz > 0 {
			if _, err := rand.Read(data); err != nil {
				t.Fatalf("rand.Read failed: %v", err)
			}
		}

		d1, _, err := store.PutBlob(ctx, bytes.NewReader(data))
		if err != nil {
			t.Fatalf("PutBlob failed for size %d: %v", sz, err)
		}

		rc, err := store.GetBlob(ctx, d1)
		if err != nil {
			t.Fatalf("GetBlob failed for size %d: %v", sz, err)
		}

		// PutBlob(GetBlob(d)) == d
		d2, _, err := store.PutBlob(ctx, rc)
		rc.Close()
		if err != nil {
			t.Fatalf("second PutBlob failed for size %d: %v", sz, err)
		}

		if d1 != d2 {
			t.Fatalf("PutBlob(GetBlob(d)) digest mismatch: %s != %s", d1, d2)
		}
	}
}

func TestBlobVerifyOnReadCorruption(t *testing.T) {
	root := t.TempDir()
	store, err := NewCASStore(root)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	content := []byte("clean uncorrupted data payload")

	digest, _, err := store.PutBlob(ctx, bytes.NewReader(content))
	if err != nil {
		t.Fatalf("PutBlob failed: %v", err)
	}

	path, err := store.layout.BlobPath(digest)
	if err != nil {
		t.Fatalf("BlobPath failed: %v", err)
	}

	// Corrupt the file on disk
	if err := os.WriteFile(path, []byte("corrupted payload content!!!"), 0644); err != nil {
		t.Fatalf("failed to corrupt blob file: %v", err)
	}

	rc, err := store.GetBlob(ctx, digest)
	if err != nil {
		t.Fatalf("GetBlob unexpectedly failed to open: %v", err)
	}
	defer rc.Close()

	_, err = io.ReadAll(rc)
	if !errors.Is(err, ErrDigestMismatch) {
		t.Fatalf("expected ErrDigestMismatch on reading corrupted blob, got: %v", err)
	}
}

func TestBlobNotFound(t *testing.T) {
	root := t.TempDir()
	store, err := NewCASStore(root)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	nonExistent := FromBytes([]byte("non-existent-content"))

	has, err := store.HasBlob(ctx, nonExistent)
	if err != nil {
		t.Fatalf("HasBlob failed: %v", err)
	}
	if has {
		t.Errorf("expected HasBlob to return false for non-existent digest")
	}

	_, err = store.GetBlob(ctx, nonExistent)
	if !errors.Is(err, ErrBlobNotFound) {
		t.Fatalf("expected ErrBlobNotFound, got: %v", err)
	}
}

func TestBlobConcurrentPuts(t *testing.T) {
	root := t.TempDir()
	store, err := NewCASStore(root)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}

	ctx := context.Background()
	var wg sync.WaitGroup
	workers := 10

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			content := []byte("shared concurrent payload across workers")
			d, _, putErr := store.PutBlob(ctx, bytes.NewReader(content))
			if putErr != nil {
				t.Errorf("worker %d PutBlob failed: %v", workerID, putErr)
				return
			}
			has, hasErr := store.HasBlob(ctx, d)
			if hasErr != nil || !has {
				t.Errorf("worker %d HasBlob failed: %v", workerID, hasErr)
			}
		}(i)
	}
	wg.Wait()
}

func FuzzBlobRoundtrip(f *testing.F) {
	root := f.TempDir()
	store, err := NewCASStore(root)
	if err != nil {
		f.Fatalf("failed to create store: %v", err)
	}

	f.Add([]byte("sample seed bytes for cas store fuzzing"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		ctx := context.Background()
		d, size, err := store.PutBlob(ctx, bytes.NewReader(data))
		if err != nil {
			t.Fatalf("PutBlob failed: %v", err)
		}
		if size != int64(len(data)) {
			t.Fatalf("size mismatch: got %d, want %d", size, len(data))
		}

		rc, err := store.GetBlob(ctx, d)
		if err != nil {
			t.Fatalf("GetBlob failed: %v", err)
		}
		defer rc.Close()

		readData, err := io.ReadAll(rc)
		if err != nil {
			t.Fatalf("ReadAll failed: %v", err)
		}

		if !bytes.Equal(readData, data) {
			t.Fatalf("data mismatch")
		}
	})
}
