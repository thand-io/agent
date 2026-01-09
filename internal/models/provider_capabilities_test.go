package models_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/thand-io/agent/internal/models"
)

func TestBaseProvider_HasCapability(t *testing.T) {
	tests := []struct {
		name      string
		setupCaps func(*models.ProviderCapabilities)
		checkCap  models.ProviderCapability
		expected  bool
	}{
		// Roles
		{
			name: "Roles enabled",
			setupCaps: func(pc *models.ProviderCapabilities) {
				pc.Roles = &models.RolesConfiguration{}
				pc.Roles.Enabled = true
				pc.Roles.Synchronizable = true
			},
			checkCap: models.ProviderCapabilityRoles,
			expected: true,
		},
		{
			name: "Roles disabled",
			setupCaps: func(pc *models.ProviderCapabilities) {
				pc.Roles = &models.RolesConfiguration{}
				pc.Roles.Enabled = false
			},
			checkCap: models.ProviderCapabilityRoles,
			expected: false,
		},

		// Permissions
		{
			name: "Permissions enabled",
			setupCaps: func(pc *models.ProviderCapabilities) {
				pc.Permissions = &models.PermissionsConfiguration{}
				pc.Permissions.Enabled = true
				pc.Permissions.Synchronizable = true
			},
			checkCap: models.ProviderCapabilityPermissions,
			expected: true,
		},

		// Resources
		{
			name: "Resources enabled",
			setupCaps: func(pc *models.ProviderCapabilities) {
				pc.Resources = &models.ResourcesConfiguration{}
				pc.Resources.Enabled = true
				pc.Resources.Synchronizable = true
			},
			checkCap: models.ProviderCapabilityResources,
			expected: true,
		},

		// Identities
		{
			name: "Identities enabled",
			setupCaps: func(pc *models.ProviderCapabilities) {
				pc.Identities = &models.IdentitiesConfiguration{}
				pc.Identities.Enabled = true
				pc.Identities.Synchronizable = true
			},
			checkCap: models.ProviderCapabilityIdentities,
			expected: true,
		},

		// Users
		{
			name: "Users enabled",
			setupCaps: func(pc *models.ProviderCapabilities) {
				pc.Users = &models.UsersConfiguration{}
				pc.Users.Enabled = true
				pc.Users.Synchronizable = true
			},
			checkCap: models.ProviderCapabilityUsers,
			expected: true,
		},

		// Groups
		{
			name: "Groups enabled",
			setupCaps: func(pc *models.ProviderCapabilities) {
				pc.Groups = &models.GroupsConfiguration{}
				pc.Groups.Enabled = true
				pc.Groups.Synchronizable = true
			},
			checkCap: models.ProviderCapabilityGroups,
			expected: true,
		},

		// Authorizer
		{
			name: "Authorizer enabled",
			setupCaps: func(pc *models.ProviderCapabilities) {
				pc.Authorizer = &models.AuthorizerConfiguration{}
				pc.Authorizer.Enabled = true
			},
			checkCap: models.ProviderCapabilityAuthorizer,
			expected: true,
		},

		// Notifier
		{
			name: "Notifier enabled",
			setupCaps: func(pc *models.ProviderCapabilities) {
				pc.Notifier = &models.NotifierConfiguration{}
				pc.Notifier.Enabled = true
			},
			checkCap: models.ProviderCapabilityNotifier,
			expected: true,
		},

		// Provisioning
		{
			name: "AuthorizeRole enabled",
			setupCaps: func(pc *models.ProviderCapabilities) {
				pc.Provisioning = &models.ProvisioningConfiguration{}
				pc.Provisioning.Enabled = true
			},
			checkCap: models.ProviderCapabilityProvisioning,
			expected: true,
		},

		// Unknown capability
		{
			name: "Unknown capability",
			setupCaps: func(pc *models.ProviderCapabilities) {
				pc.Roles = &models.RolesConfiguration{}
				pc.Roles.Enabled = true
			},
			checkCap: models.ProviderCapability("unknown"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := &models.ProviderCapabilities{}
			if tt.setupCaps != nil {
				tt.setupCaps(caps)
			}
			p := models.NewBaseProvider(
				"test-provider",
				models.ProviderConfig{Name: "Test Provider"},
				caps,
			)
			result := p.HasCapability(tt.checkCap)
			assert.Equal(t, tt.expected, result, "HasCapability(%s) returned unexpected result", tt.checkCap)
		})
	}
}

