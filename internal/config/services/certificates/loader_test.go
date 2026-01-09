package certificates

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// mockVault implements VaultImpl for testing
type mockVault struct {
	secrets map[string][]byte
	err     error
}

func (m *mockVault) Initialize() error                          { return nil }
func (m *mockVault) Shutdown() error                            { return nil }
func (m *mockVault) StoreSecret(key string, value []byte) error { return nil }

func (m *mockVault) GetSecret(key string) ([]byte, error) {
	if m.err != nil {
		return nil, m.err
	}
	if data, ok := m.secrets[key]; ok {
		return data, nil
	}
	return nil, fmt.Errorf("secret not found: %s", key)
}

// Test helpers
func generateTestCertAndKey(t *testing.T) (certPEM, keyPEM []byte) {
	t.Helper()

	// Generate RSA private key
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
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
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
	}

	// Create certificate
	certBytes, err := x509.CreateCertificate(rand.Reader, &template, &template, &privKey.PublicKey, privKey)
	require.NoError(t, err)

	// Encode certificate to PEM
	certPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE",
		Bytes: certBytes,
	})

	// Encode private key to PEM
	keyPEM = pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privKey),
	})

	return certPEM, keyPEM
}

func writeToTempFile(t *testing.T, data []byte) string {
	t.Helper()

	tmpFile, err := os.CreateTemp(t.TempDir(), "test-cert-*.pem")
	require.NoError(t, err)
	defer tmpFile.Close()

	_, err = tmpFile.Write(data)
	require.NoError(t, err)

	return tmpFile.Name()
}

// Tests for ParseCombinedPEM
func TestParseCombinedPEM(t *testing.T) {
	certPEM, keyPEM := generateTestCertAndKey(t)

	tests := []struct {
		name          string
		input         []byte
		expectError   bool
		errorContains string
	}{
		{
			name:        "Valid combined PEM (cert first, then key)",
			input:       append(certPEM, keyPEM...),
			expectError: false,
		},
		{
			name:        "Valid combined PEM (key first, then cert)",
			input:       append(keyPEM, certPEM...),
			expectError: false,
		},
		{
			name:          "Missing certificate",
			input:         keyPEM,
			expectError:   true,
			errorContains: "no certificate block found",
		},
		{
			name:          "Missing private key",
			input:         certPEM,
			expectError:   true,
			errorContains: "no private key block found",
		},
		{
			name:          "Multiple private keys",
			input:         append(append(certPEM, keyPEM...), keyPEM...),
			expectError:   true,
			errorContains: "multiple private key blocks",
		},
		{
			name:          "Empty input",
			input:         []byte{},
			expectError:   true,
			errorContains: "no certificate block found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cert, key, err := ParseCombinedPEM(tt.input)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, cert)
				assert.Nil(t, key)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, cert)
				assert.NotNil(t, key)
			}
		})
	}
}

// Tests for LoadCertificate with inline source
func TestLoadCertificate_Inline(t *testing.T) {
	certPEM, keyPEM := generateTestCertAndKey(t)

	tests := []struct {
		name          string
		config        *models.CertificateConfig
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid inline certificate",
			config: &models.CertificateConfig{
				CertPEM: string(certPEM),
				KeyPEM:  string(keyPEM),
			},
			expectError: false,
		},
		{
			name: "Missing certificate PEM",
			config: &models.CertificateConfig{
				KeyPEM: string(keyPEM),
			},
			expectError:   true,
			errorContains: "inline certificate PEM is required",
		},
		{
			name: "Missing key PEM",
			config: &models.CertificateConfig{
				CertPEM: string(certPEM),
			},
			expectError:   true,
			errorContains: "inline private key PEM is required",
		},
		{
			name: "Invalid certificate PEM",
			config: &models.CertificateConfig{
				CertPEM: "invalid-pem-data",
				KeyPEM:  string(keyPEM),
			},
			expectError:   true,
			errorContains: "failed to parse inline certificate and key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewCertificateLoader(nil)
			loaded, err := loader.LoadCertificate(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, loaded)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, loaded)
				assert.Equal(t, models.CertSourceInline, loaded.Source)
				assert.NotEmpty(t, loaded.Certificate.Certificate)
			}
		})
	}
}

