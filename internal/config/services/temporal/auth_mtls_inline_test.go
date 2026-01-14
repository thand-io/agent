package temporal

import (
	"crypto/tls"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// loadTestCertificates loads real test certificates from disk
func loadTestCertificates(t *testing.T) (certPEM, keyPEM string) {
	t.Helper()

	// Load certificate
	certData, err := os.ReadFile("test_client.pem")
	require.NoError(t, err, "failed to read test certificate")

	// Load key
	keyData, err := os.ReadFile("test_client.key")
	require.NoError(t, err, "failed to read test key")

	return string(certData), string(keyData)
}

func TestConfigureMTLSInline(t *testing.T) {
	// Load real test certificates
	testCertPEM, testKeyPEM := loadTestCertificates(t)
	tests := []struct {
		name      string
		certPEM   string
		keyPEM    string
		wantError bool
	}{
		{
			name:      "valid inline certificates",
			certPEM:   testCertPEM,
			keyPEM:    testKeyPEM,
			wantError: false,
		},
		{
			name:      "invalid certificate format",
			certPEM:   "invalid-cert",
			keyPEM:    testKeyPEM,
			wantError: true,
		},
		{
			name:      "invalid key format",
			certPEM:   testCertPEM,
			keyPEM:    "invalid-key",
			wantError: true,
		},
		{
			name:      "empty certificate",
			certPEM:   "",
			keyPEM:    testKeyPEM,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &models.TemporalConfig{}
			config.MtlsCert = tt.certPEM
			config.MtlsKey = tt.keyPEM

			temporalClient := &TemporalClient{
				config:   config,
				identity: "test-identity",
			}

			tlsConfig, err := temporalClient.configureMTLSInline()

			if tt.wantError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, tlsConfig)
			assert.Equal(t, uint16(tls.VersionTLS12), tlsConfig.MinVersion)
			assert.Len(t, tlsConfig.Certificates, 1)
		})
	}
}

func TestHasMTLSInline(t *testing.T) {
	tests := []struct {
		name string
		cert string
		key  string
		want bool
	}{
		{
			name: "has both cert and key",
			cert: "cert-data",
			key:  "key-data",
			want: true,
		},
		{
			name: "missing key",
			cert: "cert-data",
			key:  "",
			want: false,
		},
		{
			name: "missing cert",
			cert: "",
			key:  "key-data",
			want: false,
		},
		{
			name: "both empty",
			cert: "",
			key:  "",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &models.TemporalConfig{}
			config.MtlsCert = tt.cert
			config.MtlsKey = tt.key

			temporalClient := &TemporalClient{
				config: config,
			}

			got := temporalClient.hasMTLSInline()
			assert.Equal(t, tt.want, got)
		})
	}
}
