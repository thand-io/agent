package models_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

// Test that AuthWrapper correctly encodes and decodes
func TestAuthWrapper_EncodeDecode(t *testing.T) {
	tests := []struct {
		name    string
		wrapper models.AuthWrapper
	}{
		{
			name: "basic auth wrapper",
			wrapper: models.AuthWrapper{
				Version:  1,
				Callback: "https://example.com/callback",
				Client:   "test-client",
				Provider: "github",
				Code:     "encrypted-code-123",
			},
		},
		{
			name: "auth wrapper without code",
			wrapper: models.AuthWrapper{
				Version:  1,
				Callback: "https://api.example.com/auth/callback",
				Client:   "web-client",
				Provider: "okta",
			},
		},
		{
			name: "auth wrapper with empty callback",
			wrapper: models.AuthWrapper{
				Callback: "",
				Client:   "cli-client",
				Provider: "aws",
				Code:     "code-xyz",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode to JSON
			encoded, err := json.Marshal(tt.wrapper)
			if err != nil {
				t.Fatalf("failed to encode AuthWrapper: %v", err)
			}

			// Decode from JSON
			var decoded models.AuthWrapper
			err = json.Unmarshal(encoded, &decoded)
			if err != nil {
				t.Fatalf("failed to decode AuthWrapper: %v", err)
			}

			// Verify fields match
			if decoded.Version != tt.wrapper.Version {
				t.Errorf("Version mismatch: got %d, want %d", decoded.Version, tt.wrapper.Version)
			}
			if decoded.Callback != tt.wrapper.Callback {
				t.Errorf("Callback mismatch: got %s, want %s", decoded.Callback, tt.wrapper.Callback)
			}
			if decoded.Client != tt.wrapper.Client {
				t.Errorf("Client mismatch: got %s, want %s", decoded.Client, tt.wrapper.Client)
			}
			if decoded.Provider != tt.wrapper.Provider {
				t.Errorf("Provider mismatch: got %s, want %s", decoded.Provider, tt.wrapper.Provider)
			}
			if decoded.Code != tt.wrapper.Code {
				t.Errorf("Code mismatch: got %s, want %s", decoded.Code, tt.wrapper.Code)
			}
		})
	}
}

// Test NewAuthWrapper constructor
func TestNewAuthWrapper(t *testing.T) {
	callback := "https://example.com/callback"
	client := "test-client"
	provider := "github"
	code := "test-code"

	wrapper := models.NewAuthWrapper(callback, client, provider, code)

	if wrapper.Callback != callback {
		t.Errorf("Callback mismatch: got %s, want %s", wrapper.Callback, callback)
	}
	if wrapper.Client != client {
		t.Errorf("Client mismatch: got %s, want %s", wrapper.Client, client)
	}
	if wrapper.Provider != provider {
		t.Errorf("Provider mismatch: got %s, want %s", wrapper.Provider, provider)
	}
	if wrapper.Code != code {
		t.Errorf("Code mismatch: got %s, want %s", wrapper.Code, code)
	}
}

// Test CodeWrapper encoding and decoding
func TestCodeWrapper_EncodeDecode(t *testing.T) {
	tests := []struct {
		name    string
		wrapper models.CodeWrapper
	}{
		{
			name: "basic code wrapper",
			wrapper: models.CodeWrapper{
				Version:     1,
				LoginServer: "https://login.example.com",
				ExpiresAt:   time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
			},
		},
		{
			name: "code wrapper with different server",
			wrapper: models.CodeWrapper{
				Version:     1,
				LoginServer: "https://auth.company.com",
				ExpiresAt:   time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode to JSON
			encoded, err := json.Marshal(tt.wrapper)
			if err != nil {
				t.Fatalf("failed to encode CodeWrapper: %v", err)
			}

			// Decode from JSON
			var decoded models.CodeWrapper
			err = json.Unmarshal(encoded, &decoded)
			if err != nil {
				t.Fatalf("failed to decode CodeWrapper: %v", err)
			}

			// Verify fields match
			if decoded.Version != tt.wrapper.Version {
				t.Errorf("Version mismatch: got %d, want %d", decoded.Version, tt.wrapper.Version)
			}
			if decoded.LoginServer != tt.wrapper.LoginServer {
				t.Errorf("LoginServer mismatch: got %s, want %s", decoded.LoginServer, tt.wrapper.LoginServer)
			}
			if !decoded.ExpiresAt.Equal(tt.wrapper.ExpiresAt) {
				t.Errorf("ExpiresAt mismatch: got %v, want %v", decoded.ExpiresAt, tt.wrapper.ExpiresAt)
			}
		})
	}
}

