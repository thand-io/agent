package models

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestProviderDefinitions_UnmarshalJSON tests the JSON unmarshaling with various version formats
func TestProviderDefinitions_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name          string
		jsonInput     string
		expectError   bool
		expectedVer   string
		providerCount int
		validateFunc  func(t *testing.T, def *ProviderDefinitions)
	}{
		{
			name: "string version with providers",
			jsonInput: `{
				"version": "1.0",
				"providers": {
					"aws": {
						"provider": "aws",
						"name": "AWS Provider",
						"description": "Amazon Web Services provider",
						"enabled": true
					},
					"gcp": {
						"provider": "gcp",
						"name": "GCP Provider",
						"description": "Google Cloud Platform provider",
						"enabled": true
					}
				}
			}`,
			expectError:   false,
			expectedVer:   "1.0.0",
			providerCount: 2,
			validateFunc: func(t *testing.T, def *ProviderDefinitions) {
				aws, exists := def.Providers["aws"]
				assert.True(t, exists)
				assert.Equal(t, "aws", aws.Provider)
				assert.Equal(t, "AWS Provider", aws.Name)
				assert.Equal(t, "Amazon Web Services provider", aws.Description)

				gcp, exists := def.Providers["gcp"]
				assert.True(t, exists)
				assert.Equal(t, "gcp", gcp.Provider)
				assert.Equal(t, "GCP Provider", gcp.Name)
			},
		},
		{
			name: "numeric version",
			jsonInput: `{
				"version": 2.5,
				"providers": {
					"azure": {
						"provider": "azure",
						"name": "Azure Provider",
						"description": "Microsoft Azure provider",
						"enabled": true
					}
				}
			}`,
			expectError:   false,
			expectedVer:   "2.5.0",
			providerCount: 1,
		},
		{
			name: "integer version",
			jsonInput: `{
				"version": 3,
				"providers": {
					"okta": {
						"provider": "okta",
						"name": "Okta Provider",
						"description": "Okta identity provider",
						"enabled": true
					}
				}
			}`,
			expectError:   false,
			expectedVer:   "3.0.0",
			providerCount: 1,
		},
		{
			name: "semver version",
			jsonInput: `{
				"version": "1.2.3",
				"providers": {
					"slack": {
						"provider": "slack",
						"name": "Slack Provider",
						"description": "Slack messaging provider",
						"enabled": true
					}
				}
			}`,
			expectError:   false,
			expectedVer:   "1.2.3",
			providerCount: 1,
		},
		{
			name: "empty providers map",
			jsonInput: `{
				"version": "1.0",
				"providers": {}
			}`,
			expectError:   false,
			expectedVer:   "1.0.0",
			providerCount: 0,
		},
		{
			name: "missing providers field",
			jsonInput: `{
				"version": "1.0"
			}`,
			expectError:   false,
			expectedVer:   "1.0.0",
			providerCount: 0,
		},
		{
			name: "invalid version",
			jsonInput: `{
				"version": "invalid-version",
				"providers": {}
			}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var def ProviderDefinitions
			err := json.Unmarshal([]byte(tt.jsonInput), &def)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, def.Version)
			assert.Equal(t, tt.expectedVer, def.Version.String())

			if tt.providerCount > 0 {
				require.NotNil(t, def.Providers)
				assert.Len(t, def.Providers, tt.providerCount)
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, &def)
			}
		})
	}
}

// TestProviderDefinitions_UnmarshalYAML tests the YAML unmarshaling with various version formats
func TestProviderDefinitions_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name          string
		yamlInput     string
		expectError   bool
		expectedVer   string
		providerCount int
		validateFunc  func(t *testing.T, def *ProviderDefinitions)
	}{
		{
			name: "string version with providers",
			yamlInput: `version: "1.0"
providers:
  aws:
    provider: "aws"
    name: "AWS Provider"
    description: "Amazon Web Services provider"
    enabled: true
  gcp:
    provider: "gcp"
    name: "GCP Provider"
    description: "Google Cloud Platform provider"
    enabled: true`,
			expectError:   false,
			expectedVer:   "1.0.0",
			providerCount: 2,
			validateFunc: func(t *testing.T, def *ProviderDefinitions) {
				aws, exists := def.Providers["aws"]
				assert.True(t, exists)
				assert.Equal(t, "aws", aws.Provider)
				assert.Equal(t, "AWS Provider", aws.Name)

				gcp, exists := def.Providers["gcp"]
				assert.True(t, exists)
				assert.Equal(t, "gcp", gcp.Provider)
				assert.Equal(t, "GCP Provider", gcp.Name)
			},
		},
		{
			name: "numeric version",
			yamlInput: `version: 2.5
providers:
  azure:
    provider: "azure"
    name: "Azure Provider"
    description: "Microsoft Azure provider"
    enabled: true`,
			expectError:   false,
			expectedVer:   "2.5.0",
			providerCount: 1,
		},
		{
			name: "integer version",
			yamlInput: `version: 3
providers:
  okta:
    provider: "okta"
    name: "Okta Provider"
    description: "Okta identity provider"
    enabled: true`,
			expectError:   false,
			expectedVer:   "3.0.0",
			providerCount: 1,
		},
		{
			name: "empty providers map",
			yamlInput: `version: "1.0"
providers: {}`,
			expectError:   false,
			expectedVer:   "1.0.0",
			providerCount: 0,
		},
		{
			name:          "missing providers field",
			yamlInput:     `version: "1.0"`,
			expectError:   false,
			expectedVer:   "1.0.0",
			providerCount: 0,
		},
		{
			name: "invalid version",
			yamlInput: `version: "invalid-version"
providers: {}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var def ProviderDefinitions
			err := yaml.Unmarshal([]byte(tt.yamlInput), &def)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, def.Version)
			assert.Equal(t, tt.expectedVer, def.Version.String())

			if tt.providerCount > 0 {
				require.NotNil(t, def.Providers)
				assert.Len(t, def.Providers, tt.providerCount)
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, &def)
			}
		})
	}
}

