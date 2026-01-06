package models

import (
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/internal/common"
	"gopkg.in/yaml.v3"
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
	Token        string    `json:"token"`
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token"`
	Expiry       time.Time `json:"expiry"`
}

func (s *Session) IsExpired() bool {
	return time.Now().After(s.Expiry)
}

type ExportableSession struct {
	*Session
	Provider string          `json:"provider"`
	Endpoint *model.Endpoint `json:"endpoint,omitempty"`
}

// Encode the remote session from the local session
func (s *ExportableSession) GetEncodedSession(encryptor EncryptionImpl) string {
	return EncodingWrapper{
		Type: ENCODED_SESSION,
		Data: s,
	}.EncodeAndEncrypt(encryptor)
}

func (s *ExportableSession) ToLocalSession(encryptor EncryptionImpl) *LocalSession {
	return &LocalSession{
		Version:  1,
		Expiry:   s.Expiry,
		Session:  s.GetEncodedSession(encryptor),
		Endpoint: s.Endpoint,
	}
}

// Decode the remote session from the local session
func (s *LocalSession) GetDecodedSession(decryptor EncryptionImpl) (*ExportableSession, error) {
	decoded, err := EncodingWrapper{}.DecodeAndDecrypt(s.Session, decryptor)

	if err != nil {
		return nil, err
	}

	if decoded.Type != ENCODED_SESSION {
		return nil, fmt.Errorf("invalid session type: %s", decoded.Type)
	}

	var session *ExportableSession
	common.ConvertMapToInterface(decoded.Data.(map[string]any), &session)

	return session, nil
}

type SessionCreateRequest struct {
	Code     string `json:"code" binding:"required"`     // Verification code
	Provider string `json:"provider" binding:"required"` // Provider ID
	Session  string `json:"session" binding:"required"`  // Encoded session token
}

type SessionSetDefaultRequest struct {
	Provider string `json:"provider" binding:"required"` // Provider ID to set as default
}

type SessionsResponse struct {
	Version         string                  `json:"version"`         // Version of the response format
	Timestamp       time.Time               `json:"timestamp"`       // Timestamp of the response
	Sessions        map[string]LocalSession `json:"sessions"`        // Map of provider name to session
	DefaultProvider string                  `json:"defaultProvider"` // Current default provider from cookie
}

// Session stored on the users local system
type LocalSession struct {
	Version  int             `json:"version,omitempty" yaml:"version"`      // Version of the session config
	Expiry   time.Time       `json:"expiry" yaml:"expiry"`                  // Expiry time of the session
	Session  string          `json:"session,omitempty" yaml:"session,flow"` // Encoded session token
	Endpoint *model.Endpoint `json:"endpoint,omitempty" yaml:"endpoint"`    // Optional endpoint associated with the session
}

func (s *LocalSession) IsExpired() bool {
	return time.Now().After(s.Expiry)
}

// CopyWithoutEndpoint creates a shallow copy of the LocalSession without the Endpoint field
func (s *LocalSession) CopyWithoutEndpoint() *LocalSession {
	copied := &LocalSession{
		Version: s.Version,
		Expiry:  s.Expiry,
		Session: s.Session,
	}
	return copied
}

func (s *LocalSession) GetEncodedLocalSession() string {
	return EncodingWrapper{
		Type: ENCODED_SESSION_LOCAL,
		Data: s,
	}.EncodeBase64()
}

func (s *LocalSession) GetEncodedLocalSessionBytes() []byte {
	return EncodingWrapper{
		Type: ENCODED_SESSION_LOCAL,
		Data: s,
	}.EncodeBytes()
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

func DecodedLocalSessionBytes(input []byte) (*LocalSession, error) {
	wrapper, err := EncodingWrapper{}.DecodeBytes(input)
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

// UnmarshalYAML implements custom YAML unmarshaling for LocalSession to handle model.Endpoint
func (s *LocalSession) UnmarshalYAML(node *yaml.Node) error {
	// Marshal the YAML node to bytes, then use ReadDataToInterface which handles the conversion
	data, err := yaml.Marshal(node)
	if err != nil {
		return fmt.Errorf("failed to marshal yaml node: %w", err)
	}

	// Use ReadDataToInterface which converts YAML->JSON and uses model.Endpoint's UnmarshalJSON
	result, err := common.ReadDataToInterface(data, LocalSession{})
	if err != nil {
		return fmt.Errorf("failed to unmarshal LocalSession: %w", err)
	}

	*s = *result
	return nil
}

// MarshalYAML implements custom YAML marshaling for LocalSession to produce clean output
func (s *LocalSession) MarshalYAML() (any, error) {
	// Create a map for clean YAML output
	result := make(map[string]any)

	if s.Version != 0 {
		result["version"] = s.Version
	}

	if !s.Expiry.IsZero() {
		result["expiry"] = s.Expiry
	}

	if len(s.Session) != 0 {
		result["session"] = s.Session
	}

	// Handle endpoint by converting through JSON (which has proper marshaling)
	if s.Endpoint != nil {
		// Use the SDK's JSON marshaling which properly handles nested structures
		endpointMap, err := common.ConvertInterfaceToMap(s.Endpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal endpoint: %w", err)
		}

		// Only add if not empty
		if len(endpointMap) > 0 {
			result["endpoint"] = endpointMap
		}
	}

	return result, nil
}
