package models_test

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
	"gopkg.in/yaml.v3"
)

// statementsContainOperation checks if any Statement in the RoleStatements contains the given operation
func statementsContainOperation(stmts models.RoleStatements, operation string) bool {
	for _, stmt := range stmts {
		for _, op := range stmt.Operations {
			if op == operation {
				return true
			}
		}
	}
	return false
}

// TestRoleDefinitions_UnmarshalJSON tests the JSON unmarshaling with various version formats
func TestRoleDefinitions_UnmarshalJSON(t *testing.T) {
	tests := []struct {
		name         string
		jsonInput    string
		expectError  bool
		expectedVer  string
		roleCount    int
		validateFunc func(t *testing.T, def *models.RoleDefinitions)
	}{
		{
			name: "string version with roles",
			jsonInput: `{
				"version": "1.0",
				"roles": {
					"admin": {
						"name": "Administrator",
						"description": "Full access role",
						"permissions": {
							"allow": ["*"]
						}
					},
					"viewer": {
						"name": "Viewer",
						"description": "Read-only access",
						"permissions": {
							"allow": ["read"],
							"deny": ["write", "delete"]
						}
					}
				}
			}`,
			expectError: false,
			expectedVer: "1.0.0",
			roleCount:   2,
			validateFunc: func(t *testing.T, def *models.RoleDefinitions) {
				admin, exists := def.Roles["admin"]
				assert.True(t, exists)
				assert.Equal(t, "Administrator", admin.Name)
				assert.Equal(t, "Full access role", admin.Description)
				assert.True(t, statementsContainOperation(admin.Permissions.Allow, "*"), "admin should have * permission")

				viewer, exists := def.Roles["viewer"]
				assert.True(t, exists)
				assert.Equal(t, "Viewer", viewer.Name)
				assert.True(t, statementsContainOperation(viewer.Permissions.Allow, "read"), "viewer should have read permission")
				assert.True(t, statementsContainOperation(viewer.Permissions.Deny, "write"), "viewer should deny write permission")
			},
		},
		{
			name: "numeric version",
			jsonInput: `{
				"version": 2.5,
				"roles": {
					"developer": {
						"name": "Developer",
						"description": "Developer role"
					}
				}
			}`,
			expectError: false,
			expectedVer: "2.5.0",
			roleCount:   1,
		},
		{
			name: "integer version",
			jsonInput: `{
				"version": 3,
				"roles": {
					"operator": {
						"name": "Operator",
						"description": "Operations role"
					}
				}
			}`,
			expectError: false,
			expectedVer: "3.0.0",
			roleCount:   1,
		},
		{
			name: "semver version",
			jsonInput: `{
				"version": "1.2.3",
				"roles": {
					"auditor": {
						"name": "Auditor",
						"description": "Audit role"
					}
				}
			}`,
			expectError: false,
			expectedVer: "1.2.3",
			roleCount:   1,
		},
		{
			name: "empty roles map",
			jsonInput: `{
				"version": "1.0",
				"roles": {}
			}`,
			expectError: false,
			expectedVer: "1.0.0",
			roleCount:   0,
		},
		{
			name: "missing roles field",
			jsonInput: `{
				"version": "1.0"
			}`,
			expectError: false,
			expectedVer: "1.0.0",
			roleCount:   0,
		},
		{
			name: "invalid version",
			jsonInput: `{
				"version": "invalid-version",
				"roles": {}
			}`,
			expectError: true,
		},
		{
			name: "RolesResponse format with SearchResult array",
			jsonInput: `{
				"version": "1.0",
				"roles": [
					{
						"_id": "admin",
						"_score": 1.0,
						"_source": {
							"identifier": "admin",
							"name": "Administrator",
							"description": "Full access role",
							"permissions": {
								"allow": ["*"]
							}
						}
					},
					{
						"_id": "viewer",
						"_score": 0.9,
						"_source": {
							"identifier": "viewer",
							"name": "Viewer",
							"description": "Read-only access",
							"permissions": {
								"allow": ["read"],
								"deny": ["write"]
							}
						}
					}
				],
				"meta": {
					"total": 2,
					"page": 1,
					"page_size": 10
				}
			}`,
			expectError: false,
			expectedVer: "1.0.0",
			roleCount:   2,
			validateFunc: func(t *testing.T, def *models.RoleDefinitions) {
				admin, exists := def.Roles["admin"]
				assert.True(t, exists, "admin role should exist")
				assert.Equal(t, "Administrator", admin.Name)
				assert.Equal(t, "Full access role", admin.Description)
				assert.Equal(t, "admin", admin.Identifier)
				assert.True(t, statementsContainOperation(admin.Permissions.Allow, "*"), "admin should have * permission")

				viewer, exists := def.Roles["viewer"]
				assert.True(t, exists, "viewer role should exist")
				assert.Equal(t, "Viewer", viewer.Name)
				assert.Equal(t, "viewer", viewer.Identifier)
				assert.True(t, statementsContainOperation(viewer.Permissions.Allow, "read"), "viewer should have read permission")
				assert.True(t, statementsContainOperation(viewer.Permissions.Deny, "write"), "viewer should deny write permission")

				// Check meta was parsed
				assert.Equal(t, int64(2), def.Meta.Total)
				assert.Equal(t, 1, def.Meta.Page)
				assert.Equal(t, 10, def.Meta.PageSize)
			},
		},
		{
			name: "RolesResponse format with empty array",
			jsonInput: `{
				"version": "1.0",
				"roles": [],
				"meta": {
					"total": 0
				}
			}`,
			expectError: false,
			expectedVer: "1.0.0",
			roleCount:   0,
			validateFunc: func(t *testing.T, def *models.RoleDefinitions) {
				assert.Equal(t, int64(0), def.Meta.Total)
			},
		},
		{
			name: "RolesResponse format with minimal SearchResult",
			jsonInput: `{
				"version": "2.0",
				"roles": [
					{
						"_source": {
							"identifier": "developer",
							"name": "Developer",
							"description": "Dev role"
						}
					}
				]
			}`,
			expectError: false,
			expectedVer: "2.0.0",
			roleCount:   1,
			validateFunc: func(t *testing.T, def *models.RoleDefinitions) {
				dev, exists := def.Roles["developer"]
				assert.True(t, exists)
				assert.Equal(t, "Developer", dev.Name)
				assert.Equal(t, "developer", dev.Identifier)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var def models.RoleDefinitions
			err := json.Unmarshal([]byte(tt.jsonInput), &def)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, def.Version)
			assert.Equal(t, tt.expectedVer, def.Version.String())

			if tt.roleCount > 0 {
				require.NotNil(t, def.Roles)
				assert.Len(t, def.Roles, tt.roleCount)
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, &def)
			}
		})
	}
}

