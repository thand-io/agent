package verify

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"errors"
	"strings"
	"testing"

	"github.com/thand-io/agent/cmd/elevate/domain"
)

const (
	testPubCurrentB64 = "A6EHv/POEL4dcN0Y50vAmWfk1jCbpQ1fHdyGZBJVMbg="
	testPubNextB64    = "C7w0aldmfDgBIL2cf9flHSxf3+o3zS9b9AWyxr9vLXg="
	testPayloadJSON   = `{"nonce":"n","action":"grant","workflow_id":"wf","request_id":"r","username":"u","duration_seconds":60}`
	testSigCurrentB64 = "5MXERshUkd6/ih7iPJEtHLvJ1XoP/O6q1Z3HBDhILaxWI4MyxYu7lhxgIG6I9z1BoDzp0/zMqUwF/XwM4KYFAw=="
	testSigNextB64    = "+HeSNOrbUMwyisptpqdqZiXUPo7Ot0t7+A/syNv9ItCuaHcRvX41bJO3v9jQPBLCoy7GT8K/uLOr12di37RQAA=="
)

func TestGenerateNonce(t *testing.T) {
	nonce, err := GenerateNonce()
	if err != nil {
		t.Fatalf("GenerateNonce failed: %v", err)
	}
	raw, err := base64.StdEncoding.DecodeString(nonce)
	if err != nil {
		t.Fatalf("nonce is not valid base64: %v", err)
	}
	if len(raw) != NonceSizeBytes {
		t.Fatalf("unexpected nonce size: got %d want %d", len(raw), NonceSizeBytes)
	}
}

func TestCanonicalPayloadIsDeterministic(t *testing.T) {
	p := SignedPayload{Nonce: "n", Action: "grant", WorkflowID: "wf", RequestID: "r", Username: "u", DurationSeconds: 60}
	a, err := CanonicalPayload(p)
	if err != nil {
		t.Fatalf("CanonicalPayload failed: %v", err)
	}
	b, err := CanonicalPayload(p)
	if err != nil {
		t.Fatalf("CanonicalPayload failed: %v", err)
	}
	if string(a) != string(b) {
		t.Fatalf("canonical payload mismatch: %q vs %q", string(a), string(b))
	}
}

func TestDecodeSignedPayload(t *testing.T) {
	payload := SignedPayload{Nonce: "x", Action: "grant", WorkflowID: "wf", RequestID: "req", Username: "user", DurationSeconds: 10}
	raw, err := CanonicalPayload(payload)
	if err != nil {
		t.Fatalf("CanonicalPayload failed: %v", err)
	}
	enc := base64.StdEncoding.EncodeToString(raw)

	decoded, err := DecodeSignedPayload(enc)
	if err != nil {
		t.Fatalf("DecodeSignedPayload failed: %v", err)
	}
	if decoded != payload {
		t.Fatalf("decoded payload mismatch: got %+v want %+v", decoded, payload)
	}
}

func TestVerifierVerify(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	v, err := NewVerifier(WithTrustedKeys(map[string]ed25519.PublicKey{"kid": pub}))
	if err != nil {
		t.Fatalf("NewVerifier failed: %v", err)
	}

	payload := []byte("payload")
	sig := ed25519.Sign(priv, payload)

	if err := v.Verify("kid", payload, sig); err != nil {
		t.Fatalf("Verify failed: %v", err)
	}

	if err := v.Verify("missing", payload, sig); !errors.Is(err, ErrUnknownKeyID) {
		t.Fatalf("expected ErrUnknownKeyID, got: %v", err)
	}

	bad := append([]byte(nil), sig...)
	bad[0] ^= 0xff
	if err := v.Verify("kid", payload, bad); !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got: %v", err)
	}
}

func TestMatchSignedPayload(t *testing.T) {
	req := domain.RequestFrame{Action: "grant", WorkflowID: "wf", RequestID: "r", Username: "u", DurationSeconds: 123}
	payload := SignedPayload{Nonce: "n", Action: "grant", WorkflowID: "wf", RequestID: "r", Username: "u", DurationSeconds: 123}

	if err := MatchSignedPayload(req, payload, "n"); err != nil {
		t.Fatalf("MatchSignedPayload failed: %v", err)
	}

	payload.Username = "other"
	err := MatchSignedPayload(req, payload, "n")
	if !errors.Is(err, ErrPayloadMismatch) {
		t.Fatalf("expected ErrPayloadMismatch, got: %v", err)
	}
	if !strings.Contains(err.Error(), "username") {
		t.Fatalf("expected mismatch detail, got: %v", err)
	}
}