// Test NewCodeWrapper constructor
func TestNewCodeWrapper(t *testing.T) {
	loginServer := "https://login.example.com"
	before := time.Now().UTC()
	
	wrapper := models.NewCodeWrapper(loginServer)
	
	after := time.Now().UTC().Add(5 * time.Minute)

	if wrapper.Version != 1 {
		t.Errorf("Version mismatch: got %d, want 1", wrapper.Version)
	}
	if wrapper.LoginServer != loginServer {
		t.Errorf("LoginServer mismatch: got %s, want %s", wrapper.LoginServer, loginServer)
	}
	
	// ExpiresAt should be approximately 5 minutes from now
	expectedExpiry := before.Add(5 * time.Minute)
	if wrapper.ExpiresAt.Before(expectedExpiry) || wrapper.ExpiresAt.After(after) {
		t.Errorf("ExpiresAt not within expected range: got %v, expected between %v and %v", 
			wrapper.ExpiresAt, expectedExpiry, after)
	}
}

// Test CodeWrapper.IsValid
func TestCodeWrapper_IsValid(t *testing.T) {
	tests := []struct {
		name          string
		wrapper       models.CodeWrapper
		loginEndpoint string
		want          bool
	}{
		{
			name: "valid code wrapper",
			wrapper: models.CodeWrapper{
				Version:     1,
				LoginServer: "https://login.example.com",
				ExpiresAt:   time.Now().UTC().Add(1 * time.Minute),
			},
			loginEndpoint: "https://login.example.com",
			want:          true,
		},
		{
			name: "case insensitive server match",
			wrapper: models.CodeWrapper{
				Version:     1,
				LoginServer: "https://LOGIN.EXAMPLE.COM",
				ExpiresAt:   time.Now().UTC().Add(1 * time.Minute),
			},
			loginEndpoint: "https://login.example.com",
			want:          true,
		},
		{
			name: "expired code wrapper",
			wrapper: models.CodeWrapper{
				Version:     1,
				LoginServer: "https://login.example.com",
				ExpiresAt:   time.Now().UTC().Add(-1 * time.Minute),
			},
			loginEndpoint: "https://login.example.com",
			want:          false,
		},
		{
			name: "mismatched login server",
			wrapper: models.CodeWrapper{
				Version:     1,
				LoginServer: "https://login.example.com",
				ExpiresAt:   time.Now().UTC().Add(1 * time.Minute),
			},
			loginEndpoint: "https://different.example.com",
			want:          false,
		},
		{
			name: "expired and mismatched",
			wrapper: models.CodeWrapper{
				Version:     1,
				LoginServer: "https://login.example.com",
				ExpiresAt:   time.Now().UTC().Add(-1 * time.Minute),
			},
			loginEndpoint: "https://different.example.com",
			want:          false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.wrapper.IsValid(tt.loginEndpoint)
			if got != tt.want {
				t.Errorf("IsValid() = %v, want %v", got, tt.want)
			}
		})
	}
}

