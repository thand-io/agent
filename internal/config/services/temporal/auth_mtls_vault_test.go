package temporal

import (
	"crypto/tls"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// MockVault is a mock implementation of VaultImpl interface
type MockVault struct {
	mock.Mock
}

func (m *MockVault) Initialize() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockVault) Shutdown() error {
	args := m.Called()
	return args.Error(0)
}

func (m *MockVault) GetSecret(key string) ([]byte, error) {
	args := m.Called(key)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]byte), args.Error(1)
}

func (m *MockVault) StoreSecret(key string, value []byte) error {
	args := m.Called(key, value)
	return args.Error(0)
}

func TestConfigureMTLSVault_PEM_Combined(t *testing.T) {
	// Load real combined PEM certificate (cert + key)
	combinedPEM, err := os.ReadFile("test_client_combined.pem")
	require.NoError(t, err, "failed to read combined PEM certificate")

	mockVault := new(MockVault)
	mockVault.On("GetSecret", "temporal-mtls-cert").Return(combinedPEM, nil)

	config := &models.TemporalConfig{
		TemporalAuthMTLSVault: models.TemporalAuthMTLSVault{
			MtlsVaultName: "temporal-mtls-cert",
			MtlsVaultType: "pem",
		},
	}

	temporalClient := &TemporalClient{
		config:   config,
		vault:    mockVault,
		identity: "test-identity",
	}

	tlsConfig, err := temporalClient.configureMTLSVault()

	require.NoError(t, err)
	require.NotNil(t, tlsConfig)
	assert.Equal(t, uint16(tls.VersionTLS12), tlsConfig.MinVersion)
	assert.Len(t, tlsConfig.Certificates, 1)

	mockVault.AssertExpectations(t)
}

func TestConfigureMTLSVault_PEM_Separate(t *testing.T) {
	// Load real PEM certificate (cert only)
	certPEM, err := os.ReadFile("test_client.pem")
	require.NoError(t, err, "failed to read PEM certificate")

	mockVault := new(MockVault)
	mockVault.On("GetSecret", "temporal-mtls-cert").Return(certPEM, nil)

	config := &models.TemporalConfig{
		TemporalAuthMTLSVault: models.TemporalAuthMTLSVault{
			MtlsVaultName: "temporal-mtls-cert",
			MtlsVaultType: "pem",
		},
	}

	temporalClient := &TemporalClient{
		config:   config,
		vault:    mockVault,
		identity: "test-identity",
	}

	// This should fail because we only have the certificate, not the key
	_, err = temporalClient.configureMTLSVault()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no private key found")

	mockVault.AssertExpectations(t)
}

func TestConfigureMTLSVault_PKCS12_Unencrypted(t *testing.T) {
	// Load real unencrypted PKCS12 certificate
	p12Data, err := os.ReadFile("test_client.p12")
	require.NoError(t, err, "failed to read PKCS12 certificate")

	mockVault := new(MockVault)
	mockVault.On("GetSecret", "temporal-mtls-cert").Return(p12Data, nil)

	config := &models.TemporalConfig{
		TemporalAuthMTLSVault: models.TemporalAuthMTLSVault{
			MtlsVaultName: "temporal-mtls-cert",
			MtlsVaultType: "pkcs12",
		},
	}

	temporalClient := &TemporalClient{
		config:   config,
		vault:    mockVault,
		identity: "test-identity",
	}

	tlsConfig, err := temporalClient.configureMTLSVault()

	require.NoError(t, err)
	require.NotNil(t, tlsConfig)
	assert.Equal(t, uint16(tls.VersionTLS12), tlsConfig.MinVersion)
	assert.Len(t, tlsConfig.Certificates, 1)

	mockVault.AssertExpectations(t)
}

func TestConfigureMTLSVault_PKCS12_Encrypted(t *testing.T) {
	// Load real encrypted PKCS12 certificate (password: "test-password")
	p12Data, err := os.ReadFile("test_client_encrypted.p12")
	require.NoError(t, err, "failed to read encrypted PKCS12 certificate")

	mockVault := new(MockVault)
	mockVault.On("GetSecret", "temporal-mtls-cert").Return(p12Data, nil)

	config := &models.TemporalConfig{
		TemporalAuthMTLSVault: models.TemporalAuthMTLSVault{
			MtlsVaultName:     "temporal-mtls-cert",
			MtlsVaultType:     "pkcs12",
			MtlsVaultPassword: "test-password",
		},
	}

	temporalClient := &TemporalClient{
		config:   config,
		vault:    mockVault,
		identity: "test-identity",
	}

	tlsConfig, err := temporalClient.configureMTLSVault()

	require.NoError(t, err)
	require.NotNil(t, tlsConfig)
	assert.Equal(t, uint16(tls.VersionTLS12), tlsConfig.MinVersion)
	assert.Len(t, tlsConfig.Certificates, 1)

	mockVault.AssertExpectations(t)
}

