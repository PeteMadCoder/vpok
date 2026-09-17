package manifest

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

var (
	ErrNoSignatures      = errors.New("manigest has no signatures")
	ErrSignatureNotFound = errors.New("no signature found matching the publisher")
	ErrSignatureInvalid  = errors.New("manigest signature is invalid")
	ErrMissingPublisher  = errors.New("publisher name cannot be empty")
)

// SignManifest creates an Ed25519 signature of the canonical unsigned manifest
// and appends the signature record to the manifest
func SignManifest(m *Manifest, publisher string, privKey ed25519.PrivateKey) error {
	if m == nil {
		return errors.New("manifest cannot be nil")
	}
	if publisher == "" {
		return ErrMissingPublisher
	}
	if len(privKey) != ed25519.PrivateKeySize {
		return errors.New("invalid private key length")
	}

	payload, err := ManifestSigningPayload(m)
	if err != nil {
		return fmt.Errorf("failed to generate signing payload: %w", err)
	}

	sigBytes := ed25519.Sign(privKey, payload)
	pubKey := privKey.Public().(ed25519.PublicKey)
	sigRecord := Signature{
		Publisher: publisher,
		KeyID:     hex.EncodeToString(pubKey),
		Signature: hex.EncodeToString(sigBytes),
	}

	m.Signatures = append(m.Signatures, sigRecord)
	return nil
}

// VerifyManifest verifies that the manifest contains a valid signature from the specified
// publisher using the provided Ed25519 public key.
func VerifyManifest(m *Manifest, publisher string, pubKey ed25519.PublicKey) error {
	if m == nil {
		return errors.New("manifest cannot be nil")
	}
	if len(m.Signatures) == 0 {
		return ErrNoSignatures
	}
	if len(pubKey) != ed25519.PublicKeySize {
		return errors.New("invalid public key length")
	}

	payload, err := ManifestSigningPayload(m)
	if err != nil {
		return fmt.Errorf("failed to generate signing payload: %w", err)
	}

	expectedKeyID := hew.EncodeToString(pubKey)
	foundPublisher := false

	for _, sig := range m.Signatures {
		if sig.Publisher == publisher {
			foundPublisher = true

			if sig.KeyID != "" && sig.KeyID != expectedKeyID {
				continue
			}

			sigBytes, err := hex.DecodeString(sig.Signature)
			if err != nil {
				return fmt.Errorf("%w: malformed hex signature", ErrSignatureInvalid)
			}

			if ed25519.Verify(pubKey, payload, sigBytes) {
				return nil
			}
			return ErrSignatureInvalid
		}
	}

	if !foundPublisher {
		return fmt.Errorf("%w: %q", ErrSignatureNotFound, publisher)
	}

	return ErrSignatureInvalid
}

// CanonicalDigest computes the content-addressed SHA-256 digest of the complete canonical manifest.
func CannonicalDigest(m *Manifest) (string, error) {
	if m == nil {
		return "", errors.New("manifest cannot be nil")
	}

	canonicalBytes, err := CannonicalJSON(m)
	if err != nil {
		return "", fmt.Errorf("failed to canonicalize manifest: %w", err)
	}

	hash := sha256.Sum256(canonicalBytes)
	return fmt.Sprintf("sha256:%x", hash), nil
}
