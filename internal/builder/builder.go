package builder

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"fmt"
	"os"

	"github.com/PeteMadCoder/vpok/internal/manifest"
	"github.com/PeteMadCoder/vpok/internal/store"
)

// Builder coordinates the build pipeline.
type Builder struct {
	Store      store.Store
	PrivateKey ed25519.PrivateKey
}

func NewBuilder(s store.Store, privKey ed25519.PrivateKey) *Builder {
	return &Builder{
		Store:      s,
		PrivateKey: privKey,
	}
}

// Build compiles a BuildSpec and build context directory into a package stored in the CAS store.
func (b *Builder) Build(ctx context.Context, spec *manifest.BuildSpec, contextDir string) (*manifest.Manifest, store.Digest, error) {
	if err := manifest.ValidateBuild(spec); err != nil {
		return nil, "", fmt.Errorf("build spec validation failed: %w", err)
	}

	m, digest, err := b.buildOnce(ctx, spec, contextDir)
	if err != nil {
		return nil, "", err
	}

	// Verify reproducibility if requested
	if spec.Reproducible != nil && spec.Reproducible.Enabled {
		_, secondDigest, err := b.buildOnce(ctx, spec, contextDir)
		if err != nil {
			return nil, "", fmt.Errorf("reproducibility check second build failed: %w", err)
		}
		if digest != secondDigest {
			return nil, "", fmt.Errorf("reproducibility violation: build output non-deterministic (%s != %s)", digest, secondDigest)
		}
	}

	return m, digest, nil
}

func (b *Builder) buildOnce(ctx context.Context, spec *manifest.BuildSpec, contextDir string) (*manifest.Manifest, store.Digest, error) {
	stageDir, err := os.MkdirTemp("", "vpok-build-stage-*")
	if err != nil {
		return nil, "", fmt.Errorf("failed to create staging directory: %w", err)
	}
	defer os.RemoveAll(stageDir)

	env := make(map[string]string)

	// Execute build steps
	if spec.Build != nil {
		for i, step := range spec.Build.Steps {
			if err := ExecuteStep(ctx, step, contextDir, stageDir, env); err != nil {
				return nil, "", fmt.Errorf("step %d (%s) failed: %w", i, step.Action, err)
			}
		}
	}

	// Archive the stage directory into a reproducible layer tar
	var tarBuf bytes.Buffer
	if err := CreateReproducibleLayerTar(stageDir, &tarBuf); err != nil {
		return nil, "", fmt.Errorf("failed to create layer tar archive: %w", err)
	}

	layerDigest, layerSize, err := b.Store.PutBlob(ctx, &tarBuf)
	if err != nil {
		return nil, "", fmt.Errorf("failed to store layer blob: %w", err)
	}

	// Construct Manifest
	pkgManifest := &manifest.Manifest{
		APIVersion: spec.APIVersion,
		Kind:       "Manifest",
		Metadata:   spec.Metadata,
		Base:       spec.Base,
		Layers: []manifest.LayerRef{
			{
				Digest:    layerDigest.String(),
				Size:      layerSize,
				MediaType: "application/vnd.vpok.layer.v1.tar",
			},
		},
		BuildSpec:  *spec,
		EntryPoint: spec.EntryPoint,
		Requires:   spec.Requires,
		Service:    spec.Service,
	}

	// Sign manifest if private key is configured
	if len(b.PrivateKey) == ed25519.PrivateKeySize {
		if err := manifest.SignManifest(pkgManifest, spec.Metadata.Publisher, b.PrivateKey); err != nil {
			return nil, "", fmt.Errorf("failed to sign package manifest: %w", err)
		}
	}

	// Save Manifest in CAS Store
	manifestDigest, err := b.Store.PutManifest(ctx, pkgManifest)
	if err != nil {
		return nil, "", fmt.Errorf("failed to store manifest in CAS: %w", err)
	}

	return pkgManifest, manifestDigest, nil
}