func TestConfigureMTLSVault_PKCS12_WrongPassword(t *testing.T) {
	// Load real encrypted PKCS12 certificate (password: "test-password")
	p12Data, err := os.ReadFile("test_client_encrypted.p12")
	require.NoError(t, err, "failed to read encrypted PKCS12 certificate")

	mockVault := new(MockVault)
	mockVault.On("GetSecret", "temporal-mtls-cert").Return(p12Data, nil)

	config := &models.TemporalConfig{
		TemporalAuthMTLSVault: models.TemporalAuthMTLSVault{
			MtlsVaultName:     "temporal-mtls-cert",
			MtlsVaultType:     "pkcs12",
			MtlsVaultPassword: "wrong-password",
		},
	}

	temporalClient := &TemporalClient{
		config:   config,
		vault:    mockVault,
		identity: "test-identity",
	}

	_, err = temporalClient.configureMTLSVault()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to decode PKCS12")

	mockVault.AssertExpectations(t)
}

func TestConfigureMTLSVault_AutoDetect_PEM(t *testing.T) {
	// Load real combined PEM certificate
	combinedPEM, err := os.ReadFile("test_client_combined.pem")
	require.NoError(t, err, "failed to read combined PEM certificate")

	mockVault := new(MockVault)
	mockVault.On("GetSecret", "temporal-mtls-cert").Return(combinedPEM, nil)

	config := &models.TemporalConfig{
		TemporalAuthMTLSVault: models.TemporalAuthMTLSVault{
			MtlsVaultName: "temporal-mtls-cert",
			// No MtlsVaultType specified - should auto-detect as PEM
		},
	}

	temporalClient := &TemporalClient{
		config:   config,
		vault:    mockVault,
		identity: "test-identity",
	}

	tlsConfig, err := temporalClient.configureMTLSVault()

	require.NoError(t, err)
	require.NotNil(t, tlsConfig)
	assert.Equal(t, uint16(tls.VersionTLS12), tlsConfig.MinVersion)
	assert.Len(t, tlsConfig.Certificates, 1)

	mockVault.AssertExpectations(t)
}

func TestConfigureMTLSVault_AutoDetect_PKCS12(t *testing.T) {
	// Load real unencrypted PKCS12 certificate
	p12Data, err := os.ReadFile("test_client.p12")
	require.NoError(t, err, "failed to read PKCS12 certificate")

	mockVault := new(MockVault)
	mockVault.On("GetSecret", "temporal-mtls-cert").Return(p12Data, nil)

	config := &models.TemporalConfig{
		TemporalAuthMTLSVault: models.TemporalAuthMTLSVault{
			MtlsVaultName: "temporal-mtls-cert",
			// No MtlsVaultType specified - should auto-detect as PKCS12
		},
	}

	temporalClient := &TemporalClient{
		config:   config,
		vault:    mockVault,
		identity: "test-identity",
	}

	tlsConfig, err := temporalClient.configureMTLSVault()

	require.NoError(t, err)
	require.NotNil(t, tlsConfig)
	assert.Equal(t, uint16(tls.VersionTLS12), tlsConfig.MinVersion)
	assert.Len(t, tlsConfig.Certificates, 1)

	mockVault.AssertExpectations(t)
}

func TestConfigureMTLSVault_VaultError(t *testing.T) {
	mockVault := new(MockVault)
	mockVault.On("GetSecret", "temporal-mtls-cert").Return(nil, assert.AnError)

	config := &models.TemporalConfig{
		TemporalAuthMTLSVault: models.TemporalAuthMTLSVault{
			MtlsVaultName: "temporal-mtls-cert",
			MtlsVaultType: "pem",
		},
	}

	temporalClient := &TemporalClient{
		config:   config,
		vault:    mockVault,
		identity: "test-identity",
	}

	_, err := temporalClient.configureMTLSVault()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to read certificate from vault")

	mockVault.AssertExpectations(t)
}

func TestConfigureMTLSVault_InvalidFormat(t *testing.T) {
	invalidData := []byte("this is not a valid certificate")

	mockVault := new(MockVault)
	mockVault.On("GetSecret", "temporal-mtls-cert").Return(invalidData, nil)

	config := &models.TemporalConfig{
		TemporalAuthMTLSVault: models.TemporalAuthMTLSVault{
			MtlsVaultName: "temporal-mtls-cert",
			// No type specified - should try to auto-detect and fail
		},
	}

	temporalClient := &TemporalClient{
		config:   config,
		vault:    mockVault,
		identity: "test-identity",
	}

	_, err := temporalClient.configureMTLSVault()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to")

	mockVault.AssertExpectations(t)
}

func TestHasMTLSVault(t *testing.T) {
	tests := []struct {
		name       string
		vaultName  string
		vaultIsNil bool
		want       bool
	}{
		{
			name:       "has vault configuration",
			vaultName:  "temporal-mtls-cert",
			vaultIsNil: false,
			want:       true,
		},
		{
			name:       "vault config is nil",
			vaultName:  "",
			vaultIsNil: true,
			want:       false,
		},
		{
			name:       "vault name is empty",
			vaultName:  "",
			vaultIsNil: false,
			want:       false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &models.TemporalConfig{}
			
			if !tt.vaultIsNil {
				config.MtlsVaultName = tt.vaultName
			}

			temporalClient := &TemporalClient{
				config: config,
			}

			got := temporalClient.hasMTLSVault()
			assert.Equal(t, tt.want, got)
		})
	}
}