// Tests for LoadCertificate with file source
func TestLoadCertificate_File(t *testing.T) {
	certPEM, keyPEM := generateTestCertAndKey(t)
	certFile := writeToTempFile(t, certPEM)
	keyFile := writeToTempFile(t, keyPEM)

	tests := []struct {
		name          string
		config        *models.CertificateConfig
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid file paths",
			config: &models.CertificateConfig{
				CertFile: certFile,
				KeyFile:  keyFile,
			},
			expectError: false,
		},
		{
			name: "Missing certificate file",
			config: &models.CertificateConfig{
				KeyFile: keyFile,
			},
			expectError:   true,
			errorContains: "certificate file path is required",
		},
		{
			name: "Missing key file",
			config: &models.CertificateConfig{
				CertFile: certFile,
			},
			expectError:   true,
			errorContains: "private key file path is required",
		},
		{
			name: "Non-existent certificate file",
			config: &models.CertificateConfig{
				CertFile: "/nonexistent/cert.pem",
				KeyFile:  keyFile,
			},
			expectError:   true,
			errorContains: "failed to load certificate from files",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewCertificateLoader(nil)
			loaded, err := loader.LoadCertificate(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, loaded)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, loaded)
				assert.Equal(t, models.CertSourceFile, loaded.Source)
				assert.NotEmpty(t, loaded.Certificate.Certificate)
			}
		})
	}
}

// Tests for LoadCertificate with vault source
func TestLoadCertificate_Vault(t *testing.T) {
	certPEM, keyPEM := generateTestCertAndKey(t)
	combinedPEM := append(certPEM, keyPEM...)

	tests := []struct {
		name          string
		vault         *mockVault
		config        *models.CertificateConfig
		expectError   bool
		errorContains string
	}{
		{
			name: "Valid vault secret",
			vault: &mockVault{
				secrets: map[string][]byte{
					"my-cert-secret": combinedPEM,
				},
			},
			config: &models.CertificateConfig{
				CertKeySecret: "my-cert-secret",
			},
			expectError: false,
		},
		{
			name:  "Vault unavailable",
			vault: nil,
			config: &models.CertificateConfig{
				CertKeySecret: "my-cert-secret",
			},
			expectError:   true,
			errorContains: "vault service is not available",
		},
		{
			name: "Vault error retrieving secret",
			vault: &mockVault{
				err: fmt.Errorf("vault connection failed"),
			},
			config: &models.CertificateConfig{
				CertKeySecret: "my-cert-secret",
			},
			expectError:   true,
			errorContains: "failed to retrieve vault secret",
		},
		{
			name: "Secret not found",
			vault: &mockVault{
				secrets: map[string][]byte{},
			},
			config: &models.CertificateConfig{
				CertKeySecret: "nonexistent-secret",
			},
			expectError:   true,
			errorContains: "failed to retrieve vault secret",
		},
		{
			name: "Invalid PEM in secret",
			vault: &mockVault{
				secrets: map[string][]byte{
					"my-cert-secret": []byte("invalid-pem-data"),
				},
			},
			config: &models.CertificateConfig{
				CertKeySecret: "my-cert-secret",
			},
			expectError:   true,
			errorContains: "failed to parse combined PEM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewCertificateLoader(tt.vault)
			loaded, err := loader.LoadCertificate(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				if tt.errorContains != "" {
					assert.Contains(t, err.Error(), tt.errorContains)
				}
				assert.Nil(t, loaded)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, loaded)
				assert.Equal(t, models.CertSourceVault, loaded.Source)
				assert.NotEmpty(t, loaded.Certificate.Certificate)
			}
		})
	}
}

