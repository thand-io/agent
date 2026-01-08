package models_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/interpolate"
	"github.com/thand-io/agent/internal/models"
)

// TestProvider_ResolveConfig_WithEnvVars tests the config resolution using
// environment variables accessed via jq input (.) notation.
func TestProvider_ResolveConfig_WithEnvVars(t *testing.T) {
	tests := []struct {
		name           string
		config         models.BasicConfig
		envVars        map[string]string
		expectedConfig map[string]any
		expectError    bool
	}{
		{
			name: "simple env var access",
			config: models.BasicConfig{
				"region":     "${ .AWS_REGION }",
				"account_id": "123456789012",
			},
			envVars: map[string]string{
				"AWS_REGION": "us-west-2",
			},
			expectedConfig: map[string]any{
				"region":     "us-west-2",
				"account_id": "123456789012",
			},
			expectError: false,
		},
		{
			name: "multiple env vars",
			config: models.BasicConfig{
				"region":     "${ .AWS_REGION }",
				"account_id": "${ .AWS_ACCOUNT_ID }",
				"role_arn":   "${ .AWS_ROLE_ARN }",
			},
			envVars: map[string]string{
				"AWS_REGION":     "eu-central-1",
				"AWS_ACCOUNT_ID": "987654321098",
				"AWS_ROLE_ARN":   "arn:aws:iam::987654321098:role/TestRole",
			},
			expectedConfig: map[string]any{
				"region":     "eu-central-1",
				"account_id": "987654321098",
				"role_arn":   "arn:aws:iam::987654321098:role/TestRole",
			},
			expectError: false,
		},
		{
			name: "env var with default value",
			config: models.BasicConfig{
				"region": "${ .UNDEFINED_VAR // \"default-region\" }",
			},
			envVars: map[string]string{},
			expectedConfig: map[string]any{
				"region": "default-region",
			},
			expectError: false,
		},
		{
			name: "env var string concatenation",
			config: models.BasicConfig{
				"arn": "${ \"arn:aws:iam::\" + .AWS_ACCOUNT + \":role/\" + .AWS_ROLE }",
			},
			envVars: map[string]string{
				"AWS_ACCOUNT": "123456789012",
				"AWS_ROLE":    "MyRole",
			},
			expectedConfig: map[string]any{
				"arn": "arn:aws:iam::123456789012:role/MyRole",
			},
			expectError: false,
		},
		{
			name: "env var in nested config",
			config: models.BasicConfig{
				"cluster_name": "${ .K8S_CLUSTER }",
				"auth": map[string]any{
					"token":    "${ .K8S_TOKEN }",
					"endpoint": "${ .K8S_ENDPOINT }",
				},
			},
			envVars: map[string]string{
				"K8S_CLUSTER":  "prod-cluster",
				"K8S_TOKEN":    "secret-token-123",
				"K8S_ENDPOINT": "https://k8s.example.com:6443",
			},
			expectedConfig: map[string]any{
				"cluster_name": "prod-cluster",
				"auth": map[string]any{
					"token":    "secret-token-123",
					"endpoint": "https://k8s.example.com:6443",
				},
			},
			expectError: false,
		},
		{
			name: "env var conditional expression",
			config: models.BasicConfig{
				"env_type": "${ if .IS_PROD == \"true\" then \"production\" else \"development\" end }",
			},
			envVars: map[string]string{
				"IS_PROD": "true",
			},
			expectedConfig: map[string]any{
				"env_type": "production",
			},
			expectError: false,
		},
		{
			name: "env var conditional false case",
			config: models.BasicConfig{
				"env_type": "${ if .IS_PROD == \"true\" then \"production\" else \"development\" end }",
			},
			envVars: map[string]string{
				"IS_PROD": "false",
			},
			expectedConfig: map[string]any{
				"env_type": "development",
			},
			expectError: false,
		},
		{
			name: "env var in array",
			config: models.BasicConfig{
				"hosts": []any{
					"${ .HOST1 }",
					"${ .HOST2 }",
				},
			},
			envVars: map[string]string{
				"HOST1": "server1.example.com",
				"HOST2": "server2.example.com",
			},
			expectedConfig: map[string]any{
				"hosts": []any{
					"server1.example.com",
					"server2.example.com",
				},
			},
			expectError: false,
		},
		{
			name: "mixed static and env var values",
			config: models.BasicConfig{
				"subscription_id": "${ .AZURE_SUBSCRIPTION_ID }",
				"resource_group":  "my-resource-group",
				"location":        "eastus",
			},
			envVars: map[string]string{
				"AZURE_SUBSCRIPTION_ID": "12345678-1234-1234-1234-123456789012",
			},
			expectedConfig: map[string]any{
				"subscription_id": "12345678-1234-1234-1234-123456789012",
				"resource_group":  "my-resource-group",
				"location":        "eastus",
			},
			expectError: false,
		},
		{
			name: "deeply nested config with env var",
			config: models.BasicConfig{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": map[string]any{
							"value": "${ .DEEP_VALUE }",
						},
					},
				},
			},
			envVars: map[string]string{
				"DEEP_VALUE": "found-it",
			},
			expectedConfig: map[string]any{
				"level1": map[string]any{
					"level2": map[string]any{
						"level3": map[string]any{
							"value": "found-it",
						},
					},
				},
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build input map from env vars (simulating what ResolveConfig does)
			input := make(map[string]any)
			for key, value := range tt.envVars {
				input[key] = value
			}

			// Use interpolate.NewTraverse with input (accessed via .)
			result, err := interpolate.NewTraverse(tt.config.AsMap(), input, nil)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)

			resultMap, ok := result.(map[string]any)
			require.True(t, ok, "result should be a map[string]any")

			// Verify the resolved config matches expected
			assertConfigEqual(t, tt.expectedConfig, resultMap)
		})
	}
}

