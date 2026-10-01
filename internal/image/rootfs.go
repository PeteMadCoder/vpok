package image

import (
	"archive/tar"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/PeteMadCoder/vpok/internal/store"
)

// ExtractTar unpacks a tar stream into the target directory with path sanitization.
func ExtractTar(r io.Reader, destDir string) error {
	tr := tar.NewReader(r)

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read error: %w", err)
		}

		cleaned := filepath.Clean(hdr.Name)
		if strings.HasPrefix(cleaned, "..") || filepath.IsAbs(cleaned) {
			return fmt.Errorf("illegal tar path escapes root: %s", hdr.Name)
		}

		targetPath := filepath.Join(destDir, cleaned)

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, os.FileMode(hdr.Mode&0777)); err != nil {
				return fmt.Errorf("failed to create directory %s: %w", targetPath, err)
			}

		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("failed to create parent dir for %s: %w", targetPath, err)
			}
			f, err := os.OpenFile(targetPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode&0777))
			if err != nil {
				return fmt.Errorf("failed to create regular file %s: %w", targetPath, err)
			}
			_, copyErr := io.Copy(f, tr)
			f.Close()
			if copyErr != nil {
				return fmt.Errorf("failed to write content to %s: %w", targetPath, copyErr)
			}

		case tar.TypeSymlink:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0755); err != nil {
				return fmt.Errorf("failed to create parent dir for symlink %s: %w", targetPath, err)
			}
			_ = os.Remove(targetPath)
			if err := os.Symlink(hdr.Linkname, targetPath); err != nil {
				return fmt.Errorf("failed to create symlink %s -> %s: %w", targetPath, hdr.Linkname, err)
			}

		default:
			// Ignore unsupported tar entry types gracefully
			continue
		}
	}

	return nil
}

// ExtractLayers extracts a list of CAS store layer digests into destDir in sequence.
func ExtractLayers(ctx context.Context, s store.Store, layerDigests []string, destDir string) error {
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return fmt.Errorf("failed to initialize rootfs directory %s: %w", destDir, err)
	}

	for i, digStr := range layerDigests {
		d := store.Digest(digStr)
		rc, err := s.GetBlob(ctx, d)
		if err != nil {
			return fmt.Errorf("layer [%d] %s not found in store: %w", i, digStr, err)
		}

		err = ExtractTar(rc, destDir)
		rc.Close()
		if err != nil {
			return fmt.Errorf("failed to extract layer [%d] %s: %w", i, digStr, err)
		}
	}

	return nil
}
