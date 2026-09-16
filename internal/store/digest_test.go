package store

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"strings"
	"testing"
)

func TestDigestValidation(t *testing.T) {
	validHex := strings.Repeat("a", 64)
	validDigest := Digest("sha256:" + validHex)

	if err := validDigest.Validate(); err != nil {
		t.Fatalf("expected valid digest, got: %v", err)
	}

	if validDigest.Algorithm() != SHA256 {
		t.Errorf("expected algorithm %q, got %q", SHA256, validDigest.Algorithm())
	}

	if validDigest.Hex() != validHex {
		t.Errorf("expected hex %q, got %q", validHex, validDigest.Hex())
	}

	invalidTests := []struct {
		name   string
		digest Digest
	}{
		{"empty", Digest("")},
		{"no colon", Digest("sha256" + validHex)},
		{"wrong algorithm", Digest("md5:" + validHex)},
		{"too short hex", Digest("sha256:abcd")},
		{"too long hex", Digest("sha256:" + validHex + "a")},
		{"invalid characters", Digest("sha256:" + strings.Repeat("g", 64))},
		{"uppercase characters", Digest("sha256:" + strings.Repeat("A", 64))},
	}

	for _, tc := range invalidTests {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.digest.Validate(); err == nil {
				t.Fatalf("expected error for %q, got nil", tc.digest)
			}
		})
	}
}

func TestFromBytesAndFromReader(t *testing.T) {
	data := []byte("hello vpok store")
	expectedHash := sha256.Sum256(data)
	expectedDigest := Digest(fmt.Sprintf("sha256:%x", expectedHash))

	d1 := FromBytes(data)
	if d1 != expectedDigest {
		t.Errorf("FromBytes got %s, want %s", d1, expectedDigest)
	}

	d2, n, err := FromReader(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("FromReader failed: %v", err)
	}
	if n != int64(len(data)) {
		t.Errorf("FromReader length got %d, want %d", n, len(data))
	}
	if d2 != expectedDigest {
		t.Errorf("FromReader got %s, want %s", d2, expectedDigest)
	}
}
