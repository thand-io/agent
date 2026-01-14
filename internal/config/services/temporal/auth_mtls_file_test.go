package temporal

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

func TestConfigureMTLSFile(t *testing.T) {
	// Use real test certificate files
	certFile := "testdata/test_client.pem"
	keyFile := "testdata/test_client.key"

	// Verify test files exist
	_, err := os.Stat(certFile)
	require.NoError(t, err, "test certificate file not found")
	_, err = os.Stat(keyFile)
	require.NoError(t, err, "test key file not found")

	// Create temporary directory for invalid test files
	tmpDir := t.TempDir()

	tests := []struct {
		name      string
		certFile  string
		keyFile   string
		wantError bool
		setupFunc func() // Optional setup function for special cases
	}{
		{
			name:      "valid certificate files",
			certFile:  certFile,
			keyFile:   keyFile,
			wantError: false,
		},
		{
			name:      "non-existent certificate file",
			certFile:  "nonexistent-cert.pem",
			keyFile:   keyFile,
			wantError: true,
		},
		{
			name:      "non-existent key file",
			certFile:  certFile,
			keyFile:   "nonexistent-key.pem",
			wantError: true,
		},
		{
			name:      "invalid certificate content",
			certFile:  filepath.Join(tmpDir, "invalid-cert.pem"),
			keyFile:   keyFile,
			wantError: true,
			setupFunc: func() {
				err := os.WriteFile(filepath.Join(tmpDir, "invalid-cert.pem"), []byte("invalid-cert-data"), 0600)
				require.NoError(t, err)
			},
		},
		{
			name:      "invalid key content",
			certFile:  certFile,
			keyFile:   filepath.Join(tmpDir, "invalid-key.pem"),
			wantError: true,
			setupFunc: func() {
				err := os.WriteFile(filepath.Join(tmpDir, "invalid-key.pem"), []byte("invalid-key-data"), 0600)
				require.NoError(t, err)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupFunc != nil {
				tt.setupFunc()
			}

			config := &models.TemporalConfig{}
			config.MtlsCertFile = tt.certFile
			config.MtlsKeyFile = tt.keyFile

			temporalClient := &TemporalClient{
				config:   config,
				identity: "test-identity",
			}

			tlsConfig, err := temporalClient.configureMTLSFile()

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

func TestHasMTLSFile(t *testing.T) {
	tests := []struct {
		name     string
		certFile string
		keyFile  string
		want     bool
	}{
		{
			name:     "has both cert and key files",
			certFile: "/path/to/cert.pem",
			keyFile:  "/path/to/key.pem",
			want:     true,
		},
		{
			name:     "missing key file",
			certFile: "/path/to/cert.pem",
			keyFile:  "",
			want:     false,
		},
		{
			name:     "missing cert file",
			certFile: "",
			keyFile:  "/path/to/key.pem",
			want:     false,
		},
		{
			name:     "both empty",
			certFile: "",
			keyFile:  "",
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &models.TemporalConfig{}
			config.MtlsCertFile = tt.certFile
			config.MtlsKeyFile = tt.keyFile

			temporalClient := &TemporalClient{
				config: config,
			}

			got := temporalClient.hasMTLSFile()
			assert.Equal(t, tt.want, got)
		})
	}
}
