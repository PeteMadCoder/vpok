package builder

import (
	"archive/tar"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// EpochTime is used to normalize timestamps for build reproducibility
var EpochTime = time.Unix(0, 0).UTC()

// CreateReproducibleLayerTar archives the contents of srcDir into a deterministic tar stream.
// File entries are lexicographically sorted and timestamps/ownership are normalized
func CreateReproducibleLayerTar(srcDir string, w io.Writer) error {
	tw := tar.NewWriter(w)
	defer tw.Close()

	var entries []string
	err := filepath.Walk(srcDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == srcDir {
			return nil
		}
		entries = append(entries, path)
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan layer files: %w", err)
	}

	// Lixicographical sort guarantees identical tar header order
	sort.Strings(entries)

	for _, path := range entries {
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("failed to stat %q: %w", path, err)
		}

		relPath, err := filepath.Rel(srcDir, path)
		if err != nil {
			return fmt.Errorf("failed to get relative path for %q:%w", path, err)
		}

		linkTarget := ""
		if info.Mode()&os.ModeSymlink != 0 {
			linkTarget, err = os.Readlink(path)
			if err != nil {
				return fmt.Errorf("failed to read symlink %q:%w", path, err)
			}
		}

		hdr, err := tar.FileInfoHeader(info, linkTarget)
		if err != nil {
			return fmt.Errorf("failed to create tar header for %q: %w", path, err)
		}

		// Normalize headers for bit-for-bit reproducibility
		hdr.Name = filepath.ToSlash(relPath)
		hdr.ModTime = EpochTime
		hdr.AccessTime = EpochTime
		hdr.ChangeTime = EpochTime
		hdr.Uid = 0
		hdr.Gid = 0
		hdr.Uname = "root"
		hdr.Gname = "root"

		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("failed to write tar header for %q: %w", path, err)
		}

		if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file %q: %w", path, err)
			}
			_, copyErr := io.Copy(tw, f)
			f.Close()
			if copyErr != nil {
				return fmt.Errorf("failed to copy file content for %q: %w", path, copyErr)
			}
		}
	}

	return nil
}
