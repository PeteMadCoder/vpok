package store

import (
	"context"
	"io"

	"github.com/PeteMadCoder/vpok/internal/manifest"
)

type Store interface {
	PutBlob(ctx context.Context, r io.Reader) (Digest, int64, error)
	GetBlob(ctx context.Context, d Digest) (io.ReadCloser, error)
	HasBlob(ctx context.Context, d Digest) (bool, error)
	PutManifest(ctx context.Context, m *manifest.Manifest) (Digest, error)
	GetManifest(ctx context.Context, d Digest) (*manifest.Manifest, error)
	ListManifests(ctx context.Context) ([]Digest, error)
	GC(ctx context.Context, keep []Digest) error
}

type CASStore struct {
	layout *Layout
}

func NewCASStore(root string) (*CASStore, error) {
	layout := NewLayout(root)
	if err := layout.EnsureDirs(); err != nil {
		return nil, err
	}
	return &CASStore{layout: layout}, nil
}

func (s *CASStore) GC(ctx context.Context, keep []Digest) error {
	// GC implementation will be added once refs/ layout is in place
	return nil
}
