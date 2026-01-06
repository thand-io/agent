package models

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestWorkflowDefinitions_UnmarshalJSON tests the JSON unmarshaling with various version formats
func TestWorkflowDefinitions_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name          string
		jsonInput     string
		expectError   bool
		expectedVer   string
		workflowCount int
		validateFunc  func(t *testing.T, def *WorkflowDefinitions)
	}{
		{
			name: "string version with workflows",
			jsonInput: `{
				"version": "1.0",
				"workflows": {
					"deploy": {
						"name": "Deploy Workflow",
						"description": "Deploys the application",
						"enabled": true
					},
					"test": {
						"name": "Test Workflow",
						"description": "Runs tests",
						"enabled": false
					}
				}
			}`,
			expectError:   false,
			expectedVer:   "1.0.0",
			workflowCount: 2,
			validateFunc: func(t *testing.T, def *WorkflowDefinitions) {
				deploy, exists := def.Workflows["deploy"]
				assert.True(t, exists)
				assert.Equal(t, "Deploy Workflow", deploy.Name)
				assert.Equal(t, "Deploys the application", deploy.Description)
				assert.True(t, deploy.Enabled)

				test, exists := def.Workflows["test"]
				assert.True(t, exists)
				assert.Equal(t, "Test Workflow", test.Name)
				assert.False(t, test.Enabled)
			},
		},
		{
			name: "numeric version",
			jsonInput: `{
				"version": 2.5,
				"workflows": {
					"build": {
						"name": "Build",
						"description": "Build the project",
						"enabled": true
					}
				}
			}`,
			expectError:   false,
			expectedVer:   "2.5.0",
			workflowCount: 1,
		},
		{
			name: "integer version",
			jsonInput: `{
				"version": 3,
				"workflows": {
					"lint": {
						"name": "Lint",
						"description": "Lint the code",
						"enabled": true
					}
				}
			}`,
			expectError:   false,
			expectedVer:   "3.0.0",
			workflowCount: 1,
		},
		{
			name: "semver version",
			jsonInput: `{
				"version": "1.2.3",
				"workflows": {
					"release": {
						"name": "Release",
						"description": "Release workflow",
						"enabled": true
					}
				}
			}`,
			expectError:   false,
			expectedVer:   "1.2.3",
			workflowCount: 1,
		},
		{
			name: "empty workflows map",
			jsonInput: `{
				"version": "1.0",
				"workflows": {}
			}`,
			expectError:   false,
			expectedVer:   "1.0.0",
			workflowCount: 0,
		},
		{
			name: "missing workflows field",
			jsonInput: `{
				"version": "1.0"
			}`,
			expectError:   false,
			expectedVer:   "1.0.0",
			workflowCount: 0,
		},
		{
			name: "invalid version",
			jsonInput: `{
				"version": "invalid-version",
				"workflows": {}
			}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var def WorkflowDefinitions
			err := json.Unmarshal([]byte(tt.jsonInput), &def)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, def.Version)
			assert.Equal(t, tt.expectedVer, def.Version.String())

			if tt.workflowCount > 0 {
				require.NotNil(t, def.Workflows)
				assert.Len(t, def.Workflows, tt.workflowCount)
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, &def)
			}
		})
	}
}

// TestWorkflowDefinitions_UnmarshalYAML tests the YAML unmarshaling with various version formats
func TestWorkflowDefinitions_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name          string
		yamlInput     string
		expectError   bool
		expectedVer   string
		workflowCount int
		validateFunc  func(t *testing.T, def *WorkflowDefinitions)
	}{
		{
			name: "string version with workflows",
			yamlInput: `version: "1.0"
workflows:
  deploy:
    name: "Deploy Workflow"
    description: "Deploys the application"
    enabled: true
  test:
    name: "Test Workflow"
    description: "Runs tests"
    enabled: false`,
			expectError:   false,
			expectedVer:   "1.0.0",
			workflowCount: 2,
			validateFunc: func(t *testing.T, def *WorkflowDefinitions) {
				deploy, exists := def.Workflows["deploy"]
				assert.True(t, exists)
				assert.Equal(t, "Deploy Workflow", deploy.Name)
				assert.Equal(t, "Deploys the application", deploy.Description)
				assert.True(t, deploy.Enabled)

				test, exists := def.Workflows["test"]
				assert.True(t, exists)
				assert.Equal(t, "Test Workflow", test.Name)
				assert.False(t, test.Enabled)
			},
		},
		{
			name: "numeric version",
			yamlInput: `version: 2.5
workflows:
  build:
    name: "Build"
    description: "Build the project"
    enabled: true`,
			expectError:   false,
			expectedVer:   "2.5.0",
			workflowCount: 1,
		},
		{
			name: "integer version",
			yamlInput: `version: 3
workflows:
  lint:
    name: "Lint"
    description: "Lint the code"
    enabled: true`,
			expectError:   false,
			expectedVer:   "3.0.0",
			workflowCount: 1,
		},
		{
			name: "semver version",
			yamlInput: `version: "1.2.3"
workflows:
  release:
    name: "Release"
    description: "Release workflow"
    enabled: true`,
			expectError:   false,
			expectedVer:   "1.2.3",
			workflowCount: 1,
		},
		{
			name: "empty workflows map",
			yamlInput: `version: "1.0"
workflows: {}`,
			expectError:   false,
			expectedVer:   "1.0.0",
			workflowCount: 0,
		},
		{
			name:          "missing workflows field",
			yamlInput:     `version: "1.0"`,
			expectError:   false,
			expectedVer:   "1.0.0",
			workflowCount: 0,
		},
		{
			name: "invalid version",
			yamlInput: `version: "invalid-version"
workflows: {}`,
			expectError: true,
		},
		{
			name: "complex workflow with nested data",
			yamlInput: `version: "2.0"
workflows:
  scheduled-workflow:
    name: "Scheduled Workflow"
    description: "Workflow with schedule trigger"
    enabled: true`,
			expectError:   false,
			expectedVer:   "2.0.0",
			workflowCount: 1,
			validateFunc: func(t *testing.T, def *WorkflowDefinitions) {
				scheduled, exists := def.Workflows["scheduled-workflow"]
				assert.True(t, exists)
				assert.Equal(t, "Scheduled Workflow", scheduled.Name)
				assert.True(t, scheduled.Enabled)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var def WorkflowDefinitions
			err := yaml.Unmarshal([]byte(tt.yamlInput), &def)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, def.Version)
			assert.Equal(t, tt.expectedVer, def.Version.String())

			if tt.workflowCount > 0 {
				require.NotNil(t, def.Workflows)
				assert.Len(t, def.Workflows, tt.workflowCount)
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, &def)
			}
		})
	}
}

// TestWorkflowDefinitions_RoundTrip tests that unmarshaling and marshaling preserves data
func TestWorkflowDefinitions_RoundTrip(t *testing.T) {
	t.Run("JSON round trip", func(t *testing.T) {
		original := WorkflowDefinitions{
			Version: version.Must(version.NewVersion("1.2.3")),
			Workflows: map[string]Workflow{
				"deploy": {
					Name:        "Deploy",
					Description: "Deploy app",
					Enabled:     true,
				},
				"test": {
					Name:        "Test",
					Description: "Run tests",
					Enabled:     false,
				},
			},
		}

		// Marshal to JSON
		jsonData, err := json.Marshal(original)
		require.NoError(t, err)

		// Unmarshal back
		var unmarshaled WorkflowDefinitions
		err = json.Unmarshal(jsonData, &unmarshaled)
		require.NoError(t, err)

		// Verify
		assert.Equal(t, original.Version.String(), unmarshaled.Version.String())
		assert.Len(t, unmarshaled.Workflows, 2)
		assert.Equal(t, original.Workflows["deploy"].Name, unmarshaled.Workflows["deploy"].Name)
		assert.Equal(t, original.Workflows["test"].Enabled, unmarshaled.Workflows["test"].Enabled)
	})

	t.Run("YAML round trip", func(t *testing.T) {
		original := WorkflowDefinitions{
			Version: version.Must(version.NewVersion("2.0.0")),
			Workflows: map[string]Workflow{
				"build": {
					Name:        "Build",
					Description: "Build project",
					Enabled:     true,
				},
			},
		}

		// Marshal to YAML
		yamlData, err := yaml.Marshal(original)
		require.NoError(t, err)

		// Unmarshal back
		var unmarshaled WorkflowDefinitions
		err = yaml.Unmarshal(yamlData, &unmarshaled)
		require.NoError(t, err)

		// Verify
		assert.Equal(t, original.Version.String(), unmarshaled.Version.String())
		assert.Len(t, unmarshaled.Workflows, 1)
		assert.Equal(t, original.Workflows["build"].Name, unmarshaled.Workflows["build"].Name)
	})
}

// TestWorkflowDefinitions_DirectUnmarshalVsViaJSON tests both paths work correctly
func TestWorkflowDefinitions_DirectUnmarshalVsViaJSON(t *testing.T) {
	yamlInput := `version: "1.5"
workflows:
  workflow1:
    name: "Workflow 1"
    description: "First workflow"
    enabled: true
  workflow2:
    name: "Workflow 2"
    description: "Second workflow"
    enabled: false`

	t.Run("Direct YAML unmarshal", func(t *testing.T) {
		var def WorkflowDefinitions
		err := yaml.Unmarshal([]byte(yamlInput), &def)
		require.NoError(t, err)

		assert.Equal(t, "1.5.0", def.Version.String())
		assert.Len(t, def.Workflows, 2)
		assert.Equal(t, "Workflow 1", def.Workflows["workflow1"].Name)
		assert.Equal(t, "Workflow 2", def.Workflows["workflow2"].Name)
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
		var def WorkflowDefinitions
		err = json.Unmarshal(jsonData, &def)
		require.NoError(t, err)

		assert.Equal(t, "1.5.0", def.Version.String())
		assert.Len(t, def.Workflows, 2)
		assert.Equal(t, "Workflow 1", def.Workflows["workflow1"].Name)
		assert.Equal(t, "Workflow 2", def.Workflows["workflow2"].Name)
	})
}

// TestWorkflowDefinitions_EmptyAndNil tests edge cases with empty/nil workflows
func TestWorkflowDefinitions_EmptyAndNil(t *testing.T) {
	t.Run("explicit null workflows in JSON", func(t *testing.T) {
		jsonInput := `{"version": "1.0", "workflows": null}`
		var def WorkflowDefinitions
		err := json.Unmarshal([]byte(jsonInput), &def)
		require.NoError(t, err)
		assert.Equal(t, "1.0.0", def.Version.String())
		// When explicitly null, it will be nil (not an initialized empty map)
		assert.Nil(t, def.Workflows)
	})

	t.Run("explicit null workflows in YAML", func(t *testing.T) {
		yamlInput := `version: "1.0"
workflows: null`
		var def WorkflowDefinitions
		err := yaml.Unmarshal([]byte(yamlInput), &def)
		require.NoError(t, err)
		assert.Equal(t, "1.0.0", def.Version.String())
		// When explicitly null, it will be nil (not an initialized empty map)
		assert.Nil(t, def.Workflows)
	})

	t.Run("empty workflows map in JSON", func(t *testing.T) {
		jsonInput := `{"version": "1.0", "workflows": {}}`
		var def WorkflowDefinitions
		err := json.Unmarshal([]byte(jsonInput), &def)
		require.NoError(t, err)
		assert.Equal(t, "1.0.0", def.Version.String())
		assert.NotNil(t, def.Workflows)
		assert.Len(t, def.Workflows, 0)
	})

	t.Run("empty workflows map in YAML", func(t *testing.T) {
		yamlInput := `version: "1.0"
workflows: {}`
		var def WorkflowDefinitions
		err := yaml.Unmarshal([]byte(yamlInput), &def)
		require.NoError(t, err)
		assert.Equal(t, "1.0.0", def.Version.String())
		assert.NotNil(t, def.Workflows)
		assert.Len(t, def.Workflows, 0)
	})
}
