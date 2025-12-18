package saml

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

func TestSAMLProvider_ParseSAMLConfig(t *testing.T) {
	certFile, keyFile := generateTempCert(t)
	defer os.Remove(certFile)
	defer os.Remove(keyFile)

	tests := []struct {
		name           string
		config         *models.BasicConfig
		expectError    bool
		errorContains  string
		validateResult func(t *testing.T, cfg *SAMLConfig)
	}{
		{
			name:          "Nil config",
			config:        nil,
			expectError:   true,
			errorContains: "config is nil",
		},
		{
			name: "Missing required fields",
			config: &models.BasicConfig{
				"idp_metadata_url": "https://example.com/metadata",
			},
			expectError:   true,
			errorContains: "is required",
		},
		{
			name: "Valid config with all fields",
			config: &models.BasicConfig{
				"idp_metadata_url": "https://example.com/metadata",
				"entity_id":        "https://myapp.com/saml",
				"root_url":         "https://myapp.com",
				"cert_file":        certFile,
				"key_file":         keyFile,
				"sign_requests":    true,
			},
			expectError: false,
			validateResult: func(t *testing.T, cfg *SAMLConfig) {
				assert.Equal(t, "https://example.com/metadata", cfg.IDPMetadataURL)
				assert.Equal(t, "https://myapp.com/saml", cfg.EntityID)
				assert.Equal(t, "https://myapp.com", cfg.RootURL)
				assert.True(t, cfg.SignRequests)
			},
		},
		{
			name: "Valid config without sign_requests (defaults to false)",
			config: &models.BasicConfig{
				"idp_metadata_url": "https://example.com/metadata",
				"entity_id":        "https://myapp.com/saml",
				"root_url":         "https://myapp.com",
				"cert_file":        certFile,
				"key_file":         keyFile,
			},
			expectError: false,
			validateResult: func(t *testing.T, cfg *SAMLConfig) {
				assert.False(t, cfg.SignRequests)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &samlProvider{}

			samlConfig, err := provider.parseSAMLConfig(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				if len(tt.errorContains) != 0 {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, samlConfig)
				if tt.validateResult != nil {
					tt.validateResult(t, samlConfig)
				}
			}
		})
	}
}

func TestSAMLProvider_SessionValidation(t *testing.T) {
	tests := []struct {
		name          string
		session       *models.Session
		expectError   bool
		errorContains string
	}{
		{
			name:          "Nil session",
			session:       nil,
			expectError:   true,
			errorContains: "session is nil",
		},
		{
			name: "Session without user",
			session: &models.Session{
				UUID:        uuid.New(),
				AccessToken: "test-token",
				Expiry:      time.Now().Add(1 * time.Hour),
			},
			expectError:   true,
			errorContains: "user is nil",
		},
		{
			name: "Expired session",
			session: &models.Session{
				UUID:        uuid.New(),
				User:        &models.User{Username: "testuser"},
				AccessToken: "test-token",
				Expiry:      time.Now().Add(-1 * time.Hour),
			},
			expectError:   true,
			errorContains: "expired",
		},
		{
			name: "Session without access token",
			session: &models.Session{
				UUID:        uuid.New(),
				User:        &models.User{Username: "testuser"},
				AccessToken: "",
				Expiry:      time.Now().Add(1 * time.Hour),
			},
			expectError:   true,
			errorContains: "invalid access token",
		},
		{
			name: "Valid session",
			session: &models.Session{
				UUID:        uuid.New(),
				User:        &models.User{Username: "testuser"},
				AccessToken: "test-token",
				Expiry:      time.Now().Add(1 * time.Hour),
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &samlProvider{}
			ctx := context.Background()

			err := provider.ValidateSession(ctx, tt.session)

			if tt.expectError {
				assert.Error(t, err)
				if len(tt.errorContains) != 0 {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// Helper function to create a test certificate
func createTestCert(t *testing.T) (cert *x509.Certificate, key *rsa.PrivateKey) {
	t.Helper()

	// Generate RSA key
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)

	// Create certificate template
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"Test Org"},
			CommonName:   "test.example.com",
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}

	// Create certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	require.NoError(t, err)

	cert, err = x509.ParseCertificate(certBytes)
	require.NoError(t, err)

	return cert, key
}

// Helper function to write certificate and key to temporary files
func writeCertAndKeyToFiles(t *testing.T, cert *x509.Certificate, key *rsa.PrivateKey) (certFile, keyFile string) {
	t.Helper()

	tempDir := t.TempDir()

	// Write certificate
	certFile = filepath.Join(tempDir, "cert.pem")
	certPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: cert.Raw,
	})
	err := os.WriteFile(certFile, certPEM, 0644)
	require.NoError(t, err)

	// Write private key
	keyFile = filepath.Join(tempDir, "key.pem")
	keyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key),
	})
	err = os.WriteFile(keyFile, keyPEM, 0600)
	require.NoError(t, err)

	return certFile, keyFile
}

