package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const DefaultSecretsDir = "/run/vopk/secrets"

// SetupSecrets writes the provided secrets map to disk under secretsDir with mode 0400
func SetupSecrets(secretsDir string, secrets map[string]string) error {
	if len(secrets) == 0 {
		return nil
	}

	if err := os.MkdirAll(secretsDir, 0700); err != nil {
		return fmt.Errorf("failed to create secrets directory %s: %w", secretsDir, err)
	}

	for name, content := range secrets {
		targetPath := filepath.Join(secretsDir, name)

		// Clean and verify that path doesn't escape the secrets directory
		rel, err := filepath.Rel(secretsDir, targetPath)
		if err != nil || rel == ".." || filepath.IsAbs(rel) || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("invalid secret name %q: path traversal attempt", name)
		}

		// Write with 0400 read-only for owner
		if err := os.WriteFile(targetPath, []byte(content), 0400); err != nil {
			return fmt.Errorf("failed to write secret %s: %w", name, err)
		}
	}

	return nil
}
