package verify

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/thand-io/agent/cmd/elevate/domain"
)

const NonceSizeBytes = 32

var (
	ErrUnknownKeyID       = errors.New("unknown key id")
	ErrInvalidPublicKey   = errors.New("invalid public key")
	ErrInvalidSignature   = errors.New("invalid signature")
	ErrInvalidSignedData  = errors.New("invalid signed payload")
	ErrPayloadMismatch    = errors.New("signed payload mismatch")
	ErrUnsupportedKeyType = errors.New("unsupported public key type")
)

// SignedPayload is the canonical signed data schema used by the verifier.
type SignedPayload struct {
	Nonce           string `json:"nonce"`
	Action          string `json:"action"`
	WorkflowID      string `json:"workflow_id"`
	RequestID       string `json:"request_id"`
	Username        string `json:"username"`
	DurationSeconds int64  `json:"duration_seconds,omitempty"`
}

type Verifier struct {
	trustedKeys map[string]ed25519.PublicKey
}

type Option func(*Verifier) error

func WithTrustedKeys(keys map[string]ed25519.PublicKey) Option {
	return func(v *Verifier) error {
		cloned, err := cloneKeyMap(keys)
		if err != nil {
			return err
		}
		v.trustedKeys = cloned
		return nil
	}
}

func WithTrustedKeysBase64(keys map[string]string) Option {
	return func(v *Verifier) error {
		parsed, err := parseTrustedKeys(keys)
		if err != nil {
			return err
		}
		v.trustedKeys = parsed
		return nil
	}
}

func NewVerifier(opts ...Option) (*Verifier, error) {
	embeddedKeys, err := loadEmbeddedTrustedKeys()
	if err != nil {
		return nil, err
	}

	parsed, err := parseTrustedKeys(embeddedKeys)
	if err != nil {
		return nil, err
	}

	v := &Verifier{trustedKeys: parsed}
	for _, opt := range opts {
		if err := opt(v); err != nil {
			return nil, err
		}
	}

	if len(v.trustedKeys) == 0 {
		return nil, fmt.Errorf("%w: no trusted keys configured", ErrInvalidPublicKey)
	}

	return v, nil
}

func (v *Verifier) Verify(keyID string, payload []byte, signature []byte) error {
	key, ok := v.trustedKeys[keyID]
	if !ok {
		return fmt.Errorf("%w: %s", ErrUnknownKeyID, keyID)
	}

	if !ed25519.Verify(key, payload, signature) {
		return ErrInvalidSignature
	}

	return nil
}

func GenerateNonce() (string, error) {
	buf := make([]byte, NonceSizeBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}
	return base64.StdEncoding.EncodeToString(buf), nil
}

func CanonicalPayload(payload SignedPayload) ([]byte, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal signed payload: %w", err)
	}
	return b, nil
}

func DecodeSignedPayload(encoded string) (SignedPayload, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return SignedPayload{}, fmt.Errorf("decode payload: %w", err)
	}

	var payload SignedPayload
	if err := json.Unmarshal(raw, &payload); err != nil {
		return SignedPayload{}, fmt.Errorf("%w: %v", ErrInvalidSignedData, err)
	}

	return payload, nil
}

func DecodeSignature(encoded string) ([]byte, error) {
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode signature: %w", err)
	}
	return raw, nil
}

func MatchSignedPayload(req domain.RequestFrame, payload SignedPayload, nonce string) error {
	if payload.Nonce != nonce {
		return fmt.Errorf("%w: nonce mismatch", ErrPayloadMismatch)
	}
	if payload.Action != string(req.Action) {
		return fmt.Errorf("%w: action mismatch", ErrPayloadMismatch)
	}
	if payload.WorkflowID != req.WorkflowID {
		return fmt.Errorf("%w: workflow_id mismatch", ErrPayloadMismatch)
	}
	if payload.RequestID != req.RequestID {
		return fmt.Errorf("%w: request_id mismatch", ErrPayloadMismatch)
	}
	if payload.Username != req.Username {
		return fmt.Errorf("%w: username mismatch", ErrPayloadMismatch)
	}
	if payload.DurationSeconds != req.DurationSeconds {
		return fmt.Errorf("%w: duration_seconds mismatch", ErrPayloadMismatch)
	}
	return nil
}

func cloneKeyMap(in map[string]ed25519.PublicKey) (map[string]ed25519.PublicKey, error) {
	out := make(map[string]ed25519.PublicKey, len(in))
	for k, v := range in {
		if k == "" {
			return nil, fmt.Errorf("%w: empty key id", ErrInvalidPublicKey)
		}
		if len(v) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("%w: key %s must be %d bytes", ErrInvalidPublicKey, k, ed25519.PublicKeySize)
		}
		pk := make(ed25519.PublicKey, len(v))
		copy(pk, v)
		out[k] = pk
	}
	return out, nil
}
