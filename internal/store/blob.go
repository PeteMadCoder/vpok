package store

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
)

type verifyingReadCloser struct {
	rc       io.ReadCloser
	hasher   hash.Hash
	expected string
}

func (v *verifyingReadCloser) Read(p []byte) (int, error) {
	n, err := v.rc.Read(p)
	if n > 0 {
		v.hasher.Write(p[:n])
	}
	if err == io.EOF {
		actual := hex.EncodeToString(v.hasher.Sum(nil))
		if actual != v.expected {
			return n, ErrDigestMismatch
		}
	}
	return n, err
}

func (v *verifyingReadCloser) Close() error {
	return v.rc.Close()
}

func (s *CASStore) PutBlob(ctx context.Context, r io.Reader) (Digest, int64, error) {
	if err := ctx.Err(); err != nil {
		return "", 0, err
	}

	tmpFile, err := os.CreateTemp(s.layout.TmpDir(), "blob-*")
	if err != nil {
		return "", 0, fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	hasher := sha256.New()
	writer := io.MultiWriter(tmpFile, hasher)

	written, err := io.Copy(writer, r)
	if err != nil {
		tmpFile.Close()
		return "", 0, fmt.Errorf("failed to stream blob content: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return "", 0, fmt.Errorf("failed to sync temp blob: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		return "", 0, fmt.Errorf("failed to close temp blob: %w", err)
	}

	digest := NewDigest(SHA256, hex.EncodeToString(hasher.Sum(nil)))
	destPath, err := s.layout.BlobPath(digest)
	if err != nil {
		return "", 0, err
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return "", 0, fmt.Errorf("failed to create blob parent directory: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return "", 0, fmt.Errorf("failed to move blob to destination: %w", err)
	}

	return digest, written, nil
}

func (s *CASStore) GetBlob(ctx context.Context, d Digest) (io.ReadCloser, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path, err := s.layout.BlobPath(d)
	if err != nil {
		return nil, err
	}

	file, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrBlobNotFound
		}
		return nil, fmt.Errorf("failed to open blob: %w", err)
	}

	return &verifyingReadCloser{
		rc:       file,
		hasher:   sha256.New(),
		expected: d.Hex(),
	}, nil
}

func (s *CASStore) HasBlob(ctx context.Context, d Digest) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}

	path, err := s.layout.BlobPath(d)
	if err != nil {
		return false, err
	}

	_, err = os.Stat(path)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, fmt.Errorf("failed to check blob existence: %w", err)
}
