package verify

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"testing"
)

func TestParsePublicKeyText_Base64Valid(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	text := base64.StdEncoding.EncodeToString(pub)
	pk, err := parsePublicKeyText(text)
	if err != nil {
		t.Fatalf("parsePublicKeyText failed: %v", err)
	}
	if len(pk) != ed25519.PublicKeySize {
		t.Fatalf("unexpected key length: got %d want %d", len(pk), ed25519.PublicKeySize)
	}
}

func TestParsePublicKeyText_PEMValidEd25519(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey failed: %v", err)
	}
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))

	pk, err := parsePublicKeyText(pemText)
	if err != nil {
		t.Fatalf("parsePublicKeyText failed: %v", err)
	}
	if len(pk) != ed25519.PublicKeySize {
		t.Fatalf("unexpected key length: got %d want %d", len(pk), ed25519.PublicKeySize)
	}
}

func TestParsePublicKeyText_Empty(t *testing.T) {
	_, err := parsePublicKeyText("   ")
	if !errors.Is(err, ErrInvalidPublicKey) {
		t.Fatalf("expected ErrInvalidPublicKey, got: %v", err)
	}
}

func TestParsePublicKeyText_InvalidText(t *testing.T) {
	_, err := parsePublicKeyText("not-a-key")
	if !errors.Is(err, ErrInvalidPublicKey) {
		t.Fatalf("expected ErrInvalidPublicKey, got: %v", err)
	}
}

func TestParsePublicKeyText_PEMWrongType(t *testing.T) {
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Generate RSA key failed: %v", err)
	}

	der, err := x509.MarshalPKIXPublicKey(&rsaKey.PublicKey)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey failed: %v", err)
	}
	pemText := string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}))

	_, err = parsePublicKeyText(pemText)
	if !errors.Is(err, ErrUnsupportedKeyType) {
		t.Fatalf("expected ErrUnsupportedKeyType, got: %v", err)
	}
}

func TestParseTrustedKeys_Map(t *testing.T) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	keys := map[string]string{"k1": base64.StdEncoding.EncodeToString(pub)}
	parsed, err := parseTrustedKeys(keys)
	if err != nil {
		t.Fatalf("parseTrustedKeys failed: %v", err)
	}
	if _, ok := parsed["k1"]; !ok {
		t.Fatal("expected parsed key for k1")
	}
}
