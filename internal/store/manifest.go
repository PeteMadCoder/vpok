package store

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/PeteMadCoder/vpok/internal/manifest"
)

func (s *CASStore) PutManifest(ctx context.Context, m *manifest.Manifest) (Digest, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if m == nil {
		return "", errors.New("manifest cannot be nil")
	}

	data, err := json.Marshal(m)
	if err != nil {
		return "", fmt.Errorf("failed to serialize manifest: %w", err)
	}

	h := sha256.Sum256(data)
	digest := NewDigest(SHA256, hex.EncodeToString(h[:]))

	tmpFile, err := os.CreateTemp(s.layout.TmpDir(), "manifest-*")

	if err != nil {
		return "", fmt.Errorf("failed to create temp manifest file: %w", err)
	}

	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	if _, err := io.Copy(tmpFile, bytes.NewReader(data)); err != nil {
		return "", fmt.Errorf("failed to write manifest: %w", err)
	}

	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("failed to close temp manifest: %w", err)
	}

	destPath, err := s.layout.ManifestPath(digest)
	if err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
		return "", fmt.Errorf("failed to create manifest parent directory: %w", err)
	}

	if err := os.Rename(tmpPath, destPath); err != nil {
		return "", fmt.Errorf("failed to move manifest to destination: %w", err)
	}

	return digest, nil
}

func (s *CASStore) GetManifest(ctx context.Context, d Digest) (*manifest.Manifest, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	path, err := s.layout.ManifestPath(d)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrManifestNotFound
		}
		return nil, fmt.Errorf("failed to read manifest file: %w", err)
	}

	h := sha256.Sum256(data)
	actualHex := hex.EncodeToString(h[:])
	if actualHex != d.Hex() {
		return nil, ErrDigestMismatch
	}

	var m manifest.Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal manifest: %w", err)
	}

	return &m, nil
}
