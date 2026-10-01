package image

import (
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// EpochTime ensures bit-for-bit reproducible timestamps in initrd
var EpochTime = time.Unix(0, 0).UTC()

// CPIOHeader represents a CPIO "newc" format header (RFC SVR4 portable cpio format with CRC/newc)
type cpioWriter struct {
	w       io.Writer
	written int64
	ino     uint32
}

func newCPIOWriter(w io.Writer) *cpioWriter {
	return &cpioWriter{w: w, ino: 1}
}

func (c *cpioWriter) writeHeader(name string, mode uint32, size int64, linkTarget string) error {
	c.ino++
	nameWithNull := name + "\x00"
	nameLen := len(nameWithNull)

	// Format: newc cpio magic is "070701
	// Each header field is 8 hex digits, except magic (6)
	hdr := fmt.Sprintf(
		"070701%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X",
		c.ino,   // c_ino
		mode,    // c_mode
		0,       // c_uid (root)
		0,       // c_gid (root)
		1,       // c_nlink
		0,       // c_mtime (normalized to 0)
		size,    // c_filesize
		0,       // c_maj
		0,       // c_min
		0,       // c_rmaj
		0,       // c_rmin
		nameLen, // c_namesize
		0,       // c_check
	)

	if _, err := io.WriteString(c.w, hdr); err != nil {
		return err
	}

	if _, err := io.WriteString(c.w, nameWithNull); err != nil {
		return err
	}

	// Pad header + name to 4-byte boundary: (110 + nameLen + pad) % 4 == 0
	pad := (4 - ((110 + nameLen) % 4)) % 4
	if pad > 0 {
		if _, err := c.w.Write(make([]byte, pad)); err != nil {
			return err
		}
	}

	return nil
}

func (c *cpioWriter) writeData(r io.Reader, size int64) error {
	if _, err := io.CopyN(c.w, r, size); err != nil {
		return err
	}
	// Pad data to 4-byte boundary
	pad := (4 - (size % 4)) % 4
	if pad > 0 {
		if _, err := c.w.Write(make([]byte, pad)); err != nil {
			return err
		}
	}

	return nil
}

func (c *cpioWriter) writeTrailer() error {
	if err := c.writeHeader("TRAILER!!!", 0, 0, ""); err != nil {
		return err
	}
	// Pad end to 512-byte boundary
	return nil
}

// BuildInitrd creates a gzip-compressed cpio newc archive of the given rootfs directory
func BuildInitrd(rootfsDir string, outPath string) error {
	outFile, err := os.OpenFile(outPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("failed to create initrd file: %w", err)
	}
	defer outFile.Close()

	gw := gzip.NewWriter(outFile)
	defer gw.Close()

	cw := newCPIOWriter(gw)

	var entries []string
	err = filepath.Walk(rootfsDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if path == rootfsDir {
			return nil
		}
		entries = append(entries, path)
		return nil
	})
	if err != nil {
		return fmt.Errorf("failed to scan rootfs directory: %w", err)
	}

	// Lexicographical ordering for deterministic builds
	sort.Strings(entries)

	for _, path := range entries {
		info, err := os.Lstat(path)
		if err != nil {
			return fmt.Errorf("failed to stat %s: %w", path, err)
		}

		relPath, err := filepath.Rel(rootfsDir, path)
		if err != nil {
			return fmt.Errorf("failed to get rel path: %w", err)
		}

		relPath = filepath.ToSlash(relPath)

		mode := uint32(info.Mode().Perm())
		var size int64
		var linkTarget string

		switch {
		case info.Mode()&os.ModeSymlink != 0:
			mode |= 0120000 // S_IFLNK
			linkTarget, err = os.Readlink(path)
			if err != nil {
				return fmt.Errorf("failed to read symlink %s: %w", path, err)
			}
			size = int64(len(linkTarget))

		case info.IsDir():
			mode |= 0040000 // S_IFDIR
			size = 0

		case info.Mode().IsRegular():
			mode |= 0100000 // S_IFREG
			size = info.Size()

		default:
			continue
		}

		if err := cw.writeHeader(relPath, mode, size, linkTarget); err != nil {
			return fmt.Errorf("failed to write cpio header for %s: %w", relPath, err)
		}

		if info.Mode()&os.ModeSymlink != 0 {
			if err := cw.writeData(io.NopCloser(nil), 0); err != nil {
				return err
			}
			if _, err := io.WriteString(gw, linkTarget); err != nil {
				return err
			}
			pad := (4 - (len(linkTarget) % 4)) % 4
			if pad > 0 {
				if _, err := gw.Write(make([]byte, pad)); err != nil {
					return err
				}
			}
		} else if info.Mode().IsRegular() {
			f, err := os.Open(path)
			if err != nil {
				return fmt.Errorf("failed to open file %s: %w", path, err)
			}
			err = cw.writeData(f, size)
			f.Close()
			if err != nil {
				return fmt.Errorf("failed to write file data for %s: %w", relPath, err)
			}
		}
	}

	if err := cw.writeTrailer(); err != nil {
		return fmt.Errorf("failed to write cpio trailer: %w", err)
	}

	return nil
}
