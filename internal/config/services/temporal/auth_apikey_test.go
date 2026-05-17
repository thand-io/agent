package temporal

import (
	"crypto/tls"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/client"
)

func TestConfigureAPIKeyAuth(t *testing.T) {
	tests := []struct {
		name      string
		apiKey    string
		wantError bool
	}{
		{
			name:      "valid API key",
			apiKey:    "test-api-key-12345",
			wantError: false,
		},
		{
			name:      "empty API key should not error",
			apiKey:    "",
			wantError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create temporal client with API key config
			config := &models.TemporalConfig{
				Host:      "localhost",
				Port:      7233,
				Namespace: "default",
			}
			config.ApiKey = tt.apiKey

			temporalClient := NewTemporalClient(
				config,
				nil,
				"test-identity",
			)

			// Configure client options
			options := &client.Options{}
			err := temporalClient.configureAPIKeyAuth(options)

			if tt.wantError {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)

			// If API key is provided, verify configuration
			if len(tt.apiKey) > 0 {
				assert.NotNil(t, options.ConnectionOptions.TLS)
				assert.Equal(t, uint16(tls.VersionTLS12), options.ConnectionOptions.TLS.MinVersion)
				assert.NotNil(t, options.Credentials)
			} else {
				// Empty API key should not configure anything
				assert.Nil(t, options.ConnectionOptions.TLS)
				assert.Nil(t, options.Credentials)
			}
		})
	}
}

func TestHasAPIKeyAuth(t *testing.T) {
	tests := []struct {
		name   string
		apiKey string
		want   bool
	}{
		{
			name:   "has API key",
			apiKey: "test-api-key",
			want:   true,
		},
		{
			name:   "empty API key",
			apiKey: "",
			want:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := &models.TemporalConfig{}
			config.ApiKey = tt.apiKey

			temporalClient := NewTemporalClient(
				config,
				nil,
				"test-identity",
			)

			got := temporalClient.hasAPIKeyAuth()
			assert.Equal(t, tt.want, got)
		})
	}
}
