package models

import (
	"strings"
	"testing"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"gopkg.in/yaml.v3"
)

func TestLocalSession_MarshalYAML(t *testing.T) {
	tests := []struct {
		name        string
		session     LocalSession
		wantContain []string
		wantOmit    []string
	}{
		{
			name: "basic session with all fields",
			session: LocalSession{
				Version: 1,
				Expiry:  time.Date(2026, 1, 4, 10, 33, 8, 0, time.UTC),
				Session: "test-session-token",
				Endpoint: &model.Endpoint{
					EndpointConfig: &model.EndpointConfiguration{
						URI: &model.LiteralUri{Value: "https://example.com"},
						Authentication: &model.ReferenceableAuthenticationPolicy{
							AuthenticationPolicy: &model.AuthenticationPolicy{
								Basic: &model.BasicAuthenticationPolicy{
									Username: "user",
									Password: "password",
								},
							},
						},
					},
				},
			},
			wantContain: []string{"version", "expiry", "session", "endpoint", "uri", "authentication", "basic", "username", "password"},
			wantOmit:    []string{"null", "bearer", "oauth2", "digest", "oidc", "runtimeexpression", "uritemplate"},
		},
		{
			name: "session with bearer authentication",
			session: LocalSession{
				Version: 1,
				Expiry:  time.Date(2026, 1, 4, 10, 33, 8, 0, time.UTC),
				Session: "test-session-token",
				Endpoint: &model.Endpoint{
					EndpointConfig: &model.EndpointConfiguration{
						URI: &model.LiteralUri{Value: "https://example.com"},
						Authentication: &model.ReferenceableAuthenticationPolicy{
							AuthenticationPolicy: &model.AuthenticationPolicy{
								Bearer: &model.BearerAuthenticationPolicy{
									Token: "bearer-token-123",
								},
							},
						},
					},
				},
			},
			wantContain: []string{"version", "expiry", "session", "endpoint", "uri", "authentication", "bearer", "token"},
			wantOmit:    []string{"null", "basic", "oauth2", "digest", "oidc", "runtimeexpression", "uritemplate"},
		},
		{
			name: "session with proxy bearer authentication",
			session: LocalSession{
				Version: 1,
				Expiry:  time.Date(2026, 1, 4, 10, 33, 8, 0, time.UTC),
				Session: "test-session-token",
				Endpoint: &model.Endpoint{
					EndpointConfig: &model.EndpointConfiguration{
						URI: &model.LiteralUri{Value: "https://agent.example.com"},
						Authentication: &model.ReferenceableAuthenticationPolicy{
							AuthenticationPolicy: &model.AuthenticationPolicy{
								ProxyBearer: &model.ProxyBearerAuthenticationPolicy{
									Token: "proxy-bearer-token",
								},
							},
						},
					},
				},
			},
			wantContain: []string{"version", "expiry", "session", "endpoint", "uri", "authentication", "proxy_bearer", "token"},
			wantOmit:    []string{"null", "basic", "oauth2", "digest", "oidc", "runtimeexpression", "uritemplate"},
		},

		{
			name: "session without endpoint",
			session: LocalSession{
				Version: 1,
				Expiry:  time.Date(2026, 1, 4, 10, 33, 8, 0, time.UTC),
				Session: "test-session-token",
			},
			wantContain: []string{"version", "expiry", "session"},
			wantOmit:    []string{"endpoint", "null", "authentication"},
		},
		{
			name: "minimal session",
			session: LocalSession{
				Expiry:  time.Date(2026, 1, 4, 10, 33, 8, 0, time.UTC),
				Session: "test-session-token",
			},
			wantContain: []string{"expiry", "session"},
			wantOmit:    []string{"version", "endpoint", "null"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to YAML
			data, err := yaml.Marshal(&tt.session)
			if err != nil {
				t.Fatalf("MarshalYAML() error = %v", err)
			}

			yamlStr := string(data)
			t.Logf("Generated YAML:\n%s", yamlStr)

			// Check for expected content
			for _, want := range tt.wantContain {
				if !strings.Contains(yamlStr, want) {
					t.Errorf("MarshalYAML() output should contain %q, but doesn't.\nGot: %s", want, yamlStr)
				}
			}

			// Check that unwanted content is omitted
			for _, omit := range tt.wantOmit {
				if strings.Contains(yamlStr, omit) {
					t.Errorf("MarshalYAML() output should NOT contain %q, but does.\nGot: %s", omit, yamlStr)
				}
			}
		})
	}
}

