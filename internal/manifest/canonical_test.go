package manifest

import (
	"bytes"
	"testing"
)

func TestCanonicalJSON_DeterminismAndSorting(t *testing.T) {
	// Map with unordered keys
	obj1 := map[string]any{
		"z": "last",
		"a": "first",
		"m": "middle",
		"nested": map[string]any{
			"beta":  2,
			"alpha": 1,
		},
	}

	obj2 := map[string]any{
		"a": "first",
		"nested": map[string]any{
			"alpha": 1,
			"beta":  2,
		},
		"m": "middle",
		"z": "last",
	}

	canon1, err := CanonicalJSON(obj1)
	if err != nil {
		t.Fatalf("CanonicalJSON(obj1) failed: %v", err)
	}

	canon2, err := CanonicalJSON(obj2)
	if err != nil {
		t.Fatalf("CanonicalJSON(obj2) failed: %v", err)
	}

	if !bytes.Equal(canon1, canon2) {
		t.Fatalf("expected identical canonical JSON, got:\n%s\nvs\n%s", string(canon1), string(canon2))
	}

	expected := `{"a":"first","m":"middle","nested":{"alpha":1,"beta":2},"z":"last"}`
	if string(canon1) != expected {
		t.Errorf("CanonicalJSON output got %q, want %q", string(canon1), expected)
	}
}

func TestManifestSigningPayload_OmitsSignatures(t *testing.T) {
	m := &Manifest{
		APIVersion: "vpok.io/v1",
		Kind:       "Manifest",
		Metadata: BuildMetadata{
			Name:      "test-pkg",
			Version:   "1.0.0",
			Publisher: "example.org",
		},
		Signatures: []Signature{
			{
				Publisher: "example.org",
				KeyID:     "deadbeef",
				Signature: "cafe1234",
			},
		},
	}

	payloadWithSig, err := ManifestSigningPayload(m)
	if err != nil {
		t.Fatalf("ManifestSigningPayload failed: %v", err)
	}

	// Create identical manifest without signatures
	mWithoutSig := &Manifest{
		APIVersion: "vpok.io/v1",
		Kind:       "Manifest",
		Metadata: BuildMetadata{
			Name:      "test-pkg",
			Version:   "1.0.0",
			Publisher: "example.org",
		},
	}

	payloadWithoutSig, err := ManifestSigningPayload(mWithoutSig)
	if err != nil {
		t.Fatalf("ManifestSigningPayload without sig failed: %v", err)
	}

	if !bytes.Equal(payloadWithSig, payloadWithoutSig) {
		t.Fatalf("signing payload must be invariant to signatures field:\n%s\nvs\n%s", string(payloadWithSig), string(payloadWithoutSig))
	}

	if bytes.Contains(payloadWithSig, []byte("signatures")) {
		t.Errorf("signing payload should not contain 'signatures' field: %s", string(payloadWithSig))
	}
}
