package verify

import "errors"

type PlaceholderVerifier struct{}

func NewPlaceholderVerifier() *PlaceholderVerifier {
	return &PlaceholderVerifier{}
}

func (v *PlaceholderVerifier) Verify(keyID string, payload []byte, signature []byte) error {
	_ = keyID
	_ = payload
	_ = signature
	return errors.New("signature verifier not implemented")
}
