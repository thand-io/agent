package verify

import (
	"crypto/ed25519"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"strings"
)

func parseTrustedKeys(keys map[string]string) (map[string]ed25519.PublicKey, error) {
	out := make(map[string]ed25519.PublicKey, len(keys))
	for keyID, keyText := range keys {
		pk, err := parsePublicKeyText(keyText)
		if err != nil {
			return nil, fmt.Errorf("parse key %s: %w", keyID, err)
		}
		out[keyID] = pk
	}
	return out, nil
}

func parsePublicKeyText(keyText string) (ed25519.PublicKey, error) {
	trimmed := strings.TrimSpace(keyText)
	if trimmed == "" {
		return nil, fmt.Errorf("%w: empty key text", ErrInvalidPublicKey)
	}

	if strings.HasPrefix(trimmed, "-----BEGIN ") {
		return parsePEMEd25519PublicKey(trimmed)
	}

	raw, err := base64.StdEncoding.DecodeString(trimmed)
	if err != nil {
		return nil, fmt.Errorf("%w: key is neither valid base64 nor PEM", ErrInvalidPublicKey)
	}
	if len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("%w: raw key must be %d bytes", ErrInvalidPublicKey, ed25519.PublicKeySize)
	}
	pk := make(ed25519.PublicKey, ed25519.PublicKeySize)
	copy(pk, raw)
	return pk, nil
}

func parsePEMEd25519PublicKey(text string) (ed25519.PublicKey, error) {
	block, _ := pem.Decode([]byte(text))
	if block == nil {
		return nil, fmt.Errorf("%w: invalid PEM block", ErrInvalidPublicKey)
	}

	pubAny, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, fmt.Errorf("%w: parse PKIX public key: %v", ErrInvalidPublicKey, err)
	}

	pub, ok := pubAny.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("%w: expected Ed25519", ErrUnsupportedKeyType)
	}
	pk := make(ed25519.PublicKey, len(pub))
	copy(pk, pub)
	return pk, nil
}
