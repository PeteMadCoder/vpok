package manifest

import (
	"errors"
	"strings"
	"testing"
)

func sampleManifest() *Manifest {
	return &Manifest{
		APIVersion: "vpok.io/v1",
		Kind:       "Manifest",
		Metadata: BuildMetadata{
			Name:      "sample-app",
			Version:   "1.0.0",
			Publisher: "example.org",
		},
		Base: Base{
			Distribution: "alpine",
			Release:      "3.20",
			Architecture: "amd64",
		},
		EntryPoint: EntryPoint{
			Command: []string{"/app/start"},
		},
		Requires: Requires{
			Resources: RequireResources{
				Memory: "256MiB",
				CPUs:   1,
			},
		},
	}
}

func TestSignAndVerifyManifest(t *testing.T) {
	pubKey, privKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	m := sampleManifest()

	// Sign manifest
	publisher := "example.org"
	if err := SignManifest(m, publisher, privKey); err != nil {
		t.Fatalf("SignManifest failed: %v", err)
	}

	if len(m.Signatures) != 1 {
		t.Fatalf("expected 1 signature, got %d", len(m.Signatures))
	}
	if m.Signatures[0].Publisher != publisher {
		t.Errorf("expected publisher %q, got %q", publisher, m.Signatures[0].Publisher)
	}
	if m.Signatures[0].Signature == "" || m.Signatures[0].KeyID == "" {
		t.Errorf("expected non-empty signature and keyId: %+v", m.Signatures[0])
	}

	// Verify manifest
	if err := VerifyManifest(m, publisher, pubKey); err != nil {
		t.Fatalf("VerifyManifest failed: %v", err)
	}
}

func TestVerifyManifest_Tampering(t *testing.T) {
	pubKey, privKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	publisher := "example.org"
	m := sampleManifest()

	if err := SignManifest(m, publisher, privKey); err != nil {
		t.Fatalf("SignManifest failed: %v", err)
	}

	// Tamper with metadata name after signing
	m.Metadata.Name = "tampered-app"

	err = VerifyManifest(m, publisher, pubKey)
	if !errors.Is(err, ErrSignatureInvalid) {
		t.Fatalf("expected ErrSignatureInvalid on tampered manifest, got: %v", err)
	}
}

func TestVerifyManifest_WrongKey(t *testing.T) {
	_, privKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair 1 failed: %v", err)
	}
	pubKey2, _, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair 2 failed: %v", err)
	}

	publisher := "example.org"
	m := sampleManifest()

	if err := SignManifest(m, publisher, privKey); err != nil {
		t.Fatalf("SignManifest failed: %v", err)
	}

	// Verify with a different public key
	err = VerifyManifest(m, publisher, pubKey2)
	if err == nil {
		t.Fatal("expected verification to fail with wrong public key")
	}
}

func TestVerifyManifest_UnknownPublisher(t *testing.T) {
	pubKey, privKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("GenerateKeyPair failed: %v", err)
	}

	m := sampleManifest()
	if err := SignManifest(m, "example.org", privKey); err != nil {
		t.Fatalf("SignManifest failed: %v", err)
	}

	err = VerifyManifest(m, "other.org", pubKey)
	if !errors.Is(err, ErrSignatureNotFound) {
		t.Fatalf("expected ErrSignatureNotFound, got: %v", err)
	}
}

func TestCanonicalDigest(t *testing.T) {
	m := sampleManifest()

	d1, err := CanonicalDigest(m)
	if err != nil {
		t.Fatalf("CanonicalDigest failed: %v", err)
	}

	if !strings.HasPrefix(d1, "sha256:") || len(d1) != 71 {
		t.Fatalf("invalid canonical digest format: %q", d1)
	}

	// Deterministic digest
	d2, err := CanonicalDigest(m)
	if err != nil {
		t.Fatalf("second CanonicalDigest failed: %v", err)
	}

	if d1 != d2 {
		t.Fatalf("expected deterministic digest: %s != %s", d1, d2)
	}
}
