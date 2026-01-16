package daemon

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/thand-io/agent/internal/models"
)

func TestGetCookieDomain(t *testing.T) {
	tests := []struct {
		name           string
		hostname       string
		expectedDomain string
		description    string
	}{
		{
			name:           "localhost should use host-only cookies",
			hostname:       "localhost",
			expectedDomain: "",
			description:    "localhost should not set Domain attribute for security",
		},
		{
			name:           "127.0.0.1 should use host-only cookies",
			hostname:       "127.0.0.1",
			expectedDomain: "",
			description:    "loopback IP should not set Domain attribute",
		},
		{
			name:           "empty hostname should use host-only cookies",
			hostname:       "",
			expectedDomain: "",
			description:    "empty hostname should not set Domain attribute",
		},
		{
			name:           "Azure Container Apps with subdomain",
			hostname:       "thand.livelysand-47c199af.eastus.azurecontainerapps.io",
			expectedDomain: ".azurecontainerapps.io",
			description:    "Azure Container Apps should extract parent domain (azurecontainerapps.io is in public suffix list)",
		},
		{
			name:           "Azure Container Apps different region",
			hostname:       "myapp.subdomain.westus2.azurecontainerapps.io",
			expectedDomain: ".azurecontainerapps.io",
			description:    "Azure Container Apps in different region should work",
		},
		{
			name:           "AWS App Runner with subdomain",
			hostname:       "myapp-abc123.us-east-1.awsapprunner.com",
			expectedDomain: "",
			description:    "AWS App Runner - awsapprunner.com is a public suffix, uses host-only",
		},
		{
			name:           "GCP Cloud Run with subdomain",
			hostname:       "my-service-abc123.europe-west1.run.app",
			expectedDomain: "",
			description:    "GCP Cloud Run - run.app is a public suffix, uses host-only",
		},
		{
			name:           "GCP Cloud Run different region",
			hostname:       "service-xyz.us-central1.run.app",
			expectedDomain: "",
			description:    "GCP Cloud Run in different region - run.app is a public suffix",
		},
		{
			name:           "simple custom domain",
			hostname:       "example.com",
			expectedDomain: "",
			description:    "base domain should use host-only cookies",
		},
		{
			name:           "subdomain on custom domain",
			hostname:       "app.example.com",
			expectedDomain: ".example.com",
			description:    "subdomain should set parent domain",
		},
		{
			name:           "multi-level subdomain on custom domain",
			hostname:       "api.staging.example.com",
			expectedDomain: ".example.com",
			description:    "multi-level subdomain should set base domain",
		},
		{
			name:           "UK domain",
			hostname:       "app.example.co.uk",
			expectedDomain: ".example.co.uk",
			description:    "should handle multi-part TLD correctly",
		},
		{
			name:           "base UK domain",
			hostname:       "example.co.uk",
			expectedDomain: "",
			description:    "base domain with multi-part TLD should use host-only",
		},
		{
			name:           "GitHub Pages",
			hostname:       "username.github.io",
			expectedDomain: "",
			description:    "github.io is a public suffix, should use host-only",
		},
		{
			name:           "Heroku app",
			hostname:       "myapp-staging.herokuapp.com",
			expectedDomain: "",
			description:    "Heroku - herokuapp.com is a public suffix, uses host-only",
		},
		{
			name:           "Vercel deployment",
			hostname:       "myproject.vercel.app",
			expectedDomain: "",
			description:    "vercel.app is a public suffix, should use host-only",
		},
		{
			name:           "Netlify deployment",
			hostname:       "mysite.netlify.app",
			expectedDomain: "",
			description:    "netlify.app is a public suffix, should use host-only",
		},
		{
			name:           "AWS CloudFront",
			hostname:       "d1234567890.cloudfront.net",
			expectedDomain: "",
			description:    "cloudfront.net is a public suffix, should use host-only",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getCookieDomain(tt.hostname)
			assert.Equal(t, tt.expectedDomain, result, tt.description)
		})
	}
}

