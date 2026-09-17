package store

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"strings"
)

var (
	ErrInvalidDigest    = errors.New("invalid digest format")
	ErrDigestMismatch   = errors.New("digest mismatch: content is corrupt")
	ErrBlobNotFound     = errors.New("blob not found")
	ErrManifestNotFound = errors.New("manifest not found")
)

const (
	SHA256 = "sha256"
)

// Digest represents a content address in the format algorithm:hex (e.g. sha256:abc...)
type Digest string

func NewDigest(algo string, hexDigest string) Digest {
	return Digest(algo + ":" + hexDigest)
}

func FromBytes(b []byte) Digest {
	h := sha256.Sum256(b)
	return Digest(fmt.Sprintf("%s:%x", SHA256, h))
}

func FromReader(r io.Reader) (Digest, int64, error) {
	h := sha256.New()
	n, err := io.Copy(h, r)
	if err != nil {
		return "", 0, err
	}
	return Digest(fmt.Sprintf("%s:%x", SHA256, h.Sum(nil))), n, nil
}

func (d Digest) String() string {
	return string(d)
}

func (d Digest) Validate() error {
	parts := strings.SplitN(string(d), ":", 2)
	if len(parts) != 2 {
		return fmt.Errorf("%w: missing colon separator", ErrInvalidDigest)
	}
	if parts[0] != SHA256 {
		return fmt.Errorf("%w: unsupported algorithm %q", ErrInvalidDigest, parts[0])
	}
	if len(parts[1]) != 64 {
		return fmt.Errorf("%w: invalid sha256 length (%d)", ErrInvalidDigest, len(parts[1]))
	}
	for _, c := range parts[1] {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return fmt.Errorf("%w: invalid hex character %q", ErrInvalidDigest, c)
		}
	}
	return nil
}

func (d Digest) Algorithm() string {
	parts := strings.SplitN(string(d), ":", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[0]
}

func (d Digest) Hex() string {
	parts := strings.SplitN(string(d), ":", 2)
	if len(parts) != 2 {
		return string(d)
	}
	return parts[1]
}