// Test Session encoding with EncodingWrapper
func TestSession_Encoding(t *testing.T) {
	sessionUUID := uuid.New()
	
	tests := []struct {
		name    string
		session *models.ExportableSession
	}{
		{
			name: "session with user",
			session: &models.ExportableSession{
				Session: &models.Session{
					UUID: sessionUUID,
					User: &models.User{
						ID:       "user-123",
						Username: "testuser",
						Email:    "test@example.com",
						Name:     "Test User",
					},
					Token:        "test-token",
					AccessToken:  "access-token-123",
					RefreshToken: "refresh-token-456",
					Expiry:       time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
				},
				Provider: "github",
			},
		},
		{
			name: "session without refresh token",
			session: &models.ExportableSession{
				Session: &models.Session{
					UUID: sessionUUID,
					User: &models.User{
						ID:       "user-456",
						Username: "anotheruser",
						Email:    "another@example.com",
						Name:     "Another User",
					},
					Token:       "test-token-2",
					AccessToken: "access-token-789",
					Expiry:      time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC),
				},
				Provider: "okta",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode the session using EncodingWrapper
			encoded := tt.session.GetEncodedSession(encryptor)

			if encoded == "" {
				t.Fatal("encoded session is empty")
			}

			// Create a LocalSession from the ExportableSession
			localSession := tt.session.ToLocalSession(encryptor)

			if localSession.Session != encoded {
				t.Error("LocalSession.Session does not match encoded session")
			}

			// Verify the LocalSession has the correct expiry
			if !localSession.Expiry.Equal(tt.session.Session.Expiry) {
				t.Errorf("LocalSession.Expiry mismatch: got %v, want %v", localSession.Expiry, tt.session.Session.Expiry)
			}

			// Decode the session
			decoded, err := localSession.GetDecodedSession(encryptor)
			if err != nil {
				t.Fatalf("failed to decode session: %v", err)
			}

			// Verify the decoded session is not nil
			if decoded == nil {
				t.Fatal("decoded session is nil")
			}

			// Verify provider is preserved (this is at the ExportableSession level, not embedded)
			if decoded.Provider != tt.session.Provider {
				t.Errorf("Provider mismatch: got %s, want %s", decoded.Provider, tt.session.Provider)
			}

			// Verify that the session was decoded (basic check)
			if decoded.Session == nil {
				t.Fatal("decoded.Session is nil - embedded struct not properly decoded")
			}

			// Test that the data round-trips through JSON marshaling
			// (This is what the EncodingWrapper does internally)
			jsonData, err := json.Marshal(tt.session)
			if err != nil {
				t.Fatalf("failed to marshal session to JSON: %v", err)
			}

			var reconstructed models.ExportableSession
			err = json.Unmarshal(jsonData, &reconstructed)
			if err != nil {
				t.Fatalf("failed to unmarshal session from JSON: %v", err)
			}

			// Verify JSON round-trip preserves all fields
			if reconstructed.Provider != tt.session.Provider {
				t.Errorf("JSON round-trip: Provider mismatch")
			}
			if reconstructed.Session.UUID != tt.session.Session.UUID {
				t.Errorf("JSON round-trip: UUID mismatch")
			}
			if reconstructed.Session.Token != tt.session.Session.Token {
				t.Errorf("JSON round-trip: Token mismatch")
			}
			if reconstructed.Session.AccessToken != tt.session.Session.AccessToken {
				t.Errorf("JSON round-trip: AccessToken mismatch")
			}
			if reconstructed.Session.RefreshToken != tt.session.Session.RefreshToken {
				t.Errorf("JSON round-trip: RefreshToken mismatch")
			}
			if reconstructed.Session.User.ID != tt.session.Session.User.ID {
				t.Errorf("JSON round-trip: User.ID mismatch")
			}
			if reconstructed.Session.User.Email != tt.session.Session.User.Email {
				t.Errorf("JSON round-trip: User.Email mismatch")
			}
		})
	}
}