func TestGetCookieDomain_SecurityCases(t *testing.T) {
	t.Run("should not allow overly broad domain", func(t *testing.T) {
		// Test that we don't accidentally set cookie for entire TLD
		hostname := "example.com"
		result := getCookieDomain(hostname)
		assert.Equal(t, "", result, "base domain should not set Domain attribute to avoid overly broad scope")
	})

	t.Run("should handle malformed hostnames gracefully", func(t *testing.T) {
		// Test various malformed inputs
		malformedHosts := []string{
			"not..valid",
			".invalid",
			"invalid.",
			"some space.com",
		}

		for _, hostname := range malformedHosts {
			result := getCookieDomain(hostname)
			// Should either return empty string or a valid domain, never panic
			assert.NotPanics(t, func() {
				getCookieDomain(hostname)
			}, "should not panic on malformed hostname: %s", hostname)
			t.Logf("Malformed hostname '%s' returned domain: '%s'", hostname, result)
		}
	})
}

func TestGetCookieDomain_PlatformSpecific(t *testing.T) {
	t.Run("Azure Container Apps variations", func(t *testing.T) {
		testCases := []struct {
			hostname string
			expected string
		}{
			{"app.eastus.azurecontainerapps.io", ".azurecontainerapps.io"},
			{"my-app.westus.azurecontainerapps.io", ".azurecontainerapps.io"},
			{"service.northeurope.azurecontainerapps.io", ".azurecontainerapps.io"},
			{"test.sub.eastus2.azurecontainerapps.io", ".azurecontainerapps.io"},
		}

		for _, tc := range testCases {
			result := getCookieDomain(tc.hostname)
			assert.Equal(t, tc.expected, result, "Azure Container Apps hostname: %s", tc.hostname)
		}
	})

	t.Run("AWS App Runner variations", func(t *testing.T) {
		testCases := []struct {
			hostname string
			expected string
		}{
			{"abc123.us-east-1.awsapprunner.com", ""},
			{"xyz789.eu-west-1.awsapprunner.com", ""},
			{"service.us-west-2.awsapprunner.com", ""},
		}

		for _, tc := range testCases {
			result := getCookieDomain(tc.hostname)
			assert.Equal(t, tc.expected, result, "AWS App Runner hostname: %s", tc.hostname)
		}
	})

	t.Run("GCP Cloud Run variations", func(t *testing.T) {
		testCases := []struct {
			hostname string
			expected string
		}{
			{"service-abc.us-central1.run.app", ""},
			{"app-xyz.europe-west1.run.app", ""},
			{"api-123.asia-southeast1.run.app", ""},
		}

		for _, tc := range testCases {
			result := getCookieDomain(tc.hostname)
			assert.Equal(t, tc.expected, result, "GCP Cloud Run hostname: %s", tc.hostname)
		}
	})
}

