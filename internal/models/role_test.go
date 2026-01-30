package models_test

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/thand-io/agent/internal/models"
)

func TestRole_HasPermission(t *testing.T) {
	tests := []struct {
		name     string
		role     models.Role
		user     *models.User
		expected bool
	}{
		{
			name: "nil user returns false",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
			},
			user:     nil,
			expected: false,
		},
		{
			name: "no scopes defined allows access",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
			},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
			},
			expected: true,
		},
		{
			name: "empty scopes allows access",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
			},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
			},
			expected: true,
		},
		{
			name: "user in allowed users by username",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Users: []string{"testuser", "otheruser"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
			},
			expected: true,
		},
		{
			name: "user in allowed users by ID",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Users: []string{"user1", "user2"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
			},
			expected: true,
		},
		{
			name: "user not in allowed users",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Users: []string{"otheruser", "anotheruser"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
			},
			expected: false,
		},
		{
			name: "user in allowed group",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Groups: []string{"admins", "developers"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
				Groups:   []string{"developers", "testers"},
			},
			expected: true,
		},
		{
			name: "user not in allowed groups",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Groups: []string{"admins", "superusers"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
				Groups:   []string{"developers", "testers"},
			},
			expected: false,
		},
		{
			name: "user in allowed domain",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Domains: []string{"example.com", "company.org"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
			},
			expected: true,
		},
		{
			name: "user not in allowed domains but no users or groups scopes",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Domains: []string{"company.org", "internal.net"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
			},
			// Domain scopes properly deny access when user's domain doesn't match
			expected: false,
		},
		{
			name: "user matches via group when users scope is also defined",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Users:  []string{"otheruser"},
					Groups: []string{"developers"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
				Groups:   []string{"developers"},
			},
			expected: true,
		},
		{
			name: "user matches via domain when users and groups are defined",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Users:   []string{"otheruser"},
					Groups:  []string{"admins"},
					Domains: []string{"example.com"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
				Groups:   []string{"developers"},
			},
			expected: true,
		},
		{
			name: "user with no groups and group scope defined",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Groups: []string{"admins"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
				Groups:   nil,
			},
			expected: false,
		},
		{
			name: "user with empty groups and group scope defined",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Groups: []string{"admins"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
				Groups:   []string{},
			},
			expected: false,
		},
		{
			name: "user with no email and domain scope defined",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Domains: []string{"example.com"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "",
			},
			// Domain scopes deny access when user has no email (empty domain won't match)
			expected: false,
		},
		{
			name: "multiple users in scope - first match",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Users: []string{"testuser", "user2", "user3"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
			},
			expected: true,
		},
		{
			name: "multiple users in scope - last match",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Users: []string{"user1", "user2", "testuser"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "differentuser",
				Email:    "test@example.com",
			},
			expected: true,
		},
		{
			name: "user in multiple groups - one matches",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Groups: []string{"admins"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
				Groups:   []string{"developers", "admins", "testers"},
			},
			expected: true,
		},
		{
			name: "only domains scope defined - user not matching is denied",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Domains: []string{"company.org"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
			},
			// Domain scopes properly deny access when user's domain doesn't match
			expected: false,
		},
		{
			name: "domains with users scope - user domain not matching and user not in list",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Users:   []string{"otheruser"},
					Domains: []string{"company.org"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
			},
			expected: false,
		},
		// Case-insensitive matching tests
		{
			name: "user matches by username case-insensitive",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Users: []string{"TestUser"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
			},
			expected: true,
		},
		{
			name: "user matches by email case-insensitive",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Users: []string{"Test@Example.COM"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
			},
			expected: true,
		},
		{
			name: "user matches group case-insensitive",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Groups: []string{"ADMINS", "Developers"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
				Groups:   []string{"admins"},
			},
			expected: true,
		},
		{
			name: "user matches domain case-insensitive",
			role: models.Role{
				Name:        "admin",
				Description: "Admin role",
				Scopes: models.RoleScopes{Allow: models.ScopeIdentities{
					Domains: []string{"Example.COM"},
				},
				}},
			user: &models.User{
				ID:       "user1",
				Username: "testuser",
				Email:    "test@example.com",
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.role.HasPermission(tt.user)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRole_IsValid(t *testing.T) {
	tests := []struct {
		name     string
		role     models.Role
		expected bool
	}{
		{
			name: "valid role with name and description",
			role: models.Role{
				Name:        "admin",
				Description: "Administrator role",
			},
			expected: true,
		},
		{
			name: "invalid role - empty name",
			role: models.Role{
				Name:        "",
				Description: "Administrator role",
			},
			expected: false,
		},
		{
			name: "invalid role - empty description",
			role: models.Role{
				Name:        "admin",
				Description: "",
			},
			expected: false,
		},
		{
			name: "invalid role - both empty",
			role: models.Role{
				Name:        "",
				Description: "",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := tt.role.IsValid()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRole_GetName(t *testing.T) {
	role := models.Role{Name: "admin"}
	assert.Equal(t, "admin", role.GetName())
}

func TestRole_GetSnakeCaseName(t *testing.T) {
	tests := []struct {
		name     string
		roleName string
		expected string
	}{
		{
			name:     "simple name",
			roleName: "admin",
			expected: "admin",
		},
		{
			name:     "camelCase name",
			roleName: "superAdmin",
			expected: "superadmin",
		},
		{
			name:     "name with spaces",
			roleName: "Super Admin",
			expected: "super_admin",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			role := models.Role{Name: tt.roleName}
			result := role.GetSnakeCaseName()
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestRole_GetDescription(t *testing.T) {
	role := models.Role{Description: "Test description"}
	assert.Equal(t, "Test description", role.GetDescription())
}

func TestRole_AsMap(t *testing.T) {
	role := models.Role{
		Name:        "admin",
		Description: "Administrator role",
		Enabled:     true,
		Providers:   []string{"aws", "gcp"},
	}

	result := role.AsMap()
	assert.NotNil(t, result)
	assert.Equal(t, "admin", result["name"])
	assert.Equal(t, "Administrator role", result["description"])
	assert.Equal(t, true, result["enabled"])
}

func TestStatements_UnmarshalJSON_BackwardsCompatibility(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected models.RoleStatements
		wantErr  bool
	}{
		{
			name: "old format - array of strings",
			json: `["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]`,
			expected: models.RoleStatements{
				{Operations: []string{"s3:GetObject"}, Targets: []string{}},
				{Operations: []string{"s3:PutObject"}, Targets: []string{}},
				{Operations: []string{"s3:DeleteObject"}, Targets: []string{}},
			},
			wantErr: false,
		},
		{
			name: "new format - array of statement objects",
			json: `[
				{
					"operations": ["s3:GetObject", "s3:ListBucket"],
					"targets": ["arn:aws:s3:::my-bucket/*"]
				}
			]`,
			expected: models.RoleStatements{
				{
					Operations: []string{"s3:GetObject", "s3:ListBucket"},
					Targets:    []string{"arn:aws:s3:::my-bucket/*"},
				},
			},
			wantErr: false,
		},
		{
			name: "new format with conditions",
			json: `[
				{
					"operations": ["s3:GetObject"],
					"targets": ["arn:aws:s3:::my-bucket/*"],
					"conditions": {"IpAddress": {"aws:SourceIp": "10.0.0.0/8"}}
				}
			]`,
			expected: models.RoleStatements{
				{
					Operations: []string{"s3:GetObject"},
					Targets:    []string{"arn:aws:s3:::my-bucket/*"},
					Conditions: map[string]any{
						"IpAddress": map[string]any{
							"aws:SourceIp": "10.0.0.0/8",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "mixed format - strings and objects",
			json: `[
				"s3:ListBucket",
				{
					"operations": ["s3:GetObject"],
					"targets": ["arn:aws:s3:::my-bucket/*"]
				},
				"s3:PutObject"
			]`,
			expected: models.RoleStatements{
				{Operations: []string{"s3:ListBucket"}, Targets: []string{}},
				{
					Operations: []string{"s3:GetObject"},
					Targets:    []string{"arn:aws:s3:::my-bucket/*"},
				},
				{Operations: []string{"s3:PutObject"}, Targets: []string{}},
			},
			wantErr: false,
		},
		{
			name:     "empty array",
			json:     `[]`,
			expected: models.RoleStatements{},
			wantErr:  false,
		},
		{
			name:    "invalid json",
			json:    `not valid json`,
			wantErr: true,
		},
		{
			name:    "invalid element type - number",
			json:    `[123, "s3:GetObject"]`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var statements models.RoleStatements
			err := statements.UnmarshalJSON([]byte(tt.json))

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, len(tt.expected), len(statements))

			for i, expected := range tt.expected {
				assert.Equal(t, expected.Operations, statements[i].Operations, "Operations mismatch at index %d", i)
				assert.Equal(t, expected.Targets, statements[i].Targets, "Targets mismatch at index %d", i)
				if expected.Conditions != nil {
					assert.NotNil(t, statements[i].Conditions)
				}
			}
		})
	}
}

func TestPermissions_UnmarshalJSON_BackwardsCompatibility(t *testing.T) {
	tests := []struct {
		name     string
		json     string
		expected models.RolePermissions
		wantErr  bool
	}{
		{
			name: "old format - string arrays for allow and deny",
			json: `{
				"allow": ["s3:GetObject", "s3:PutObject"],
				"deny": ["s3:DeleteObject"]
			}`,
			expected: models.RolePermissions{
				Allow: models.RoleStatements{
					{Operations: []string{"s3:GetObject"}, Targets: []string{}},
					{Operations: []string{"s3:PutObject"}, Targets: []string{}},
				},
				Deny: models.RoleStatements{
					{Operations: []string{"s3:DeleteObject"}, Targets: []string{}},
				},
			},
			wantErr: false,
		},
		{
			name: "new format - statement objects",
			json: `{
				"allow": [
					{
						"operations": ["s3:GetObject"],
						"targets": ["arn:aws:s3:::my-bucket/*"]
					}
				],
				"deny": []
			}`,
			expected: models.RolePermissions{
				Allow: models.RoleStatements{
					{
						Operations: []string{"s3:GetObject"},
						Targets:    []string{"arn:aws:s3:::my-bucket/*"},
					},
				},
				Deny: models.RoleStatements{},
			},
			wantErr: false,
		},
		{
			name: "mixed format in allow",
			json: `{
				"allow": [
					"s3:ListBucket",
					{
						"operations": ["s3:GetObject"],
						"targets": ["arn:aws:s3:::my-bucket/*"]
					}
				]
			}`,
			expected: models.RolePermissions{
				Allow: models.RoleStatements{
					{Operations: []string{"s3:ListBucket"}, Targets: []string{}},
					{
						Operations: []string{"s3:GetObject"},
						Targets:    []string{"arn:aws:s3:::my-bucket/*"},
					},
				},
			},
			wantErr: false,
		},
		{
			name:     "empty permissions",
			json:     `{}`,
			expected: models.RolePermissions{},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var permissions models.RolePermissions
			err := json.Unmarshal([]byte(tt.json), &permissions)

			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			assert.NoError(t, err)
			assert.Equal(t, len(tt.expected.Allow), len(permissions.Allow), "Allow length mismatch")
			assert.Equal(t, len(tt.expected.Deny), len(permissions.Deny), "Deny length mismatch")

			for i, expected := range tt.expected.Allow {
				assert.Equal(t, expected.Operations, permissions.Allow[i].Operations)
				assert.Equal(t, expected.Targets, permissions.Allow[i].Targets)
			}

			for i, expected := range tt.expected.Deny {
				assert.Equal(t, expected.Operations, permissions.Deny[i].Operations)
				assert.Equal(t, expected.Targets, permissions.Deny[i].Targets)
			}
		})
	}
}

func TestRole_GetUniqueIdentifier(t *testing.T) {
	tests := []struct {
		name     string
		role     models.Role
		user     *models.User
		expected string
		validate func(t *testing.T, identifier string)
	}{
		{
			name: "basic role with user",
			role: models.Role{
				Identifier: "role-123",
				Name:       "Admin Role",
			},
			user: &models.User{
				ID:       "user-456",
				Username: "testuser",
				Email:    "test@example.com",
			},
			validate: func(t *testing.T, identifier string) {
				assert.NotEmpty(t, identifier)
				assert.Contains(t, identifier, "admin_role-", "identifier should contain snake_case role name")
				assert.Regexp(t, `^admin_role-[0-9a-f]{6}$`, identifier, "identifier should match pattern: snake_case_name-6hexchars")
			},
		},
		{
			name: "nil user generates identifier with empty user",
			role: models.Role{
				Identifier: "role-123",
				Name:       "Test Role",
			},
			user: nil,
			validate: func(t *testing.T, identifier string) {
				assert.NotEmpty(t, identifier)
				assert.Contains(t, identifier, "test_role-", "identifier should contain snake_case role name")
				assert.Regexp(t, `^test_role-[0-9a-f]{6}$`, identifier, "identifier should match pattern even with nil user")
			},
		},
		{
			name: "same role and user produces consistent identifier",
			role: models.Role{
				Identifier: "role-789",
				Name:       "Developer",
			},
			user: &models.User{
				ID:       "user-123",
				Username: "devuser",
				Email:    "dev@example.com",
			},
			validate: func(t *testing.T, identifier string) {
				// Generate identifier again with same inputs
				role2 := models.Role{
					Identifier: "role-789",
					Name:       "Developer",
				}
				user2 := &models.User{
					ID:       "user-123",
					Username: "devuser",
					Email:    "dev@example.com",
				}
				identifier2 := role2.GetUniqueIdentifier(user2)
				assert.Equal(t, identifier, identifier2, "same inputs should produce same identifier")
			},
		},
		{
			name: "different users produce different identifiers",
			role: models.Role{
				Identifier: "role-789",
				Name:       "Developer",
			},
			user: &models.User{
				ID:       "user-123",
				Username: "devuser1",
				Email:    "dev1@example.com",
			},
			validate: func(t *testing.T, identifier string) {
				// Generate identifier with different user
				user2 := &models.User{
					ID:       "user-456",
					Username: "devuser2",
					Email:    "dev2@example.com",
				}
				role2 := &models.Role{
					Identifier: "role-789",
					Name:       "Developer",
				}
				identifier2 := role2.GetUniqueIdentifier(user2)
				assert.NotEqual(t, identifier, identifier2, "different users should produce different identifiers")
			},
		},
		{
			name: "role with version includes version in hash",
			role: func() models.Role {
				v, _ := version.NewVersion("1.2.3")
				return models.Role{
					Identifier: "role-versioned",
					Name:       "Versioned Role",
					Version:    v,
				}
			}(),
			user: &models.User{
				ID:       "user-999",
				Username: "testuser",
				Email:    "test@example.com",
			},
			validate: func(t *testing.T, identifier string) {
				assert.NotEmpty(t, identifier)
				assert.Contains(t, identifier, "versioned_role-")

				// Compare with different version
				v2, _ := version.NewVersion("2.0.0")
				role2 := models.Role{
					Identifier: "role-versioned",
					Name:       "Versioned Role",
					Version:    v2,
				}
				identifier2 := role2.GetUniqueIdentifier(&models.User{
					ID:       "user-999",
					Username: "testuser",
					Email:    "test@example.com",
				})
				assert.NotEqual(t, identifier, identifier2, "different versions should produce different identifiers")
			},
		},
		{
			name: "special characters in role name convert to snake_case",
			role: models.Role{
				Identifier: "role-special",
				Name:       "My Special-Role_2024!",
			},
			user: &models.User{
				ID:    "user-001",
				Email: "test@example.com",
			},
			validate: func(t *testing.T, identifier string) {
				assert.NotEmpty(t, identifier)
				// ConvertToSnakeCase preserves allowed special chars like hyphen
				assert.Regexp(t, `^my_special-role_2024-[0-9a-f]{6}$`, identifier, "special characters should be normalized")
			},
		},
		{
			name: "identifier format is consistent",
			role: models.Role{
				Identifier: "test-id",
				Name:       "Test",
			},
			user: &models.User{
				ID: "user-1",
			},
			validate: func(t *testing.T, identifier string) {
				// Verify the hash portion is exactly 6 hex characters
				parts := len(identifier)
				assert.True(t, parts > 6, "identifier should be longer than 6 characters")
				hashPart := identifier[len(identifier)-6:]
				assert.Regexp(t, `^[0-9a-f]{6}$`, hashPart, "last 6 characters should be hex")
			},
		},
		{
			name: "empty role identifier still generates valid identifier",
			role: models.Role{
				Identifier: "",
				Name:       "Empty Identifier",
			},
			user: &models.User{
				ID: "user-1",
			},
			validate: func(t *testing.T, identifier string) {
				assert.NotEmpty(t, identifier)
				assert.Contains(t, identifier, "empty_identifier-")
			},
		},
		{
			name: "multiple consecutive calls produce identical output",
			role: models.Role{
				Identifier: "role-consistency",
				Name:       "Consistency Test",
			},
			user: &models.User{
				ID:       "user-consistent",
				Username: "consistent_user",
				Email:    "consistent@example.com",
			},
			validate: func(t *testing.T, identifier string) {
				// Call GetUniqueIdentifier multiple times consecutively
				role := &models.Role{
					Identifier: "role-consistency",
					Name:       "Consistency Test",
				}
				user := &models.User{
					ID:       "user-consistent",
					Username: "consistent_user",
					Email:    "consistent@example.com",
				}

				id1 := role.GetUniqueIdentifier(user)
				id2 := role.GetUniqueIdentifier(user)
				id3 := role.GetUniqueIdentifier(user)
				id4 := role.GetUniqueIdentifier(user)
				id5 := role.GetUniqueIdentifier(user)

				// All identifiers should be identical
				assert.Equal(t, id1, id2, "second call should produce same identifier")
				assert.Equal(t, id1, id3, "third call should produce same identifier")
				assert.Equal(t, id1, id4, "fourth call should produce same identifier")
				assert.Equal(t, id1, id5, "fifth call should produce same identifier")
				assert.Equal(t, identifier, id1, "should match first identifier from test")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identifier := (&tt.role).GetUniqueIdentifier(tt.user)

			if tt.expected != "" {
				assert.Equal(t, tt.expected, identifier)
			}

			if tt.validate != nil {
				tt.validate(t, identifier)
			}
		})
	}
}