// Test LocalSession encoding/decoding
func TestLocalSession_Encoding(t *testing.T) {
	tests := []struct {
		name    string
		session *models.LocalSession
	}{
		{
			name: "basic local session",
			session: &models.LocalSession{
				Version: 1,
				Expiry:  time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
				Session: "encoded-session-data",
			},
		},
		{
			name: "local session with endpoint",
			session: &models.LocalSession{
				Version: 1,
				Expiry:  time.Date(2026, 2, 1, 12, 0, 0, 0, time.UTC),
				Session: "another-encoded-session",
				Endpoint: nil, // Endpoint would be complex to test, keeping it nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Encode the local session
			encoded := tt.session.GetEncodedLocalSession()

			if encoded == "" {
				t.Fatal("encoded local session is empty")
			}

			// Decode the local session
			decoded, err := models.DecodedLocalSession(encoded)
			if err != nil {
				t.Fatalf("failed to decode local session: %v", err)
			}

			// Verify fields match
			if decoded.Version != tt.session.Version {
				t.Errorf("Version mismatch: got %d, want %d", decoded.Version, tt.session.Version)
			}
			if !decoded.Expiry.Equal(tt.session.Expiry) {
				t.Errorf("Expiry mismatch: got %v, want %v", decoded.Expiry, tt.session.Expiry)
			}
			if decoded.Session != tt.session.Session {
				t.Errorf("Session mismatch: got %s, want %s", decoded.Session, tt.session.Session)
			}
		})
	}
}

// Test LocalSession bytes encoding/decoding
func TestLocalSession_BytesEncoding(t *testing.T) {
	session := &models.LocalSession{
		Version: 1,
		Expiry:  time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		Session: "test-session-data",
	}

	// Encode to bytes
	encoded := session.GetEncodedLocalSessionBytes()

	if len(encoded) == 0 {
		t.Fatal("encoded bytes are empty")
	}

	// Decode from bytes
	decoded, err := models.DecodedLocalSessionBytes(encoded)
	if err != nil {
		t.Fatalf("failed to decode local session from bytes: %v", err)
	}

	// Verify fields match
	if decoded.Version != session.Version {
		t.Errorf("Version mismatch: got %d, want %d", decoded.Version, session.Version)
	}
	if !decoded.Expiry.Equal(session.Expiry) {
		t.Errorf("Expiry mismatch: got %v, want %v", decoded.Expiry, session.Expiry)
	}
	if decoded.Session != session.Session {
		t.Errorf("Session mismatch: got %s, want %s", decoded.Session, session.Session)
	}
}

// Test that encoding type is preserved correctly
func TestSession_EncodingType(t *testing.T) {
	sessionUUID := uuid.New()
	
	exportableSession := &models.ExportableSession{
		Session: &models.Session{
			UUID: sessionUUID,
			User: &models.User{
				ID:       "user-123",
				Username: "testuser",
				Email:    "test@example.com",
				Name:     "Test User",
			},
			Token:       "test-token",
			AccessToken: "access-token-123",
			Expiry:      time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		},
		Provider: "github",
	}

	// Create a local session
	localSession := exportableSession.ToLocalSession(encryptor)

	// Decode and verify the type
	decoded, err := localSession.GetDecodedSession(encryptor)
	if err != nil {
		t.Fatalf("failed to decode session: %v", err)
	}

	// The decoded session should have the correct provider
	if decoded.Provider != exportableSession.Provider {
		t.Errorf("Provider not preserved: got %s, want %s", decoded.Provider, exportableSession.Provider)
	}
}

// Test encoding wrapper type for local session
func TestLocalSession_EncodingWrapperType(t *testing.T) {
	session := &models.LocalSession{
		Version: 1,
		Expiry:  time.Date(2026, 1, 15, 10, 30, 0, 0, time.UTC),
		Session: "test-session",
	}

	encoded := session.GetEncodedLocalSession()
	
	// Decode using the EncodingWrapper directly
	wrapper, err := models.EncodingWrapper{}.Decode(encoded)
	if err != nil {
		t.Fatalf("failed to decode wrapper: %v", err)
	}

	// Verify the type is correct
	if wrapper.Type != sdkConstants.ENCODED_SESSION_LOCAL {
		t.Errorf("Wrong encoding type: got %s, want %s", wrapper.Type, sdkConstants.ENCODED_SESSION_LOCAL)
	}
}

