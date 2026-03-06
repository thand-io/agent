package config

import (
	"testing"

	"github.com/hashicorp/go-version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

// TestApplyRoles_KeyBecomesIdentifier verifies that the YAML map key used in a
// RoleDefinitions entry is assigned as the Identifier on the resulting Role
// and is not modified.
func TestApplyRoles_KeyBecomesIdentifier(t *testing.T) {
	cfg := &Config{mode: ModeServer}

	yamlKey := "engineer-readonly"
	defs := []*models.RoleDefinitions{
		{
			Version: version.Must(version.NewVersion("1.0.0")),
			Roles: map[string]models.Role{
				yamlKey: {
					Name:    "Engineer Read Only",
					Enabled: true,
				},
			},
		},
	}

	result, err := cfg.ApplyRoles(defs)
	require.NoError(t, err)
	require.Len(t, result, 1, "expected exactly one role")

	role, exists := result[yamlKey]
	require.True(t, exists, "expected the map key to be %q", yamlKey)
	assert.Equal(t, yamlKey, role.Identifier, "Role.Identifier must equal the YAML key")
}
