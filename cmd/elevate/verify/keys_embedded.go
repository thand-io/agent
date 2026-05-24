package verify

import (
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"strings"
)

// Embedded key files are compile-time pinned key material.
// Values are PEM-encoded PKIX Ed25519 public keys.
//
//go:embed keys
var trustedKeyFiles embed.FS
var trustedKeysFS fs.FS = trustedKeyFiles

func loadEmbeddedTrustedKeys() (map[string]string, error) {
	entries, err := fs.ReadDir(trustedKeysFS, "keys")
	if err != nil {
		return nil, fmt.Errorf("read embedded keys directory: %w", err)
	}

	out := make(map[string]string, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if filepath.Ext(name) != ".pem" {
			continue
		}

		keyID := strings.TrimSuffix(name, ".pem")
		if keyID == "" {
			return nil, errors.New("embedded trusted key file has empty key id")
		}
		if _, exists := out[keyID]; exists {
			return nil, fmt.Errorf("duplicate embedded key id: %s", keyID)
		}

		raw, err := fs.ReadFile(trustedKeysFS, filepath.ToSlash(filepath.Join("keys", name)))
		if err != nil {
			return nil, fmt.Errorf("read embedded trusted key file %s: %w", name, err)
		}

		keyText := strings.TrimSpace(string(raw))
		if keyText == "" {
			return nil, errors.New("embedded trusted key file is empty for key id: " + keyID)
		}
		out[keyID] = keyText
	}

	return out, nil
}