func TestLocalSession_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name      string
		yaml      string
		wantError bool
		validate  func(t *testing.T, session *LocalSession)
	}{
		{
			name: "basic session with authentication",
			yaml: `version: 1
expiry: 2026-01-04T10:33:08Z
session: test-session-token
endpoint:
  uri: https://github.com/serverlessworkflow/catalog
  authentication:
    basic:
      username: user
      password: "012345"
`,
			wantError: false,
			validate: func(t *testing.T, session *LocalSession) {
				if session.Version != 1 {
					t.Errorf("Version = %v, want 1", session.Version)
				}
				if session.Session != "test-session-token" {
					t.Errorf("Session = %v, want test-session-token", session.Session)
				}
				if session.Endpoint == nil {
					t.Fatal("Endpoint should not be nil")
				}
				if session.Endpoint.EndpointConfig == nil {
					t.Fatal("EndpointConfig should not be nil")
				}
				if session.Endpoint.EndpointConfig.URI.String() != "https://github.com/serverlessworkflow/catalog" {
					t.Errorf("URI = %v, want https://github.com/serverlessworkflow/catalog", session.Endpoint.EndpointConfig.URI.String())
				}
				if session.Endpoint.EndpointConfig.Authentication == nil {
					t.Fatal("Authentication should not be nil")
				}
				if session.Endpoint.EndpointConfig.Authentication.AuthenticationPolicy == nil {
					t.Fatal("AuthenticationPolicy should not be nil")
				}
				if session.Endpoint.EndpointConfig.Authentication.AuthenticationPolicy.Basic == nil {
					t.Fatal("Basic auth should not be nil")
				}
				if session.Endpoint.EndpointConfig.Authentication.AuthenticationPolicy.Basic.Username != "user" {
					t.Errorf("Username = %v, want user", session.Endpoint.EndpointConfig.Authentication.AuthenticationPolicy.Basic.Username)
				}
			},
		},
		{
			name: "session with bearer authentication",
			yaml: `version: 1
expiry: 2026-01-04T10:33:08Z
session: test-session-token
endpoint:
  uri: https://api.example.com
  authentication:
    bearer:
      token: bearer-token-123
`,
			wantError: false,
			validate: func(t *testing.T, session *LocalSession) {
				if session.Endpoint == nil {
					t.Fatal("Endpoint should not be nil")
				}
				if session.Endpoint.EndpointConfig.Authentication.AuthenticationPolicy.Bearer == nil {
					t.Fatal("Bearer auth should not be nil")
				}
				if session.Endpoint.EndpointConfig.Authentication.AuthenticationPolicy.Bearer.Token != "bearer-token-123" {
					t.Errorf("Token = %v, want bearer-token-123", session.Endpoint.EndpointConfig.Authentication.AuthenticationPolicy.Bearer.Token)
				}
			},
		},
		{
			name: "session with simple URI endpoint",
			yaml: `version: 1
expiry: 2026-01-04T10:33:08Z
session: test-session-token
endpoint: https://example.com
`,
			wantError: false,
			validate: func(t *testing.T, session *LocalSession) {
				if session.Endpoint == nil {
					t.Fatal("Endpoint should not be nil")
				}
				if session.Endpoint.String() != "https://example.com" {
					t.Errorf("Endpoint URI = %v, want https://example.com", session.Endpoint.String())
				}
			},
		},
		{
			name: "session without endpoint",
			yaml: `version: 1
expiry: 2026-01-04T10:33:08Z
session: test-session-token
`,
			wantError: false,
			validate: func(t *testing.T, session *LocalSession) {
				if session.Version != 1 {
					t.Errorf("Version = %v, want 1", session.Version)
				}
				if session.Endpoint != nil {
					t.Error("Endpoint should be nil")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var session LocalSession
			err := yaml.Unmarshal([]byte(tt.yaml), &session)

			if (err != nil) != tt.wantError {
				t.Errorf("UnmarshalYAML() error = %v, wantError %v", err, tt.wantError)
				return
			}

			if !tt.wantError && tt.validate != nil {
				tt.validate(t, &session)
			}
		})
	}
}

