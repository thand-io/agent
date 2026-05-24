package verify

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
)

func TestLoadEmbeddedTrustedKeys_EmptyWhenNoPEM(t *testing.T) {
	setTrustedKeysFS(t, fstest.MapFS{
		"keys/my-key-example.pem.example": &fstest.MapFile{
			Data: []byte("example only"),
		},
	})

	keys, err := loadEmbeddedTrustedKeys()
	if err != nil {
		t.Fatalf("loadEmbeddedTrustedKeys failed: %v", err)
	}
	if len(keys) != 0 {
		t.Fatalf("expected no trusted keys, got %d", len(keys))
	}
}

func TestLoadEmbeddedTrustedKeys_LoadsPEM(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey failed: %v", err)
	}
	pemText := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})

	setTrustedKeysFS(t, fstest.MapFS{
		"keys/local-test-key.pem": &fstest.MapFile{
			Data: pemText,
		},
		"keys/ignored.pem.example": &fstest.MapFile{
			Data: []byte("ignored"),
		},
	})

	keys, err := loadEmbeddedTrustedKeys()
	if err != nil {
		t.Fatalf("loadEmbeddedTrustedKeys failed: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 trusted key, got %d", len(keys))
	}
	for keyID, keyText := range keys {
		if strings.TrimSpace(keyText) == "" {
			t.Fatalf("expected non-empty key text for %s", keyID)
		}
		if !strings.Contains(keyText, "BEGIN PUBLIC KEY") {
			t.Fatalf("expected PEM public key format for %s", keyID)
		}
		key, err := parsePublicKeyText(keyText)
		if err != nil {
			t.Fatalf("expected parseable key for %s: %v", keyID, err)
		}
		if len(key) != ed25519.PublicKeySize {
			t.Fatalf("unexpected public key size for %s: got %d", keyID, len(key))
		}
	}
}

func TestNewVerifier_RequiresTrustedKeys(t *testing.T) {
	setTrustedKeysFS(t, fstest.MapFS{
		"keys/my-key-example.pem.example": &fstest.MapFile{
			Data: []byte("example only"),
		},
	})

	_, err := NewVerifier()
	if err == nil {
		t.Fatal("expected NewVerifier to fail without trusted keys")
	}
	if !errors.Is(err, ErrInvalidPublicKey) {
		t.Fatalf("expected ErrInvalidPublicKey, got: %v", err)
	}
	if !strings.Contains(err.Error(), "no trusted keys configured") {
		t.Fatalf("expected no trusted keys error, got: %v", err)
	}
}

func setTrustedKeysFS(t *testing.T, keyFS fs.FS) {
	t.Helper()
	original := trustedKeysFS
	trustedKeysFS = keyFS
	t.Cleanup(func() {
		trustedKeysFS = original
	})
}