// TestProviderDefinitions_DirectUnmarshalVsViaJSON tests both paths work correctly
func TestProviderDefinitions_DirectUnmarshalVsViaJSON(t *testing.T) {
	yamlInput := `version: "1.5"
providers:
  provider1:
    type: "custom1"
    name: "Provider 1"
    description: "First provider"
  provider2:
    type: "custom2"
    name: "Provider 2"
    description: "Second provider"`

	t.Run("Direct YAML unmarshal", func(t *testing.T) {
		var def ProviderDefinitions
		err := yaml.Unmarshal([]byte(yamlInput), &def)
		require.NoError(t, err)

		assert.Equal(t, "1.5.0", def.Version.String())
		assert.Len(t, def.Providers, 2)
		assert.Equal(t, "Provider 1", def.Providers["provider1"].Name)
		assert.Equal(t, "Provider 2", def.Providers["provider2"].Name)
	})

	t.Run("YAML->JSON->Struct unmarshal (production path)", func(t *testing.T) {
		// Step 1: YAML to generic interface
		var yamlData any
		err := yaml.Unmarshal([]byte(yamlInput), &yamlData)
		require.NoError(t, err)

		// Step 2: Generic interface to JSON
		jsonData, err := json.Marshal(yamlData)
		require.NoError(t, err)

		// Step 3: JSON to struct
		var def ProviderDefinitions
		err = json.Unmarshal(jsonData, &def)
		require.NoError(t, err)

		assert.Equal(t, "1.5.0", def.Version.String())
		assert.Len(t, def.Providers, 2)
		assert.Equal(t, "Provider 1", def.Providers["provider1"].Name)
		assert.Equal(t, "Provider 2", def.Providers["provider2"].Name)
	})
}

// TestProviderDefinitions_RoundTrip tests that unmarshaling and marshaling preserves data
func TestProviderDefinitions_RoundTrip(t *testing.T) {
	t.Run("JSON round trip", func(t *testing.T) {
		original := ProviderDefinitions{
			Version: version.Must(version.NewVersion("1.2.3")),
			Providers: map[string]ProviderConfig{
				"aws": {
					Provider:    "aws",
					Name:        "AWS",
					Description: "AWS provider",
					Enabled:     true,
				},
			},
		}

		// Marshal to JSON
		jsonData, err := json.Marshal(original)
		require.NoError(t, err)

		// Unmarshal back
		var unmarshaled ProviderDefinitions
		err = json.Unmarshal(jsonData, &unmarshaled)
		require.NoError(t, err)

		// Verify
		assert.Equal(t, original.Version.String(), unmarshaled.Version.String())
		assert.Len(t, unmarshaled.Providers, 1)
		assert.Equal(t, original.Providers["aws"].Name, unmarshaled.Providers["aws"].Name)
	})

	t.Run("YAML round trip", func(t *testing.T) {
		original := ProviderDefinitions{
			Version: version.Must(version.NewVersion("2.0.0")),
			Providers: map[string]ProviderConfig{
				"gcp": {
					Provider:    "gcp",
					Name:        "GCP",
					Description: "GCP provider",
					Enabled:     true,
				},
			},
		}

		// Marshal to YAML
		yamlData, err := yaml.Marshal(original)
		require.NoError(t, err)

		// Unmarshal back
		var unmarshaled ProviderDefinitions
		err = yaml.Unmarshal(yamlData, &unmarshaled)
		require.NoError(t, err)

		// Verify
		assert.Equal(t, original.Version.String(), unmarshaled.Version.String())
		assert.Len(t, unmarshaled.Providers, 1)
		assert.Equal(t, original.Providers["gcp"].Name, unmarshaled.Providers["gcp"].Name)
	})
}
