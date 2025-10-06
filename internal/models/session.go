package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/thand-io/agent/internal/common"
)

// Local User session structure
type LocalSessionConfig struct {

	// The session key is the provider id and the active session JWT
	Sessions map[string]string `json:"sessions"` // Map of session UUIDs to Session objects

}

// Session as part of the auth handlers
type Session struct {
	UUID         uuid.UUID `json:"uuid"`
	User         *User     `json:"user"`
	AccessToken  string    `json:"token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
}

// Encode the remote session from the local session
func (s *Session) GetEncodedSession(encryptor EncryptionImpl) string {
	return EncodingWrapper{
		Type: ENCODED_SESSION,
		Data: s,
	}.EncodeAndEncrypt(encryptor)
}

// Decode the remote session from the local session
func (s *LocalSession) GetDecodedSession(decryptor EncryptionImpl) (*Session, error) {
	decoded, err := EncodingWrapper{}.DecodeAndDecrypt(s.Session, decryptor)

	if err != nil {
		return nil, err
	}

	if decoded.Type != ENCODED_SESSION {
		return nil, fmt.Errorf("invalid session type: %s", decoded.Type)
	}

	var session *Session
	common.ConvertMapToInterface(decoded.Data.(map[string]any), &session)

	return session, nil
}

type SessionCreateRequest struct {
	Provider string `json:"provider" binding:"required"` // Provider ID
	Session  string `json:"session" binding:"required"`  // Encoded session token
}

// Session stored on the users local system
type LocalSession struct {
	Version int       `json:"version" yaml:"version"`      // Version of the session config
	Expiry  time.Time `json:"expiry" yaml:"expiry"`        // Expiry time of the session
	Session string    `json:"session" yaml:"session,flow"` // Encoded session token
}

func (s *LocalSession) IsExpired() bool {
	return time.Now().After(s.Expiry)
}

func (s *LocalSession) GetEncodedLocalSession() string {
	return EncodingWrapper{
		Type: ENCODED_SESSION_LOCAL,
		Data: s,
	}.Encode()
}

func DecodedLocalSession(input string) (*LocalSession, error) {
	wrapper, err := EncodingWrapper{}.Decode(input)
	if err != nil {
		return nil, err
	}

	if wrapper.Type != ENCODED_SESSION_LOCAL {
		return nil, fmt.Errorf("invalid session type: %s", wrapper.Type)
	}

	var session *LocalSession
	common.ConvertMapToInterface(wrapper.Data.(map[string]any), &session)
	return session, nil
}