// Helper function to create a mock IDP metadata server
func createMockIDPMetadataServer(t *testing.T) *httptest.Server {
	t.Helper()

	metadataXML := `<?xml version="1.0" encoding="UTF-8"?>
<EntityDescriptor xmlns="urn:oasis:names:tc:SAML:2.0:metadata" entityID="https://idp.example.com">
  <IDPSSODescriptor protocolSupportEnumeration="urn:oasis:names:tc:SAML:2.0:protocol">
    <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST" Location="https://idp.example.com/sso"/>
    <SingleSignOnService Binding="urn:oasis:names:tc:SAML:2.0:bindings:HTTP-Redirect" Location="https://idp.example.com/sso"/>
  </IDPSSODescriptor>
</EntityDescriptor>`

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(metadataXML))
	}))

	return server
}

// Helper function to create a test SAML provider with mock setup
func createTestSAMLProvider(t *testing.T) *samlProvider {
	t.Helper()

	cert, key := createTestCert(t)

	rootURL, err := url.Parse("https://test.example.com")
	require.NoError(t, err)

	acsURL := *rootURL
	acsURL.Path = "/api/v1/auth/callback/test-saml"

	metadataURL := *rootURL
	metadataURL.Path = "/saml/metadata"

	// Create IDP metadata
	idpMetadata := &saml.EntityDescriptor{
		EntityID: "https://idp.example.com",
		IDPSSODescriptors: []saml.IDPSSODescriptor{
			{
				SingleSignOnServices: []saml.Endpoint{
					{
						Binding:  "urn:oasis:names:tc:SAML:2.0:bindings:HTTP-POST",
						Location: "https://idp.example.com/sso",
					},
				},
			},
		},
	}

	sp := &saml.ServiceProvider{
		EntityID:          "test-entity-id",
		Key:               key,
		Certificate:       cert,
		MetadataURL:       metadataURL,
		AcsURL:            acsURL,
		IDPMetadata:       idpMetadata,
		AuthnNameIDFormat: saml.EmailAddressNameIDFormat,
		AllowIDPInitiated: true,
	}

	middleware := &samlsp.Middleware{
		ServiceProvider: *sp,
	}

	return &samlProvider{
		middleware:  middleware,
		idpMetadata: idpMetadata,
		certificates: []tls.Certificate{
			{
				Certificate: [][]byte{cert.Raw},
				PrivateKey:  key,
				Leaf:        cert,
			},
		},
	}
}

// TestSAMLProvider_CreateSession tests CreateSession with various scenarios
func TestSAMLProvider_CreateSession(t *testing.T) {
	tests := []struct {
		name           string
		setupProvider  func(t *testing.T) *samlProvider
		authRequest    *models.AuthorizeUser
		expectError    bool
		errorContains  string
		validateResult func(t *testing.T, session *models.Session)
	}{
		{
			name: "Uninitialized provider",
			setupProvider: func(t *testing.T) *samlProvider {
				return &samlProvider{}
			},
			authRequest: &models.AuthorizeUser{
				Code:  "test-saml-response",
				State: "test-state",
			},
			expectError:   true,
			errorContains: "not initialized",
		},
		{
			name: "Empty SAML response code",
			setupProvider: func(t *testing.T) *samlProvider {
				return createTestSAMLProvider(t)
			},
			authRequest: &models.AuthorizeUser{
				Code:  "",
				State: "test-state",
			},
			expectError:   true,
			errorContains: "no SAML response provided",
		},
		{
			name: "Invalid SAML response format",
			setupProvider: func(t *testing.T) *samlProvider {
				return createTestSAMLProvider(t)
			},
			authRequest: &models.AuthorizeUser{
				Code:  "invalid-saml-response",
				State: "test-state",
			},
			expectError:   true,
			errorContains: "failed to parse SAML response",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := tt.setupProvider(t)
			ctx := context.Background()

			session, err := provider.CreateSession(ctx, tt.authRequest)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, session)
				if len(tt.errorContains) != 0 {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, session)
				if tt.validateResult != nil {
					tt.validateResult(t, session)
				}
			}
		})
	}
}