// TestRoleDefinitions_UnmarshalYAML tests the YAML unmarshaling with various version formats
func TestRoleDefinitions_UnmarshalYAML(t *testing.T) {
	tests := []struct {
		name         string
		yamlInput    string
		expectError  bool
		expectedVer  string
		roleCount    int
		validateFunc func(t *testing.T, def *models.RoleDefinitions)
	}{
		{
			name: "string version with roles",
			yamlInput: `version: "1.0"
roles:
  admin:
    name: "Administrator"
    description: "Full access role"
    permissions:
      allow:
        - operations: ["*"]
  viewer:
    name: "Viewer"
    description: "Read-only access"
    permissions:
      allow:
        - operations: ["read"]
      deny:
        - operations: ["write"]
        - operations: ["delete"]`,
			expectError: false,
			expectedVer: "1.0.0",
			roleCount:   2,
			validateFunc: func(t *testing.T, def *models.RoleDefinitions) {
				admin, exists := def.Roles["admin"]
				assert.True(t, exists)
				assert.Equal(t, "Administrator", admin.Name)

				viewer, exists := def.Roles["viewer"]
				assert.True(t, exists)
				assert.Equal(t, "Viewer", viewer.Name)
			},
		},
		{
			name: "numeric version",
			yamlInput: `version: 2.5
roles:
  developer:
    name: "Developer"
    description: "Developer role"`,
			expectError: false,
			expectedVer: "2.5.0",
			roleCount:   1,
		},
		{
			name: "integer version",
			yamlInput: `version: 3
roles:
  operator:
    name: "Operator"
    description: "Operations role"`,
			expectError: false,
			expectedVer: "3.0.0",
			roleCount:   1,
		},
		{
			name: "empty roles map",
			yamlInput: `version: "1.0"
roles: {}`,
			expectError: false,
			expectedVer: "1.0.0",
			roleCount:   0,
		},
		{
			name:        "missing roles field",
			yamlInput:   `version: "1.0"`,
			expectError: false,
			expectedVer: "1.0.0",
			roleCount:   0,
		},
		{
			name: "invalid version",
			yamlInput: `version: "invalid-version"
roles: {}`,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var def models.RoleDefinitions
			err := yaml.Unmarshal([]byte(tt.yamlInput), &def)

			if tt.expectError {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, def.Version)
			assert.Equal(t, tt.expectedVer, def.Version.String())

			if tt.roleCount > 0 {
				require.NotNil(t, def.Roles)
				assert.Len(t, def.Roles, tt.roleCount)
			}

			if tt.validateFunc != nil {
				tt.validateFunc(t, &def)
			}
		})
	}
}