func TestWithTrustedKeysBase64RejectsInvalidKey(t *testing.T) {
	_, err := NewVerifier(WithTrustedKeysBase64(map[string]string{"k": "not-base64"}))
	if err == nil {
		t.Fatal("expected error for invalid base64 key")
	}
}

func TestNewVerifierLoadsEmbeddedKeys(t *testing.T) {
	v, err := NewVerifier()
	if err != nil {
		t.Fatalf("NewVerifier failed with embedded keys: %v", err)
	}

	// Sanity check: verifier exists and unknown key path returns the expected sentinel.
	if err := v.Verify("missing", []byte("p"), []byte("s")); !errors.Is(err, ErrUnknownKeyID) {
		t.Fatalf("expected ErrUnknownKeyID, got: %v", err)
	}
}

func TestWithTrustedKeysSupportsPEM(t *testing.T) {
	pub, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("GenerateKey failed: %v", err)
	}

	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		t.Fatalf("MarshalPKIXPublicKey failed: %v", err)
	}

	pemText := string(pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: der,
	}))

	v, err := NewVerifier(WithTrustedKeysBase64(map[string]string{"pem-key": pemText}))
	if err != nil {
		t.Fatalf("NewVerifier failed with PEM key: %v", err)
	}

	payload := []byte("payload")
	sig := ed25519.Sign(priv, payload)
	if err := v.Verify("pem-key", payload, sig); err != nil {
		t.Fatalf("Verify failed with PEM key: %v", err)
	}
}

func TestVerifySignatureVectors(t *testing.T) {
	trusted := map[string]string{
		"thand-server-current": testPubCurrentB64,
		"thand-server-next":    testPubNextB64,
	}

	v, err := NewVerifier(WithTrustedKeysBase64(trusted))
	if err != nil {
		t.Fatalf("NewVerifier failed: %v", err)
	}

	payload := []byte(testPayloadJSON)
	sigCurrent, err := base64.StdEncoding.DecodeString(testSigCurrentB64)
	if err != nil {
		t.Fatalf("decode current signature failed: %v", err)
	}
	sigNext, err := base64.StdEncoding.DecodeString(testSigNextB64)
	if err != nil {
		t.Fatalf("decode next signature failed: %v", err)
	}

	tamperedPayload := append([]byte(nil), payload...)
	tamperedPayload[0] ^= 0x01

	tamperedSig := append([]byte(nil), sigCurrent...)
	tamperedSig[0] ^= 0x01

	tests := []struct {
		name    string
		keyID   string
		payload []byte
		sig     []byte
		errIs   error
	}{
		{
			name:    "valid current key signature",
			keyID:   "thand-server-current",
			payload: payload,
			sig:     sigCurrent,
		},
		{
			name:    "valid next key signature",
			keyID:   "thand-server-next",
			payload: payload,
			sig:     sigNext,
		},
		{
			name:    "unknown key id",
			keyID:   "missing",
			payload: payload,
			sig:     sigCurrent,
			errIs:   ErrUnknownKeyID,
		},
		{
			name:    "tampered payload",
			keyID:   "thand-server-current",
			payload: tamperedPayload,
			sig:     sigCurrent,
			errIs:   ErrInvalidSignature,
		},
		{
			name:    "tampered signature",
			keyID:   "thand-server-current",
			payload: payload,
			sig:     tamperedSig,
			errIs:   ErrInvalidSignature,
		},
		{
			name:    "mismatched key and signature",
			keyID:   "thand-server-next",
			payload: payload,
			sig:     sigCurrent,
			errIs:   ErrInvalidSignature,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Verify(tt.keyID, tt.payload, tt.sig)
			if tt.errIs == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.errIs) {
				t.Fatalf("expected errors.Is(err, %v)=true, got err=%v", tt.errIs, err)
			}
		})
	}
}