// TestSAMLProvider_AuthorizeSession tests SAML authorization request generation
func TestSAMLProvider_AuthorizeSession(t *testing.T) {
	tests := []struct {
		name           string
		setupProvider  func(t *testing.T) *samlProvider
		authRequest    *models.AuthorizeUser
		expectError    bool
		errorContains  string
		validateResult func(t *testing.T, response *models.AuthorizeSessionResponse)
	}{
		{
			name: "Successful authorization request",
			setupProvider: func(t *testing.T) *samlProvider {
				return createTestSAMLProvider(t)
			},
			authRequest: &models.AuthorizeUser{
				State:       "test-state",
				RedirectUri: "https://test.example.com/callback",
			},
			expectError: false,
			validateResult: func(t *testing.T, response *models.AuthorizeSessionResponse) {
				assert.NotEmpty(t, response.Url)
				authURL, err := url.Parse(response.Url)
				require.NoError(t, err)
				// SAML redirect binding URL should contain SAMLRequest parameter
				assert.Contains(t, authURL.String(), "SAMLRequest")
			},
		},
		{
			name: "Uninitialized provider",
			setupProvider: func(t *testing.T) *samlProvider {
				return &samlProvider{}
			},
			authRequest: &models.AuthorizeUser{
				State:       "test-state",
				RedirectUri: "https://test.example.com/callback",
			},
			expectError:   true,
			errorContains: "not initialized",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := tt.setupProvider(t)
			ctx := context.Background()

			response, err := provider.AuthorizeSession(ctx, tt.authRequest)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, response)
				if len(tt.errorContains) != 0 {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, response)
				if tt.validateResult != nil {
					tt.validateResult(t, response)
				}
			}
		})
	}
}

// TestSAMLProvider_RenewSession tests session renewal
func TestSAMLProvider_RenewSession(t *testing.T) {
	tests := []struct {
		name           string
		session        *models.Session
		expectError    bool
		errorContains  string
		validateResult func(t *testing.T, original *models.Session, renewed *models.Session)
	}{
		{
			name: "Renew valid session",
			session: &models.Session{
				UUID:         uuid.New(),
				User:         &models.User{Username: "testuser"},
				AccessToken:  "test-token",
				RefreshToken: "test-refresh-token",
				Expiry:       time.Now().Add(1 * time.Hour),
			},
			expectError: false,
			validateResult: func(t *testing.T, original *models.Session, renewed *models.Session) {
				assert.NotEqual(t, original.UUID, renewed.UUID)
				assert.Equal(t, original.User, renewed.User)
				assert.True(t, renewed.Expiry.After(original.Expiry))
			},
		},
		{
			name: "Cannot renew expired session",
			session: &models.Session{
				UUID:         uuid.New(),
				User:         &models.User{Username: "testuser"},
				AccessToken:  "test-token",
				RefreshToken: "test-refresh-token",
				Expiry:       time.Now().Add(-1 * time.Hour),
			},
			expectError:   true,
			errorContains: "cannot renew expired session",
		},
		{
			name:          "Cannot renew nil session",
			session:       nil,
			expectError:   true,
			errorContains: "session is nil",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &samlProvider{}
			ctx := context.Background()

			renewedSession, err := provider.RenewSession(ctx, tt.session)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, renewedSession)
				if len(tt.errorContains) != 0 {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, renewedSession)
				if tt.validateResult != nil {
					tt.validateResult(t, tt.session, renewedSession)
				}
			}
		})
	}
}

// TestSAMLProvider_ValidateRole tests role validation
func TestSAMLProvider_ValidateRole(t *testing.T) {
	tests := []struct {
		name        string
		identity    *models.Identity
		role        *models.Role
		expectError bool
	}{
		{
			name: "Valid identity and role",
			identity: &models.Identity{
				ID:    "testuser@example.com",
				Label: "Test User",
			},
			role: &models.Role{
				Name: "test-role",
			},
			expectError: false,
		},
		{
			name:        "Nil identity",
			identity:    nil,
			role:        &models.Role{Name: "test-role"},
			expectError: true,
		},
		{
			name: "Nil role",
			identity: &models.Identity{
				ID: "testuser@example.com",
			},
			role:        nil,
			expectError: true,
		},
		{
			name:        "Both nil",
			identity:    nil,
			role:        nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &samlProvider{}
			ctx := context.Background()

			result, err := provider.ValidateRole(ctx, tt.identity, tt.role)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Nil(t, result)
			}
		})
	}
}