// Test session encoding and decoding to validate user data preservation
func TestSessionEncodingDecoding(t *testing.T) {
	encryptor := newMockEncryptor()

	t.Run("encode and decode session with user", func(t *testing.T) {
		// Create a session with user data
		user := &models.User{
			ID:       "user-123",
			Username: "testuser",
			Email:    "test@example.com",
			Name:     "Test User",
			Groups:   []string{"admin", "developers"},
			Source:   "github",
		}

		session := &models.Session{
			UUID:         uuid.New(),
			User:         user,
			Token:        "test-token-123",
			AccessToken:  "access-token-abc",
			RefreshToken: "refresh-token-xyz",
			Expiry:       time.Now().Add(1 * time.Hour),
		}

		exportableSession := &models.ExportableSession{
			Session:  session,
			Provider: "github",
		}

		// Encode the session
		encoded := exportableSession.GetEncodedSession(encryptor)
		assert.NotEmpty(t, encoded, "Encoded session should not be empty")

		// Create a LocalSession
		localSession := exportableSession.ToLocalSession(encryptor)
		assert.NotNil(t, localSession, "LocalSession should not be nil")
		assert.Equal(t, encoded, localSession.Session, "LocalSession should contain the encoded session")

		// Decode the session
		decoded, err := localSession.GetDecodedSession(encryptor)
		assert.NoError(t, err, "Should decode session without error")
		assert.NotNil(t, decoded, "Decoded session should not be nil")

		// Verify provider is preserved
		assert.Equal(t, "github", decoded.Provider, "Provider should be preserved")

		// Verify session fields are preserved
		assert.NotNil(t, decoded.Session, "Decoded Session should not be nil")
		if decoded.Session != nil {
			assert.Equal(t, session.UUID, decoded.Session.UUID, "UUID should be preserved")
			assert.Equal(t, session.Token, decoded.Session.Token, "Token should be preserved")
			assert.Equal(t, session.AccessToken, decoded.Session.AccessToken, "AccessToken should be preserved")
			assert.Equal(t, session.RefreshToken, decoded.Session.RefreshToken, "RefreshToken should be preserved")
			assert.True(t, session.Expiry.Equal(decoded.Session.Expiry), "Expiry should be preserved")

			// Verify user data is preserved
			assert.NotNil(t, decoded.Session.User, "User should not be nil after decoding")
			if decoded.Session.User != nil {
				assert.Equal(t, user.ID, decoded.Session.User.ID, "User ID should be preserved")
				assert.Equal(t, user.Username, decoded.Session.User.Username, "Username should be preserved")
				assert.Equal(t, user.Email, decoded.Session.User.Email, "Email should be preserved")
				assert.Equal(t, user.Name, decoded.Session.User.Name, "Name should be preserved")
				assert.Equal(t, user.Source, decoded.Session.User.Source, "Source should be preserved")
				assert.Equal(t, user.Groups, decoded.Session.User.Groups, "Groups should be preserved")
			}
		}
	})

	t.Run("encode and decode session without user", func(t *testing.T) {
		session := &models.Session{
			UUID:        uuid.New(),
			User:        nil, // No user
			Token:       "test-token-456",
			AccessToken: "access-token-def",
			Expiry:      time.Now().Add(2 * time.Hour),
		}

		exportableSession := &models.ExportableSession{
			Session:  session,
			Provider: "okta",
		}

		// Encode and decode
		localSession := exportableSession.ToLocalSession(encryptor)
		decoded, err := localSession.GetDecodedSession(encryptor)

		assert.NoError(t, err, "Should decode session without error")
		assert.NotNil(t, decoded, "Decoded session should not be nil")
		assert.Equal(t, "okta", decoded.Provider, "Provider should be preserved")

		if decoded.Session != nil {
			assert.Equal(t, session.Token, decoded.Session.Token, "Token should be preserved")
			// User should be nil since we didn't set one
			// Note: This might fail if embedded struct doesn't handle nil properly
		}
	})

	t.Run("encode and decode multiple sessions", func(t *testing.T) {
		sessions := []*models.ExportableSession{
			{
				Session: &models.Session{
					UUID:        uuid.New(),
					User:        &models.User{ID: "user-1", Email: "user1@example.com", Name: "User One"},
					Token:       "token-1",
					AccessToken: "access-1",
					Expiry:      time.Now().Add(1 * time.Hour),
				},
				Provider: "github",
			},
			{
				Session: &models.Session{
					UUID:        uuid.New(),
					User:        &models.User{ID: "user-2", Email: "user2@example.com", Name: "User Two"},
					Token:       "token-2",
					AccessToken: "access-2",
					Expiry:      time.Now().Add(2 * time.Hour),
				},
				Provider: "okta",
			},
			{
				Session: &models.Session{
					UUID:        uuid.New(),
					User:        &models.User{ID: "user-3", Email: "user3@example.com", Name: "User Three"},
					Token:       "token-3",
					AccessToken: "access-3",
					Expiry:      time.Now().Add(3 * time.Hour),
				},
				Provider: "aws",
			},
		}

		for i, exportableSession := range sessions {
			// Encode and decode each session
			localSession := exportableSession.ToLocalSession(encryptor)
			decoded, err := localSession.GetDecodedSession(encryptor)

			assert.NoError(t, err, "Session %d: Should decode without error", i)
			assert.NotNil(t, decoded, "Session %d: Decoded session should not be nil", i)
			assert.Equal(t, exportableSession.Provider, decoded.Provider, "Session %d: Provider should match", i)

			if decoded.Session != nil && decoded.Session.User != nil {
				assert.Equal(t, exportableSession.Session.User.ID, decoded.Session.User.ID,
					"Session %d: User ID should be preserved", i)
				assert.Equal(t, exportableSession.Session.User.Email, decoded.Session.User.Email,
					"Session %d: User Email should be preserved", i)
				assert.Equal(t, exportableSession.Session.User.Name, decoded.Session.User.Name,
					"Session %d: User Name should be preserved", i)
			}
		}
	})
}

// Mock encryptor for testing (no actual encryption)
type mockEncryptor struct{}

func newMockEncryptor() *mockEncryptor {
	return &mockEncryptor{}
}

func (m *mockEncryptor) Initialize() error {
	return nil
}

func (m *mockEncryptor) Shutdown() error {
	return nil
}

func (m *mockEncryptor) Encrypt(ctx context.Context, data []byte) ([]byte, error) {
	return data, nil
}

func (m *mockEncryptor) Decrypt(ctx context.Context, data []byte) ([]byte, error) {
	return data, nil
}