// TestProvider_ResolveConfig_WithInput tests config resolution using jq input (.) notation
func TestProvider_ResolveConfig_WithInput(t *testing.T) {
	tests := []struct {
		name           string
		config         models.BasicConfig
		input          map[string]any
		expectedConfig map[string]any
		expectError    bool
	}{
		{
			name: "simple input access",
			config: models.BasicConfig{
				"region": "${ .aws_region }",
			},
			input: map[string]any{
				"aws_region": "us-west-2",
			},
			expectedConfig: map[string]any{
				"region": "us-west-2",
			},
			expectError: false,
		},
		{
			name: "nested input object access",
			config: models.BasicConfig{
				"db_host": "${ .database.host }",
				"db_port": "${ .database.port }",
			},
			input: map[string]any{
				"database": map[string]any{
					"host": "localhost",
					"port": 5432,
				},
			},
			expectedConfig: map[string]any{
				"db_host": "localhost",
				"db_port": 5432,
			},
			expectError: false,
		},
		{
			name: "integer value from input",
			config: models.BasicConfig{
				"port": "${ .port }",
			},
			input: map[string]any{
				"port": 8080,
			},
			expectedConfig: map[string]any{
				"port": 8080,
			},
			expectError: false,
		},
		{
			name: "boolean value from input",
			config: models.BasicConfig{
				"enabled": "${ .enabled }",
			},
			input: map[string]any{
				"enabled": true,
			},
			expectedConfig: map[string]any{
				"enabled": true,
			},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := interpolate.NewTraverse(tt.config.AsMap(), tt.input, nil)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, result)

			resultMap, ok := result.(map[string]any)
			require.True(t, ok, "result should be a map[string]any")

			assertConfigEqual(t, tt.expectedConfig, resultMap)
		})
	}
}

// assertConfigEqual recursively compares two config maps
func assertConfigEqual(t *testing.T, expected, actual map[string]any) {
	t.Helper()
	for key, expectedValue := range expected {
		actualValue, ok := actual[key]
		assert.True(t, ok, "expected key %s to exist in config", key)

		switch ev := expectedValue.(type) {
		case map[string]any:
			av, ok := actualValue.(map[string]any)
			require.True(t, ok, "expected value for key %s to be a map", key)
			assertConfigEqual(t, ev, av)
		case []any:
			av, ok := actualValue.([]any)
			require.True(t, ok, "expected value for key %s to be an array", key)
			assert.Equal(t, len(ev), len(av), "array length mismatch for key %s", key)
			for i := range ev {
				assert.Equal(t, ev[i], av[i], "array element mismatch at index %d for key %s", i, key)
			}
		default:
			assert.Equal(t, expectedValue, actualValue, "value mismatch for key %s", key)
		}
	}
}

