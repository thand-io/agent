package models_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thand-io/agent/internal/models"
)

func TestCORSConfig_WithDefaults(t *testing.T) {
	tests := []struct {
		name     string
		config   models.CORSConfig
		expected models.CORSConfig
	}{
		{
			name:   "empty config gets all defaults",
			config: models.CORSConfig{},
			expected: models.CORSConfig{
				AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
				AllowedHeaders: []string{
					"Origin",
					"Content-Length",
					"Content-Type",
					"Authorization",
					"Accept",
					"X-Requested-With",
				},
				MaxAge: 86400,
			},
		},
		{
			name: "existing AllowedMethods preserved",
			config: models.CORSConfig{
				AllowedMethods: []string{"GET", "POST"},
			},
			expected: models.CORSConfig{
				AllowedMethods: []string{"GET", "POST"},
				AllowedHeaders: []string{
					"Origin",
					"Content-Length",
					"Content-Type",
					"Authorization",
					"Accept",
					"X-Requested-With",
				},
				MaxAge: 86400,
			},
		},
		{
			name: "existing AllowedHeaders preserved",
			config: models.CORSConfig{
				AllowedHeaders: []string{"Content-Type", "X-Custom-Header"},
			},
			expected: models.CORSConfig{
				AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
				AllowedHeaders: []string{"Content-Type", "X-Custom-Header"},
				MaxAge:         86400,
			},
		},
		{
			name: "existing MaxAge preserved",
			config: models.CORSConfig{
				MaxAge: 3600,
			},
			expected: models.CORSConfig{
				AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
				AllowedHeaders: []string{
					"Origin",
					"Content-Length",
					"Content-Type",
					"Authorization",
					"Accept",
					"X-Requested-With",
				},
				MaxAge: 3600,
			},
		},
		{
			name: "all existing values preserved",
			config: models.CORSConfig{
				AllowedOrigins:   []string{"https://example.com"},
				AllowedMethods:   []string{"GET"},
				AllowedHeaders:   []string{"Authorization"},
				ExposeHeaders:    []string{"X-Request-Id"},
				AllowCredentials: true,
				MaxAge:           7200,
			},
			expected: models.CORSConfig{
				AllowedOrigins:   []string{"https://example.com"},
				AllowedMethods:   []string{"GET"},
				AllowedHeaders:   []string{"Authorization"},
				ExposeHeaders:    []string{"X-Request-Id"},
				AllowCredentials: true,
				MaxAge:           7200,
			},
		},
		{
			name: "AllowCredentials and ExposeHeaders not affected by defaults",
			config: models.CORSConfig{
				AllowCredentials: true,
				ExposeHeaders:    []string{"X-Custom-Header"},
			},
			expected: models.CORSConfig{
				AllowedMethods: []string{"GET", "POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"},
				AllowedHeaders: []string{
					"Origin",
					"Content-Length",
					"Content-Type",
					"Authorization",
					"Accept",
					"X-Requested-With",
				},
				ExposeHeaders:    []string{"X-Custom-Header"},
				AllowCredentials: true,
				MaxAge:           86400,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.config.WithDefaults()

			assert.Equal(t, tt.expected.AllowedOrigins, result.AllowedOrigins)
			assert.Equal(t, tt.expected.AllowedMethods, result.AllowedMethods)
			assert.Equal(t, tt.expected.AllowedHeaders, result.AllowedHeaders)
			assert.Equal(t, tt.expected.ExposeHeaders, result.ExposeHeaders)
			assert.Equal(t, tt.expected.AllowCredentials, result.AllowCredentials)
			assert.Equal(t, tt.expected.MaxAge, result.MaxAge)
		})
	}
}

func TestCORSConfig_WithDefaults_DoesNotModifyOriginal(t *testing.T) {
	original := models.CORSConfig{
		AllowedOrigins: []string{"https://example.com"},
	}

	result := original.WithDefaults()

	// Verify original is unchanged
	assert.Nil(t, original.AllowedMethods)
	assert.Nil(t, original.AllowedHeaders)
	assert.Equal(t, 0, original.MaxAge)

	// Verify result has defaults applied
	assert.NotNil(t, result.AllowedMethods)
	assert.NotNil(t, result.AllowedHeaders)
	assert.Equal(t, 86400, result.MaxAge)
}

func TestCORSConfig_AddOrigins(t *testing.T) {
	tests := []struct {
		name            string
		initialOrigins  []string
		originsToAdd    []string
		expectedOrigins []string
	}{
		{
			name:            "add to empty list",
			initialOrigins:  nil,
			originsToAdd:    []string{"https://example.com"},
			expectedOrigins: []string{"https://example.com"},
		},
		{
			name:            "add single origin to existing list",
			initialOrigins:  []string{"https://example.com"},
			originsToAdd:    []string{"https://other.com"},
			expectedOrigins: []string{"https://example.com", "https://other.com"},
		},
		{
			name:            "add multiple origins to existing list",
			initialOrigins:  []string{"https://example.com"},
			originsToAdd:    []string{"https://other.com", "https://third.com"},
			expectedOrigins: []string{"https://example.com", "https://other.com", "https://third.com"},
		},
		{
			name:            "add no origins",
			initialOrigins:  []string{"https://example.com"},
			originsToAdd:    []string{},
			expectedOrigins: []string{"https://example.com"},
		},
		{
			name:            "add wildcard origin",
			initialOrigins:  []string{"https://example.com"},
			originsToAdd:    []string{"https://*.app.thand.io"},
			expectedOrigins: []string{"https://example.com", "https://*.app.thand.io"},
		},
		{
			name:            "add duplicate origin (allowed by implementation)",
			initialOrigins:  []string{"https://example.com"},
			originsToAdd:    []string{"https://example.com"},
			expectedOrigins: []string{"https://example.com", "https://example.com"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			config := models.CORSConfig{
				AllowedOrigins: tt.initialOrigins,
			}

			config.AddOrigins(tt.originsToAdd...)

			assert.Equal(t, tt.expectedOrigins, config.AllowedOrigins)
		})
	}
}

func TestCORSConfig_AddOrigins_ModifiesInPlace(t *testing.T) {
	config := models.CORSConfig{
		AllowedOrigins: []string{"https://example.com"},
	}

	config.AddOrigins("https://other.com")

	// Verify the config itself was modified
	assert.Equal(t, []string{"https://example.com", "https://other.com"}, config.AllowedOrigins)
}