func TestLocalSession_RoundTrip(t *testing.T) {
	tests := []struct {
		name    string
		session LocalSession
	}{
		{
			name: "complete session with basic auth",
			session: LocalSession{
				Version: 1,
				Expiry:  time.Date(2026, 1, 4, 10, 33, 8, 0, time.UTC),
				Session: "test-session-token",
				Endpoint: &model.Endpoint{
					EndpointConfig: &model.EndpointConfiguration{
						URI: &model.LiteralUri{Value: "https://github.com/serverlessworkflow/catalog"},
						Authentication: &model.ReferenceableAuthenticationPolicy{
							AuthenticationPolicy: &model.AuthenticationPolicy{
								Basic: &model.BasicAuthenticationPolicy{
									Username: "user",
									Password: "012345",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "session with bearer token",
			session: LocalSession{
				Version: 1,
				Expiry:  time.Date(2026, 1, 4, 10, 33, 8, 0, time.UTC),
				Session: "test-session-token",
				Endpoint: &model.Endpoint{
					EndpointConfig: &model.EndpointConfiguration{
						URI: &model.LiteralUri{Value: "https://api.example.com"},
						Authentication: &model.ReferenceableAuthenticationPolicy{
							AuthenticationPolicy: &model.AuthenticationPolicy{
								Bearer: &model.BearerAuthenticationPolicy{
									Token: "bearer-token-123",
								},
							},
						},
					},
				},
			},
		},
		{
			name: "simple session without endpoint",
			session: LocalSession{
				Version: 1,
				Expiry:  time.Date(2026, 1, 4, 10, 33, 8, 0, time.UTC),
				Session: "test-session-token",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Marshal to YAML
			data, err := yaml.Marshal(&tt.session)
			if err != nil {
				t.Fatalf("Marshal error = %v", err)
			}

			t.Logf("Marshaled YAML:\n%s", string(data))

			// Unmarshal back
			var decoded LocalSession
			err = yaml.Unmarshal(data, &decoded)
			if err != nil {
				t.Fatalf("Unmarshal error = %v", err)
			}

			// Compare key fields
			if decoded.Version != tt.session.Version {
				t.Errorf("Version mismatch: got %v, want %v", decoded.Version, tt.session.Version)
			}
			if !decoded.Expiry.Equal(tt.session.Expiry) {
				t.Errorf("Expiry mismatch: got %v, want %v", decoded.Expiry, tt.session.Expiry)
			}
			if decoded.Session != tt.session.Session {
				t.Errorf("Session mismatch: got %v, want %v", decoded.Session, tt.session.Session)
			}

			// Compare endpoint if present
			if tt.session.Endpoint != nil {
				if decoded.Endpoint == nil {
					t.Fatal("Decoded endpoint should not be nil")
				}
				if decoded.Endpoint.String() != tt.session.Endpoint.String() {
					t.Errorf("Endpoint URI mismatch: got %v, want %v", decoded.Endpoint.String(), tt.session.Endpoint.String())
				}
			}
		})
	}
}

func TestLocalSession_NoNullValues(t *testing.T) {
	// Test that demonstrates the original issue - marshaling should not include null values
	session := LocalSession{
		Version: 1,
		Expiry:  time.Date(2026, 1, 4, 10, 33, 8, 0, time.UTC),
		Session: "session-token-xyz",
		Endpoint: &model.Endpoint{
			EndpointConfig: &model.EndpointConfiguration{
				URI: &model.LiteralUri{Value: "https://agent.example.com"},
				Authentication: &model.ReferenceableAuthenticationPolicy{
					AuthenticationPolicy: &model.AuthenticationPolicy{
						ProxyBearer: &model.ProxyBearerAuthenticationPolicy{
							Token: "proxy-bearer-token",
						},
					},
				},
			},
		},
	}

	// Marshal to YAML
	data, err := yaml.Marshal(&session)
	if err != nil {
		t.Fatalf("Marshal error = %v", err)
	}

	yamlStr := string(data)
	t.Logf("Generated YAML:\n%s", yamlStr)

	// Verify no null values in output
	if strings.Contains(yamlStr, "null") {
		t.Errorf("YAML output should not contain 'null' values.\nGot: %s", yamlStr)
	}

	// Verify it contains expected fields
	expectedFields := []string{"version", "expiry", "session", "endpoint", "uri", "authentication", "proxy_bearer", "token"}
	for _, field := range expectedFields {
		if !strings.Contains(yamlStr, field) {
			t.Errorf("YAML should contain %q field", field)
		}
	}

	// Verify it does NOT contain unwanted fields (note: proxy_bearer contains bearer, so skip that check)
	unwantedFields := []string{"runtimeexpression", "uritemplate", "endpointconfig", "basic", "digest", "oauth2", "oidc", "authenticationpolicy"}
	for _, field := range unwantedFields {
		if strings.Contains(yamlStr, field) {
			t.Errorf("YAML should NOT contain %q field, but does.\nGot: %s", field, yamlStr)
		}
	}
}

// TestLocalSession_EndpointFromString tests the real-world scenario where endpoints are created
// using model.NewEndpoint(url) with a string URL, as done in the daemon auth code
func TestLocalSession_EndpointFromString(t *testing.T) {
	tests := []struct {
		name        string
		urlString   string
		wantError   bool
		validateURL func(t *testing.T, endpoint *model.Endpoint)
	}{
		{
			name:      "simple HTTPS URL",
			urlString: "https://auth.thand.io",
			wantError: false,
			validateURL: func(t *testing.T, endpoint *model.Endpoint) {
				if endpoint == nil {
					t.Fatal("Endpoint should not be nil")
				}
				if endpoint.String() != "https://auth.thand.io" {
					t.Errorf("Endpoint URL = %v, want https://auth.thand.io", endpoint.String())
				}
			},
		},
		{
			name:      "URL with port",
			urlString: "https://example.com:8080",
			wantError: false,
			validateURL: func(t *testing.T, endpoint *model.Endpoint) {
				if endpoint.String() != "https://example.com:8080" {
					t.Errorf("Endpoint URL = %v, want https://example.com:8080", endpoint.String())
				}
			},
		},
		{
			name:      "URL with path",
			urlString: "https://api.example.com/v1/auth",
			wantError: false,
			validateURL: func(t *testing.T, endpoint *model.Endpoint) {
				if endpoint.String() != "https://api.example.com/v1/auth" {
					t.Errorf("Endpoint URL = %v, want https://api.example.com/v1/auth", endpoint.String())
				}
			},
		},
		{
			name:      "localhost URL",
			urlString: "http://localhost:3000",
			wantError: false,
			validateURL: func(t *testing.T, endpoint *model.Endpoint) {
				if endpoint.String() != "http://localhost:3000" {
					t.Errorf("Endpoint URL = %v, want http://localhost:3000", endpoint.String())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create session with endpoint from string, mimicking the real code:
			// Endpoint: model.NewEndpoint(orgUrl.String())
			session := LocalSession{
				Version:  1,
				Expiry:   time.Date(2026, 2, 7, 21, 42, 24, 0, time.UTC),
				Session:  "test-session-token-12345",
				Endpoint: model.NewEndpoint(tt.urlString),
			}

			// Marshal to YAML
			data, err := yaml.Marshal(&session)
			if err != nil {
				t.Fatalf("Marshal error = %v", err)
			}

			yamlStr := string(data)
			t.Logf("Marshaled YAML:\n%s", yamlStr)

			// Verify no null values
			if strings.Contains(yamlStr, "null") {
				t.Errorf("YAML should not contain null values.\nGot: %s", yamlStr)
			}

			// Verify endpoint field is present
			if !strings.Contains(yamlStr, "endpoint") {
				t.Errorf("YAML should contain 'endpoint' field.\nGot: %s", yamlStr)
			}

			// Unmarshal back
			var decoded LocalSession
			err = yaml.Unmarshal(data, &decoded)
			if (err != nil) != tt.wantError {
				t.Fatalf("Unmarshal error = %v, wantError %v", err, tt.wantError)
			}

			if !tt.wantError {
				// Validate the decoded session
				if decoded.Version != session.Version {
					t.Errorf("Version mismatch: got %v, want %v", decoded.Version, session.Version)
				}
				if !decoded.Expiry.Equal(session.Expiry) {
					t.Errorf("Expiry mismatch: got %v, want %v", decoded.Expiry, session.Expiry)
				}
				if decoded.Session != session.Session {
					t.Errorf("Session mismatch: got %v, want %v", decoded.Session, session.Session)
				}

				// Validate endpoint
				if tt.validateURL != nil {
					tt.validateURL(t, decoded.Endpoint)
				}

				// Ensure endpoint URLs match
				if decoded.Endpoint.String() != session.Endpoint.String() {
					t.Errorf("Endpoint URL mismatch after round-trip: got %v, want %v",
						decoded.Endpoint.String(), session.Endpoint.String())
				}
			}
		})
	}
}

// TestLocalSession_UnmarshalYAML_StringEndpoint tests that YAML with endpoint as a string
// can be unmarshaled correctly (backward compatibility)
func TestLocalSession_UnmarshalYAML_StringEndpoint(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		validate func(t *testing.T, session *LocalSession)
	}{
		{
			name: "endpoint as string URL",
			yaml: `version: 1
expiry: 2026-02-07T21:42:24Z
session: test-session-token
endpoint: https://auth.thand.io
`,
			validate: func(t *testing.T, session *LocalSession) {
				if session.Endpoint == nil {
					t.Fatal("Endpoint should not be nil")
				}
				if session.Endpoint.String() != "https://auth.thand.io" {
					t.Errorf("Endpoint = %v, want https://auth.thand.io", session.Endpoint.String())
				}
			},
		},
		{
			name: "endpoint as string with port",
			yaml: `version: 1
expiry: 2026-02-07T21:42:24Z
session: test-session-token
endpoint: "https://example.com:8080"
`,
			validate: func(t *testing.T, session *LocalSession) {
				if session.Endpoint == nil {
					t.Fatal("Endpoint should not be nil")
				}
				if session.Endpoint.String() != "https://example.com:8080" {
					t.Errorf("Endpoint = %v, want https://example.com:8080", session.Endpoint.String())
				}
			},
		},
		{
			name: "endpoint as string with path",
			yaml: `version: 1
expiry: 2026-02-07T21:42:24Z
session: test-session-token
endpoint: "https://api.example.com/v1/auth"
`,
			validate: func(t *testing.T, session *LocalSession) {
				if session.Endpoint == nil {
					t.Fatal("Endpoint should not be nil")
				}
				if session.Endpoint.String() != "https://api.example.com/v1/auth" {
					t.Errorf("Endpoint = %v, want https://api.example.com/v1/auth", session.Endpoint.String())
				}
			},
		},
		{
			name: "endpoint as object with uri",
			yaml: `version: 1
expiry: 2026-02-07T21:42:24Z
session: test-session-token
endpoint:
  uri: https://auth.thand.io
`,
			validate: func(t *testing.T, session *LocalSession) {
				if session.Endpoint == nil {
					t.Fatal("Endpoint should not be nil")
				}
				if session.Endpoint.String() != "https://auth.thand.io" {
					t.Errorf("Endpoint = %v, want https://auth.thand.io", session.Endpoint.String())
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var session LocalSession
			err := yaml.Unmarshal([]byte(tt.yaml), &session)
			if err != nil {
				t.Fatalf("Unmarshal error = %v", err)
			}

			if tt.validate != nil {
				tt.validate(t, &session)
			}
		})
	}
}

// TestLocalSession_RoundTrip_NewEndpoint tests the full round-trip scenario matching the real-world usage
// This simulates: ExportableSession with Endpoint: model.NewEndpoint(orgUrl.String())
func TestLocalSession_RoundTrip_NewEndpoint(t *testing.T) {
	tests := []struct {
		name      string
		urlString string
		provider  string
	}{
		{
			name:      "thand provider with auth URL",
			urlString: "https://auth.thand.io",
			provider:  "thand",
		},
		{
			name:      "custom org URL",
			urlString: "https://myorg.example.com",
			provider:  "thand",
		},
		{
			name:      "localhost development",
			urlString: "http://localhost:8080",
			provider:  "thand",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the real code pattern from daemon/auth.go:
			// exportableSession := &thandModels.ExportableSession{
			//     Session:  session,
			//     Provider: "thand",
			//     Endpoint: model.NewEndpoint(orgUrl.String()),
			// }
			// localSession := exportableSession.ToLocalSession(encryptor)

			originalSession := LocalSession{
				Version:  1,
				Expiry:   time.Date(2026, 2, 7, 21, 42, 24, 0, time.UTC),
				Session:  "encoded-session-token-xyz",
				Endpoint: model.NewEndpoint(tt.urlString),
			}

			// Step 1: Marshal to YAML (simulates storage)
			yamlData, err := yaml.Marshal(&originalSession)
			if err != nil {
				t.Fatalf("Marshal error = %v", err)
			}

			t.Logf("Stored YAML:\n%s", string(yamlData))

			// Verify YAML quality
			yamlStr := string(yamlData)
			if strings.Contains(yamlStr, "null") {
				t.Errorf("YAML should not contain null values")
			}
			if !strings.Contains(yamlStr, "endpoint") {
				t.Errorf("YAML should contain endpoint field")
			}

			// Step 2: Unmarshal from YAML (simulates retrieval)
			var retrievedSession LocalSession
			err = yaml.Unmarshal(yamlData, &retrievedSession)
			if err != nil {
				t.Fatalf("Unmarshal error = %v", err)
			}

			// Step 3: Verify integrity
			if retrievedSession.Version != originalSession.Version {
				t.Errorf("Version mismatch: got %v, want %v", retrievedSession.Version, originalSession.Version)
			}
			if !retrievedSession.Expiry.Equal(originalSession.Expiry) {
				t.Errorf("Expiry mismatch: got %v, want %v", retrievedSession.Expiry, originalSession.Expiry)
			}
			if retrievedSession.Session != originalSession.Session {
				t.Errorf("Session token mismatch: got %v, want %v", retrievedSession.Session, originalSession.Session)
			}

			// Step 4: Verify endpoint URL is preserved
			if retrievedSession.Endpoint == nil {
				t.Fatal("Retrieved endpoint should not be nil")
			}
			if retrievedSession.Endpoint.String() != originalSession.Endpoint.String() {
				t.Errorf("Endpoint URL mismatch: got %v, want %v",
					retrievedSession.Endpoint.String(), originalSession.Endpoint.String())
			}
			if retrievedSession.Endpoint.String() != tt.urlString {
				t.Errorf("Endpoint URL should match original: got %v, want %v",
					retrievedSession.Endpoint.String(), tt.urlString)
			}
		})
	}
}