// Tests for multiple sources configured (error case)
func TestLoadCertificate_MultipleSources(t *testing.T) {
	certPEM, keyPEM := generateTestCertAndKey(t)
	certFile := writeToTempFile(t, certPEM)
	keyFile := writeToTempFile(t, keyPEM)

	tests := []struct {
		name          string
		config        *models.CertificateConfig
		errorContains string
	}{
		{
			name: "Inline + File",
			config: &models.CertificateConfig{
				CertPEM:  string(certPEM),
				KeyPEM:   string(keyPEM),
				CertFile: certFile,
				KeyFile:  keyFile,
			},
			errorContains: "multiple certificate sources",
		},
		{
			name: "Inline + Vault",
			config: &models.CertificateConfig{
				CertPEM:       string(certPEM),
				KeyPEM:        string(keyPEM),
				CertKeySecret: "my-secret",
			},
			errorContains: "multiple certificate sources",
		},
		{
			name: "File + Vault",
			config: &models.CertificateConfig{
				CertFile:      certFile,
				KeyFile:       keyFile,
				CertKeySecret: "my-secret",
			},
			errorContains: "multiple certificate sources",
		},
		{
			name: "All three sources",
			config: &models.CertificateConfig{
				CertPEM:       string(certPEM),
				KeyPEM:        string(keyPEM),
				CertFile:      certFile,
				KeyFile:       keyFile,
				CertKeySecret: "my-secret",
			},
			errorContains: "multiple certificate sources",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewCertificateLoader(nil)
			loaded, err := loader.LoadCertificate(tt.config)

			assert.Error(t, err)
			assert.Contains(t, err.Error(), tt.errorContains)
			assert.Nil(t, loaded)
		})
	}
}

// Tests for CA certificate loading
func TestLoadCertificate_WithCA(t *testing.T) {
	certPEM, keyPEM := generateTestCertAndKey(t)
	caPEM, _ := generateTestCertAndKey(t) // Use cert as CA

	caFile := writeToTempFile(t, caPEM)

	vault := &mockVault{
		secrets: map[string][]byte{
			"ca-secret": caPEM,
		},
	}

	tests := []struct {
		name        string
		config      *models.CertificateConfig
		vault       *mockVault
		expectError bool
	}{
		{
			name: "Inline cert with inline CA",
			config: &models.CertificateConfig{
				CertPEM: string(certPEM),
				KeyPEM:  string(keyPEM),
				CAPEM:   string(caPEM),
			},
			expectError: false,
		},
		{
			name: "Inline cert with CA file",
			config: &models.CertificateConfig{
				CertPEM: string(certPEM),
				KeyPEM:  string(keyPEM),
				CAFile:  caFile,
			},
			expectError: false,
		},
		{
			name: "Inline cert with CA vault secret",
			config: &models.CertificateConfig{
				CertPEM:  string(certPEM),
				KeyPEM:   string(keyPEM),
				CASecret: "ca-secret",
			},
			vault:       vault,
			expectError: false,
		},
		{
			name: "Multiple CA sources",
			config: &models.CertificateConfig{
				CertPEM:  string(certPEM),
				KeyPEM:   string(keyPEM),
				CAPEM:    string(caPEM),
				CAFile:   caFile,
				CASecret: "ca-secret",
			},
			vault:       vault,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loader := NewCertificateLoader(tt.vault)
			loaded, err := loader.LoadCertificate(tt.config)

			if tt.expectError {
				assert.Error(t, err)
				assert.Nil(t, loaded)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, loaded)
				assert.NotNil(t, loaded.CAPool, "CA pool should be set")
			}
		})
	}
}

// Tests for no certificate source configured
func TestLoadCertificate_NoSource(t *testing.T) {
	loader := NewCertificateLoader(nil)

	tests := []struct {
		name   string
		config *models.CertificateConfig
	}{
		{
			name:   "Nil config",
			config: nil,
		},
		{
			name:   "Empty config",
			config: &models.CertificateConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			loaded, err := loader.LoadCertificate(tt.config)

			assert.Error(t, err)
			assert.Nil(t, loaded)
		})
	}
}

// Test ValidateCertificate function
func TestValidateCertificate(t *testing.T) {
	certPEM, _ := generateTestCertAndKey(t)

	tests := []struct {
		name        string
		input       []byte
		expectError bool
	}{
		{
			name:        "Valid certificate",
			input:       certPEM,
			expectError: false,
		},
		{
			name:        "Invalid PEM",
			input:       []byte("not-a-pem"),
			expectError: true,
		},
		{
			name:        "Empty input",
			input:       []byte{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateCertificate(tt.input)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