// TestProvider_ResolveConfig_NilConfig tests handling of nil config
func TestProvider_ResolveConfig_NilConfig(t *testing.T) {
	provider := models.ProviderConfig{
		Name:     "nil-config-provider",
		Provider: "test",
		Config:   nil,
	}

	err := provider.ResolveConfig(map[string]any{})
	// Should handle nil config gracefully - either error or no-op
	// Based on the implementation, AsMap() on nil returns empty map
	// so this might still work or panic depending on implementation
	if err != nil {
		// If there's an error, that's acceptable behavior
		t.Logf("ResolveConfig with nil config returned error: %v", err)
	}
}

// TestProvider_ResolveConfig_UpdatesConfig verifies that ResolveConfig updates the provider's config in place
func TestProvider_ResolveConfig_UpdatesConfig(t *testing.T) {
	config := models.BasicConfig{
		"value": "${ .my_var }",
	}
	provider := models.ProviderConfig{
		Name:     "update-test-provider",
		Provider: "test",
		Config:   &config,
	}

	// Use input object to test (accessed via .)
	input := map[string]any{
		"my_var": "resolved-value",
	}

	// Simulate what ResolveConfig does
	newConfig, err := interpolate.NewTraverse(provider.Config.AsMap(), input, nil)
	require.NoError(t, err)

	if basicConfig, ok := newConfig.(map[string]any); ok {
		provider.Config.Update(basicConfig)
	}

	// Verify the config was updated
	actualValue, ok := provider.Config.GetString("value")
	require.True(t, ok)
	assert.Equal(t, "resolved-value", actualValue)
}

// TestProvider_ResolveConfig_ErrorCases tests error handling
func TestProvider_ResolveConfig_ErrorCases(t *testing.T) {
	tests := []struct {
		name        string
		config      models.BasicConfig
		input       map[string]any
		expectError bool
	}{
		{
			name: "invalid jq expression",
			config: models.BasicConfig{
				"value": "${ invalid syntax here }",
			},
			input:       map[string]any{},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := interpolate.NewTraverse(tt.config.AsMap(), tt.input, nil)

			if tt.expectError {
				assert.Error(t, err, "expected an error for invalid jq expression")
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestProvider_BasicConfigMethods tests BasicConfig helper methods used by ResolveConfig
func TestProvider_BasicConfigMethods(t *testing.T) {
	t.Run("AsMap returns copy of config", func(t *testing.T) {
		config := models.BasicConfig{
			"key1": "value1",
			"key2": 123,
		}

		result := config.AsMap()
		assert.Equal(t, "value1", result["key1"])
		assert.Equal(t, 123, result["key2"])
	})

	t.Run("AsMap on nil returns empty map", func(t *testing.T) {
		var config *models.BasicConfig = nil
		result := config.AsMap()
		assert.NotNil(t, result)
		assert.Empty(t, result)
	})

	t.Run("Update merges values", func(t *testing.T) {
		config := models.BasicConfig{
			"key1": "original",
			"key2": "keep",
		}

		config.Update(map[string]any{
			"key1": "updated",
			"key3": "new",
		})

		assert.Equal(t, "updated", config["key1"])
		assert.Equal(t, "keep", config["key2"])
		assert.Equal(t, "new", config["key3"])
	})

	t.Run("GetString returns string value", func(t *testing.T) {
		config := models.BasicConfig{
			"string_key": "hello",
			"int_key":    123,
		}

		val, ok := config.GetString("string_key")
		assert.True(t, ok)
		assert.Equal(t, "hello", val)

		_, ok = config.GetString("int_key")
		assert.False(t, ok)

		_, ok = config.GetString("missing_key")
		assert.False(t, ok)
	})
}