// Helper to create fully initialized capabilities for testing Enable/Disable
func newInitializedCapabilities() *models.ProviderCapabilities {
	return &models.ProviderCapabilities{
		Roles:        &models.RolesConfiguration{},
		Permissions:  &models.PermissionsConfiguration{},
		Resources:    &models.ResourcesConfiguration{},
		Identities:   &models.IdentitiesConfiguration{},
		Users:        &models.UsersConfiguration{},
		Groups:       &models.GroupsConfiguration{},
		Authorizer:   &models.AuthorizerConfiguration{},
		Notifier:     &models.NotifierConfiguration{},
		Provisioning: &models.ProvisioningConfiguration{},
	}
}

func TestBaseProvider_EnableCapability(t *testing.T) {
	tests := []struct {
		name            string
		capabilityToAdd models.ProviderCapability
	}{
		{"Roles", models.ProviderCapabilityRoles},
		{"Permissions", models.ProviderCapabilityPermissions},
		{"Resources", models.ProviderCapabilityResources},
		{"Identities", models.ProviderCapabilityIdentities},
		{"Users", models.ProviderCapabilityUsers},
		{"Groups", models.ProviderCapabilityGroups},
		{"Authorizer", models.ProviderCapabilityAuthorizer},
		{"Notifier", models.ProviderCapabilityNotifier},
		{"Provisioning", models.ProviderCapabilityProvisioning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := models.NewBaseProvider(
				"test-provider",
				models.ProviderConfig{Name: "Test Provider"},
				newInitializedCapabilities(),
			)

			// Ensure it's disabled initially
			// (default bool is false, so Enabled=false)

			p.EnableCapability(tt.capabilityToAdd)

			assert.True(t, p.HasCapability(tt.capabilityToAdd),
				"Expected to have capability %s after enabling", tt.capabilityToAdd)
		})
	}
}

func TestBaseProvider_DisableCapability(t *testing.T) {
	tests := []struct {
		name               string
		capabilityToRemove models.ProviderCapability
	}{
		{"Roles", models.ProviderCapabilityRoles},
		{"Permissions", models.ProviderCapabilityPermissions},
		{"Resources", models.ProviderCapabilityResources},
		{"Identities", models.ProviderCapabilityIdentities},
		{"Users", models.ProviderCapabilityUsers},
		{"Groups", models.ProviderCapabilityGroups},
		{"Authorizer", models.ProviderCapabilityAuthorizer},
		{"Notifier", models.ProviderCapabilityNotifier},
		{"Provisioning", models.ProviderCapabilityProvisioning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := models.NewBaseProvider(
				"test-provider",
				models.ProviderConfig{Name: "Test Provider"},
				newInitializedCapabilities(),
			)

			// Enable it first
			p.EnableCapability(tt.capabilityToRemove)
			assert.True(t, p.HasCapability(tt.capabilityToRemove), "Setup: failed to enable capability")

			// Disable it
			p.DisableCapability(tt.capabilityToRemove)

			assert.False(t, p.HasCapability(tt.capabilityToRemove),
				"Expected to NOT have capability %s after disabling", tt.capabilityToRemove)
		})
	}
}

func TestBaseProvider_EnableDisable_Idempotency(t *testing.T) {
	p := models.NewBaseProvider(
		"test-provider",
		models.ProviderConfig{Name: "Test Provider"},
		newInitializedCapabilities(),
	)

	cap := models.ProviderCapabilityRoles
	// Enable twice
	p.EnableCapability(cap)
	p.EnableCapability(cap)
	assert.True(t, p.HasCapability(cap))

	// Disable twice
	p.DisableCapability(cap)
	p.DisableCapability(cap)
	assert.False(t, p.HasCapability(cap))
}

