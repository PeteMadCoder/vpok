package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
)

// CanonicalJSON marshals any JSON-compatible value into RFC 8785 canonical JSON format.
// Properties are lexicographically sorted by UTF-16 code unit representation,
// and all non-essential whitespace is omitted.
func CanonicalJSON(v any) ([]byte, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal value: %w", err)
	}

	var raw any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&raw); err != nil {
		return nil, fmt.Errorf("failed to decode raw json: %w", err)
	}

	var buf bytes.Buffer
	if err := writeCanonical(&buf, raw); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// ManifestSigningPayload produces the canonical JSON representation of a manifest
// with its signatures omitted, guaranteeing a deterministic payload for signing.
func ManifestSigningPayload(m *Manifest) ([]byte, error) {
	if m == nil {
		return nil, errors.New("manifest cannot be nil")
	}

	// Create a copy without signatures
	clone := *m
	clone.Signatures = nil

	return CanonicalJSON(&clone)
}

func writeCanonical(buf *bytes.Buffer, v any) error {
	switch val := v.(type) {
	case nil:
		buf.WriteString("null")
	case bool:
		if val {
			buf.WriteString("true")
		} else {
			buf.WriteString("false")
		}
	case string:
		enc, err := json.Marshal(val)
		if err != nil {
			return err
		}
		buf.Write(enc)
	case json.Number:
		buf.WriteString(val.String())
	case float64:
		enc, err := json.Marshal(val)
		if err != nil {
			return err
		}
		buf.Write(enc)
	case []any:
		buf.WriteByte('[')
		for i, elem := range val {
			if i > 0 {
				buf.WriteByte(',')
			}
			if err := writeCanonical(buf, elem); err != nil {
				return err
			}
		}
		buf.WriteByte(']')
	case map[string]any:
		buf.WriteByte('{')
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		// Sort keys lexicographically by Unicode code points (RFC 8785)
		sort.Strings(keys)

		for i, k := range keys {
			if i > 0 {
				buf.WriteByte(',')
			}
			keyBytes, err := json.Marshal(k)
			if err != nil {
				return err
			}
			buf.Write(keyBytes)
			buf.WriteByte(':')
			if err := writeCanonical(buf, val[k]); err != nil {
				return err
			}
		}
		buf.WriteByte('}')
	default:
		return fmt.Errorf("unsupported type in canonical JSON %T", v)
	}
	return nil
}