// TestSAMLProvider_Initialize tests provider initialization with various scenarios
func TestSAMLProvider_Initialize(t *testing.T) {
	tests := []struct {
		name          string
		setupConfig   func(t *testing.T) models.Provider
		expectError   bool
		errorContains string
		validate      func(t *testing.T, p *samlProvider)
	}{
		{
			name: "Successful initialization with valid config",
			setupConfig: func(t *testing.T) models.Provider {
				cert, key := createTestCert(t)
				certFile, keyFile := writeCertAndKeyToFiles(t, cert, key)
				idpServer := createMockIDPMetadataServer(t)
				t.Cleanup(idpServer.Close)

				return models.Provider{
					Name:     "test-saml",
					Provider: SamlProviderName,
					Config: &models.BasicConfig{
						"idp_metadata_url": idpServer.URL,
						"entity_id":        "test-entity",
						"root_url":         "https://test.example.com",
						"cert_file":        certFile,
						"key_file":         keyFile,
					},
				}
			},
			expectError: false,
			validate: func(t *testing.T, p *samlProvider) {
				assert.NotNil(t, p.middleware)
				assert.NotNil(t, p.idpMetadata)
				assert.NotEmpty(t, p.certificates)
				assert.Equal(t, "test-entity", p.middleware.ServiceProvider.EntityID)
				assert.Contains(t, p.middleware.ServiceProvider.AcsURL.String(), "/api/v1/auth/callback/test-saml")
			},
		},
		{
			name: "Invalid IDP metadata URL",
			setupConfig: func(t *testing.T) models.Provider {
				cert, key := createTestCert(t)
				certFile, keyFile := writeCertAndKeyToFiles(t, cert, key)

				return models.Provider{
					Name:     "test-saml",
					Provider: SamlProviderName,
					Config: &models.BasicConfig{
						"idp_metadata_url": "https://invalid-url-that-does-not-exist.example.com/metadata",
						"entity_id":        "test-entity",
						"root_url":         "https://test.example.com",
						"cert_file":        certFile,
						"key_file":         keyFile,
					},
				}
			},
			expectError:   true,
			errorContains: "failed to fetch IdP metadata",
		},
		{
			name: "Certificate file does not exist",
			setupConfig: func(t *testing.T) models.Provider {
				idpServer := createMockIDPMetadataServer(t)
				t.Cleanup(idpServer.Close)

				return models.Provider{
					Name:     "test-saml",
					Provider: SamlProviderName,
					Config: &models.BasicConfig{
						"idp_metadata_url": idpServer.URL,
						"entity_id":        "test-entity",
						"root_url":         "https://test.example.com",
						"cert_file":        "/tmp/nonexistent-cert-file.pem",
						"key_file":         "/tmp/nonexistent-key-file.pem",
					},
				}
			},
			expectError:   true,
			errorContains: "failed to load SAML certificate",
		},
		{
			name: "Invalid root URL",
			setupConfig: func(t *testing.T) models.Provider {
				cert, key := createTestCert(t)
				certFile, keyFile := writeCertAndKeyToFiles(t, cert, key)
				idpServer := createMockIDPMetadataServer(t)
				t.Cleanup(idpServer.Close)

				return models.Provider{
					Name:     "test-saml",
					Provider: SamlProviderName,
					Config: &models.BasicConfig{
						"idp_metadata_url": idpServer.URL,
						"entity_id":        "test-entity",
						"root_url":         "://invalid-url",
						"cert_file":        certFile,
						"key_file":         keyFile,
					},
				}
			},
			expectError:   true,
			errorContains: "invalid root URL",
		},
		{
			name: "Invalid config format",
			setupConfig: func(t *testing.T) models.Provider {
				return models.Provider{
					Name:     "test-saml",
					Provider: SamlProviderName,
					Config: &models.BasicConfig{
						"idp_metadata_url": "https://example.com/metadata",
						// Missing required fields
					},
				}
			},
			expectError:   true,
			errorContains: "failed to parse SAML config",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provider := &samlProvider{}
			config := tt.setupConfig(t)

			err := provider.Initialize("test-saml", config)

			if tt.expectError {
				assert.Error(t, err)
				if len(tt.errorContains) != 0 {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
			} else {
				assert.NoError(t, err)
				if tt.validate != nil {
					tt.validate(t, provider)
				}
			}
		})
	}
}