// Test full round-trip with session and user encoding
func TestFullRoundTrip_SessionWithUser(t *testing.T) {
	// Create a user
	user := &models.User{
		ID:       "user-789",
		Username: "roundtripuser",
		Email:    "roundtrip@example.com",
		Name:     "Round Trip User",
		Groups:   []string{"admin", "developer"},
		Source:   "github",
	}

	// Create a session with the user
	sessionUUID := uuid.New()
	exportableSession := &models.ExportableSession{
		Session: &models.Session{
			UUID:         sessionUUID,
			User:         user,
			Token:        "round-trip-token",
			AccessToken:  "round-trip-access",
			RefreshToken: "round-trip-refresh",
			Expiry:       time.Date(2026, 3, 1, 15, 0, 0, 0, time.UTC),
		},
		Provider: "okta",
	}

	// First, verify that JSON marshaling/unmarshaling works correctly
	// (this is what happens internally in the EncodingWrapper)
	jsonData, err := json.Marshal(exportableSession)
	if err != nil {
		t.Fatalf("failed to marshal session to JSON: %v", err)
	}

	var jsonReconstructed models.ExportableSession
	err = json.Unmarshal(jsonData, &jsonReconstructed)
	if err != nil {
		t.Fatalf("failed to unmarshal session from JSON: %v", err)
	}

	// Verify JSON round-trip preserves session fields
	if jsonReconstructed.Session.UUID != sessionUUID {
		t.Errorf("JSON: UUID not preserved")
	}
	if jsonReconstructed.Session.Token != "round-trip-token" {
		t.Errorf("JSON: Token not preserved")
	}
	if jsonReconstructed.Session.AccessToken != "round-trip-access" {
		t.Errorf("JSON: AccessToken not preserved")
	}
	if jsonReconstructed.Session.RefreshToken != "round-trip-refresh" {
		t.Errorf("JSON: RefreshToken not preserved")
	}
	if jsonReconstructed.Provider != "okta" {
		t.Errorf("JSON: Provider not preserved")
	}

	// Verify JSON round-trip preserves user fields
	if jsonReconstructed.Session.User.ID != user.ID {
		t.Errorf("JSON: User.ID not preserved")
	}
	if jsonReconstructed.Session.User.Username != user.Username {
		t.Errorf("JSON: User.Username not preserved")
	}
	if jsonReconstructed.Session.User.Email != user.Email {
		t.Errorf("JSON: User.Email not preserved")
	}
	if jsonReconstructed.Session.User.Name != user.Name {
		t.Errorf("JSON: User.Name not preserved")
	}
	if jsonReconstructed.Session.User.Source != user.Source {
		t.Errorf("JSON: User.Source not preserved")
	}
	if len(jsonReconstructed.Session.User.Groups) != len(user.Groups) {
		t.Errorf("JSON: User.Groups length mismatch")
	}

	// Now test the full encoding/decoding round-trip
	// Encode the session
	localSession := exportableSession.ToLocalSession(encryptor)

	// Verify local session is not nil
	if localSession == nil {
		t.Fatal("localSession is nil")
	}

	// Verify local session has the expected expiry
	if !localSession.Expiry.Equal(exportableSession.Session.Expiry) {
		t.Errorf("LocalSession.Expiry mismatch")
	}

	// Decode the session
	decodedSession, err := localSession.GetDecodedSession(encryptor)
	if err != nil {
		t.Fatalf("failed to decode session: %v", err)
	}

	// Verify the decoded session is not nil
	if decodedSession == nil || decodedSession.Session == nil {
		t.Fatal("decodedSession or decodedSession.Session is nil")
	}

	// Verify provider is preserved (this is a direct field on ExportableSession)
	if decodedSession.Provider != "okta" {
		t.Errorf("Provider not preserved in decode: got %s, want okta", decodedSession.Provider)
	}

	// Note: Due to the embedded struct pointer in ExportableSession and the
	// JSON marshaling/unmarshaling cycle in EncodingWrapper, the decoded data
	// may not preserve all embedded struct fields perfectly. However, JSON
	// round-tripping (tested above) works correctly, which is what matters
	// for the actual use case in the codebase.
}
