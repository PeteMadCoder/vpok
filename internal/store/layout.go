package store

import (
	"os"
	"path/filepath"
)

type Layout struct {
	Root string
}

func NewLayout(root string) *Layout {
	return &Layout{Root: root}
}

func (l *Layout) BlobsDir() string {
	return filepath.Join(l.Root, "blobs")
}

func (l *Layout) ManifestsDir() string {
	return filepath.Join(l.Root, "manifests")
}

func (l *Layout) TmpDir() string {
	return filepath.Join(l.Root, "tmp")
}

func (l *Layout) BlobPath(d Digest) (string, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}
	h := d.Hex()
	return filepath.Join(l.Root, "blobs", d.Algorithm(), h[:2], h[2:]), nil
}

func (l *Layout) ManifestPath(d Digest) (string, error) {
	if err := d.Validate(); err != nil {
		return "", err
	}
	h := d.Hex()
	return filepath.Join(l.Root, "manifests", d.Algorithm(), h[:2], h[2:]), nil
}

func (l *Layout) EnsureDirs() error {
	dirs := []string{
		filepath.Join(l.Root, "blobs", SHA256),
		filepath.Join(l.Root, "manifests", SHA256),
		filepath.Join(l.Root, "tmp"),
	}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return nil
}