// TestRoleDefinitions_DirectUnmarshalVsViaJSON tests both paths work correctly
func TestRoleDefinitions_DirectUnmarshalVsViaJSON(t *testing.T) {
	yamlInput := `version: "1.5"
roles:
  role1:
    name: "Role 1"
    description: "First role"
  role2:
    name: "Role 2"
    description: "Second role"`

	t.Run("Direct YAML unmarshal", func(t *testing.T) {
		var def models.RoleDefinitions
		err := yaml.Unmarshal([]byte(yamlInput), &def)
		require.NoError(t, err)

		assert.Equal(t, "1.5.0", def.Version.String())
		assert.Len(t, def.Roles, 2)
		assert.Equal(t, "Role 1", def.Roles["role1"].Name)
		assert.Equal(t, "Role 2", def.Roles["role2"].Name)
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
		var def models.RoleDefinitions
		err = json.Unmarshal(jsonData, &def)
		require.NoError(t, err)

		assert.Equal(t, "1.5.0", def.Version.String())
		assert.Len(t, def.Roles, 2)
		assert.Equal(t, "Role 1", def.Roles["role1"].Name)
		assert.Equal(t, "Role 2", def.Roles["role2"].Name)
	})
}

// TestRoleDefinitions_RoundTrip tests that unmarshaling and marshaling preserves data
func TestRoleDefinitions_RoundTrip(t *testing.T) {
	t.Run("JSON round trip", func(t *testing.T) {
		original := models.RoleDefinitions{
			Version: version.Must(version.NewVersion("1.2.3")),
			Roles: map[string]models.Role{
				"admin": {
					Name:        "Admin",
					Description: "Admin role",
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{{Operations: []string{"*"}}},
					},
				},
			},
		}

		// Marshal to JSON
		jsonData, err := json.Marshal(original)
		require.NoError(t, err)

		// Unmarshal back
		var unmarshaled models.RoleDefinitions
		err = json.Unmarshal(jsonData, &unmarshaled)
		require.NoError(t, err)

		// Verify
		assert.Equal(t, original.Version.String(), unmarshaled.Version.String())
		assert.Len(t, unmarshaled.Roles, 1)
		assert.Equal(t, original.Roles["admin"].Name, unmarshaled.Roles["admin"].Name)
	})

	t.Run("YAML round trip", func(t *testing.T) {
		original := models.RoleDefinitions{
			Version: version.Must(version.NewVersion("2.0.0")),
			Roles: map[string]models.Role{
				"viewer": {
					Name:        "Viewer",
					Description: "View role",
				},
			},
		}

		// Marshal to YAML
		yamlData, err := yaml.Marshal(original)
		require.NoError(t, err)

		// Unmarshal back
		var unmarshaled models.RoleDefinitions
		err = yaml.Unmarshal(yamlData, &unmarshaled)
		require.NoError(t, err)

		// Verify
		assert.Equal(t, original.Version.String(), unmarshaled.Version.String())
		assert.Len(t, unmarshaled.Roles, 1)
		assert.Equal(t, original.Roles["viewer"].Name, unmarshaled.Roles["viewer"].Name)
	})
}

// TestRoleDefinitions_BothFormats tests that both RoleDefinitions and RolesResponse formats work
func TestRoleDefinitions_BothFormats(t *testing.T) {
	t.Run("RoleDefinitions map format", func(t *testing.T) {
		jsonInput := `{
			"version": "1.0",
			"roles": {
				"admin": {
					"identifier": "admin",
					"name": "Administrator",
					"description": "Admin role"
				},
				"viewer": {
					"identifier": "viewer",
					"name": "Viewer",
					"description": "View role"
				}
			}
		}`

		var def models.RoleDefinitions
		err := json.Unmarshal([]byte(jsonInput), &def)
		require.NoError(t, err)

		assert.Equal(t, "1.0.0", def.Version.String())
		assert.Len(t, def.Roles, 2)
		assert.Equal(t, "Administrator", def.Roles["admin"].Name)
		assert.Equal(t, "Viewer", def.Roles["viewer"].Name)
	})

	t.Run("RolesResponse array format", func(t *testing.T) {
		jsonInput := `{
			"version": "1.0",
			"roles": [
				{
					"_source": {
						"identifier": "admin",
						"name": "Administrator",
						"description": "Admin role"
					}
				},
				{
					"_source": {
						"identifier": "viewer",
						"name": "Viewer",
						"description": "View role"
					}
				}
			]
		}`

		var def models.RoleDefinitions
		err := json.Unmarshal([]byte(jsonInput), &def)
		require.NoError(t, err)

		assert.Equal(t, "1.0.0", def.Version.String())
		assert.Len(t, def.Roles, 2)
		assert.Equal(t, "Administrator", def.Roles["admin"].Name)
		assert.Equal(t, "Viewer", def.Roles["viewer"].Name)
	})

	t.Run("RolesResponse with missing identifier skipped", func(t *testing.T) {
		jsonInput := `{
			"version": "1.0",
			"roles": [
				{
					"_source": {
						"identifier": "valid",
						"name": "Valid Role"
					}
				},
				{
					"_source": {
						"name": "Invalid Role - No Identifier"
					}
				}
			]
		}`

		var def models.RoleDefinitions
		err := json.Unmarshal([]byte(jsonInput), &def)
		require.NoError(t, err)

		// Should only have the role with an identifier
		assert.Len(t, def.Roles, 1)
		assert.Equal(t, "Valid Role", def.Roles["valid"].Name)
	})
}