func TestBaseProvider_HasAnyCapability(t *testing.T) {
	tests := []struct {
		name      string
		setupCaps func(*models.ProviderCapabilities)
		checkCaps []models.ProviderCapability
		expected  bool
	}{
		{
			name: "Has one of the capabilities",
			setupCaps: func(pc *models.ProviderCapabilities) {
				pc.Roles = &models.RolesConfiguration{}
				pc.Roles.Enabled = true
				pc.Roles.Synchronizable = true

				pc.Resources = &models.ResourcesConfiguration{}
				// Resources disabled by default
			},
			checkCaps: []models.ProviderCapability{models.ProviderCapabilityRoles, models.ProviderCapabilityResources},
			expected:  true,
		},
		{
			name: "Has none of the capabilities",
			setupCaps: func(pc *models.ProviderCapabilities) {
				pc.Roles = &models.RolesConfiguration{}
				pc.Roles.Enabled = false

				pc.Resources = &models.ResourcesConfiguration{}
				pc.Resources.Enabled = false
			},
			checkCaps: []models.ProviderCapability{models.ProviderCapabilityRoles, models.ProviderCapabilityResources},
			expected:  false,
		},
		{
			name: "Empty capabilities list",
			setupCaps: func(pc *models.ProviderCapabilities) {
				pc.Roles = &models.RolesConfiguration{}
				pc.Roles.Enabled = true
			},
			checkCaps: []models.ProviderCapability{},
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := &models.ProviderCapabilities{}
			if tt.setupCaps != nil {
				tt.setupCaps(caps)
			}
			p := models.NewBaseProvider(
				"test-provider",
				models.ProviderConfig{Name: "Test Provider"},
				caps,
			)
			result := p.HasAnyCapability(tt.checkCaps...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBaseProvider_DisableCapabilities(t *testing.T) {
	p := models.NewBaseProvider(
		"test-provider",
		models.ProviderConfig{Name: "Test Provider"},
		newInitializedCapabilities(),
	)

	// Enable multiple capabilities
	p.EnableCapability(models.ProviderCapabilityRoles)
	p.EnableCapability(models.ProviderCapabilityResources)

	assert.True(t, p.HasCapability(models.ProviderCapabilityRoles))
	assert.True(t, p.HasCapability(models.ProviderCapabilityResources))
	// Disable them
	p.DisableCapabilities(models.ProviderCapabilityRoles, models.ProviderCapabilityResources)

	assert.False(t, p.HasCapability(models.ProviderCapabilityRoles))
	assert.False(t, p.HasCapability(models.ProviderCapabilityResources))
}

func TestBaseProvider_CapabilityConfigurationOverrides(t *testing.T) {
	t.Run("Cannot enable capability unsupported by provider", func(t *testing.T) {
		// Simulate a provider that does not support Roles (struct is nil)
		caps := &models.ProviderCapabilities{
			Roles: nil,
			Permissions: &models.PermissionsConfiguration{
				Enabled: true,
			},
		}
		p := models.NewBaseProvider(
			"test-provider",
			models.ProviderConfig{Name: "Test Provider"},
			caps,
		)
		// Attempt to enable an unsupported capability
		p.EnableCapability(models.ProviderCapabilityRoles)

		assert.False(t, p.HasCapability(models.ProviderCapabilityRoles),
			"Should not be able to enable a capability that the provider does not support (nil config)")
	})

	t.Run("Override synchronization settings", func(t *testing.T) {
		caps := &models.ProviderCapabilities{
			Roles: &models.RolesConfiguration{
				Enabled:        true,
				Synchronizable: true,
				Interval:       1,
			},
		}
		p := models.NewBaseProvider(
			"test-provider",
			models.ProviderConfig{Name: "Test Provider"},
			caps,
		)

		// Verify initial state
		assert.True(t, p.HasCapability(models.ProviderCapabilityRoles))
		assert.True(t, caps.Roles.Synchronizable)

		// Change interval for synchronizable task
		caps.Roles.Interval = 10
		assert.Equal(t, 10, caps.Roles.Interval)

		// Disable synchronization for the task
		caps.Roles.Synchronizable = false
		assert.False(t, caps.Roles.Synchronizable)

		// Disable the capability entirely (override enabled -> disabled)
		p.DisableCapability(models.ProviderCapabilityRoles)
		assert.False(t, p.HasCapability(models.ProviderCapabilityRoles))
	})
}

func TestProviderCapabilities_Update(t *testing.T) {
	t.Run("Update enabled capability", func(t *testing.T) {
		pc := models.NewProviderCapabilities()
		pc.Roles.Enabled = true
		pc.Roles.Synchronizable = true
		pc.Roles.Interval = 60

		updates := &models.ProviderCapabilities{
			Roles: &models.RolesConfiguration{
				Enabled:        false, // Disable it
				Synchronizable: false, // Disable sync
				Interval:       30,    // Change interval
			},
		}

		pc.Update(updates)

		assert.False(t, pc.Roles.Enabled)
		assert.False(t, pc.Roles.Synchronizable)
		assert.Equal(t, 30, pc.Roles.Interval)
	})

	t.Run("Cannot enable disabled capability", func(t *testing.T) {
		pc := models.NewProviderCapabilities()
		pc.Roles.Enabled = false

		updates := &models.ProviderCapabilities{
			Roles: &models.RolesConfiguration{
				Enabled: true,
			},
		}

		pc.Update(updates)

		assert.False(t, pc.Roles.Enabled, "Should not be able to enable a disabled capability")
	})

	t.Run("Update interval only (disables because enabled=false default)", func(t *testing.T) {
		pc := models.NewProviderCapabilities()
		pc.Roles.Enabled = true

		updates := &models.ProviderCapabilities{
			Roles: &models.RolesConfiguration{
				Interval: 30,
			},
		}

		pc.Update(updates)

		assert.Equal(t, 30, pc.Roles.Interval)
		assert.False(t, pc.Roles.Enabled, "Implicitly disabled because enabled was missing/false in update")
	})

	t.Run("Update non-synchronizable capability", func(t *testing.T) {
		pc := models.NewProviderCapabilities()
		pc.Authorizer.Enabled = true

		updates := &models.ProviderCapabilities{
			Authorizer: &models.AuthorizerConfiguration{
				Enabled: false,
			},
		}

		pc.Update(updates)
		assert.False(t, pc.Authorizer.Enabled)
	})

	t.Run("Ignore update for nil capability in base", func(t *testing.T) {
		pc := models.NewProviderCapabilities()
		pc.Roles = nil // Simulate unsupported capability

		updates := &models.ProviderCapabilities{
			Roles: &models.RolesConfiguration{
				Enabled: true,
			},
		}

		pc.Update(updates)
		assert.Nil(t, pc.Roles)
	})

	t.Run("Update multiple capabilities", func(t *testing.T) {
		pc := models.NewProviderCapabilities()
		pc.Roles.Enabled = true
		pc.Users.Enabled = true
		pc.Users.Interval = 60

		updates := &models.ProviderCapabilities{
			Roles: &models.RolesConfiguration{Enabled: false},
			Users: &models.UsersConfiguration{Interval: 120, Enabled: true},
		}

		pc.Update(updates)
		assert.False(t, pc.Roles.Enabled)
		assert.True(t, pc.Users.Enabled)
		assert.Equal(t, 120, pc.Users.Interval)
	})

	t.Run("Update synchronization state", func(t *testing.T) {
		pc := models.NewProviderCapabilities()
		pc.Roles.Enabled = true
		pc.Roles.Synchronizable = true

		// Disable sync
		updates := &models.ProviderCapabilities{
			Roles: &models.RolesConfiguration{
				Enabled:        true,
				Synchronizable: false,
			},
		}
		pc.Update(updates)
		assert.False(t, pc.Roles.Synchronizable)
		assert.True(t, pc.Roles.Enabled)

		// Enable sync
		updates2 := &models.ProviderCapabilities{
			Roles: &models.RolesConfiguration{
				Enabled:        true,
				Synchronizable: true,
			},
		}
		pc.Update(updates2)
		assert.True(t, pc.Roles.Synchronizable)
	})
}
