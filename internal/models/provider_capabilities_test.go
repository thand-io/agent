package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBaseProvider_HasCapability(t *testing.T) {
	tests := []struct {
		name      string
		setupCaps func(*ProviderCapabilities)
		checkCap  ProviderCapability
		expected  bool
	}{
		// Roles
		{
			name: "Roles enabled",
			setupCaps: func(pc *ProviderCapabilities) {
				pc.Roles = &RolesConfiguration{}
				pc.Roles.Enabled = true
				pc.Roles.Synchronizable = true
			},
			checkCap: ProviderCapabilityRoles,
			expected: true,
		},
		{
			name: "Roles disabled",
			setupCaps: func(pc *ProviderCapabilities) {
				pc.Roles = &RolesConfiguration{}
				pc.Roles.Enabled = false
			},
			checkCap: ProviderCapabilityRoles,
			expected: false,
		},

		// Permissions
		{
			name: "Permissions enabled",
			setupCaps: func(pc *ProviderCapabilities) {
				pc.Permissions = &PermissionsConfiguration{}
				pc.Permissions.Enabled = true
				pc.Permissions.Synchronizable = true
			},
			checkCap: ProviderCapabilityPermissions,
			expected: true,
		},

		// Resources
		{
			name: "Resources enabled",
			setupCaps: func(pc *ProviderCapabilities) {
				pc.Resources = &ResourcesConfiguration{}
				pc.Resources.Enabled = true
				pc.Resources.Synchronizable = true
			},
			checkCap: ProviderCapabilityResources,
			expected: true,
		},

		// Identities
		{
			name: "Identities enabled",
			setupCaps: func(pc *ProviderCapabilities) {
				pc.Identities = &IdentitiesConfiguration{}
				pc.Identities.Enabled = true
				pc.Identities.Synchronizable = true
			},
			checkCap: ProviderCapabilityIdentities,
			expected: true,
		},

		// Users
		{
			name: "Users enabled",
			setupCaps: func(pc *ProviderCapabilities) {
				pc.Users = &UsersConfiguration{}
				pc.Users.Enabled = true
				pc.Users.Synchronizable = true
			},
			checkCap: ProviderCapabilityUsers,
			expected: true,
		},

		// Groups
		{
			name: "Groups enabled",
			setupCaps: func(pc *ProviderCapabilities) {
				pc.Groups = &GroupsConfiguration{}
				pc.Groups.Enabled = true
				pc.Groups.Synchronizable = true
			},
			checkCap: ProviderCapabilityGroups,
			expected: true,
		},

		// Authorizer
		{
			name: "Authorizer enabled",
			setupCaps: func(pc *ProviderCapabilities) {
				pc.Authorizer = &AuthorizerConfiguration{}
				pc.Authorizer.Enabled = true
			},
			checkCap: ProviderCapabilityAuthorizer,
			expected: true,
		},

		// Notifier
		{
			name: "Notifier enabled",
			setupCaps: func(pc *ProviderCapabilities) {
				pc.Notifier = &NotifierConfiguration{}
				pc.Notifier.Enabled = true
			},
			checkCap: ProviderCapabilityNotifier,
			expected: true,
		},

		// Provisioning
		{
			name: "AuthorizeRole enabled",
			setupCaps: func(pc *ProviderCapabilities) {
				pc.Provisioning = &ProviderConfiguration{}
				pc.Provisioning.Enabled = true
			},
			checkCap: ProviderCapabilityProvisioning,
			expected: true,
		},

		// Unknown capability
		{
			name: "Unknown capability",
			setupCaps: func(pc *ProviderCapabilities) {
				pc.Roles = &RolesConfiguration{}
				pc.Roles.Enabled = true
			},
			checkCap: ProviderCapability("unknown"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := &ProviderCapabilities{}
			if tt.setupCaps != nil {
				tt.setupCaps(caps)
			}
			p := &BaseProvider{
				capabilities: caps,
			}
			result := p.HasCapability(tt.checkCap)
			assert.Equal(t, tt.expected, result, "HasCapability(%s) returned unexpected result", tt.checkCap)
		})
	}
}

// Helper to create fully initialized capabilities for testing Enable/Disable
func newInitializedCapabilities() *ProviderCapabilities {
	return &ProviderCapabilities{
		Roles:        &RolesConfiguration{},
		Permissions:  &PermissionsConfiguration{},
		Resources:    &ResourcesConfiguration{},
		Identities:   &IdentitiesConfiguration{},
		Users:        &UsersConfiguration{},
		Groups:       &GroupsConfiguration{},
		Authorizer:   &AuthorizerConfiguration{},
		Notifier:     &NotifierConfiguration{},
		Provisioning: &ProvisioningConfiguration{},
	}
}

func TestBaseProvider_EnableCapability(t *testing.T) {
	tests := []struct {
		name            string
		capabilityToAdd ProviderCapability
	}{
		{"Roles", ProviderCapabilityRoles},
		{"Permissions", ProviderCapabilityPermissions},
		{"Resources", ProviderCapabilityResources},
		{"Identities", ProviderCapabilityIdentities},
		{"Users", ProviderCapabilityUsers},
		{"Groups", ProviderCapabilityGroups},
		{"Authorizer", ProviderCapabilityAuthorizer},
		{"Notifier", ProviderCapabilityNotifier},
		{"Provisioning", ProviderCapabilityProvisioning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &BaseProvider{
				capabilities: newInitializedCapabilities(),
			}

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
		capabilityToRemove ProviderCapability
	}{
		{"Roles", ProviderCapabilityRoles},
		{"Permissions", ProviderCapabilityPermissions},
		{"Resources", ProviderCapabilityResources},
		{"Identities", ProviderCapabilityIdentities},
		{"Users", ProviderCapabilityUsers},
		{"Groups", ProviderCapabilityGroups},
		{"Authorizer", ProviderCapabilityAuthorizer},
		{"Notifier", ProviderCapabilityNotifier},
		{"Provisioning", ProviderCapabilityProvisioning},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &BaseProvider{
				capabilities: newInitializedCapabilities(),
			}

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
	p := &BaseProvider{
		capabilities: newInitializedCapabilities(),
	}

	cap := ProviderCapabilityRoles

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
		setupCaps func(*ProviderCapabilities)
		checkCaps []ProviderCapability
		expected  bool
	}{
		{
			name: "Has one of the capabilities",
			setupCaps: func(pc *ProviderCapabilities) {
				pc.Roles = &RolesConfiguration{}
				pc.Roles.Enabled = true
				pc.Roles.Synchronizable = true

				pc.Resources = &ResourcesConfiguration{}
				// Resources disabled by default
			},
			checkCaps: []ProviderCapability{ProviderCapabilityRoles, ProviderCapabilityResources},
			expected:  true,
		},
		{
			name: "Has none of the capabilities",
			setupCaps: func(pc *ProviderCapabilities) {
				pc.Roles = &RolesConfiguration{}
				pc.Roles.Enabled = false

				pc.Resources = &ResourcesConfiguration{}
				pc.Resources.Enabled = false
			},
			checkCaps: []ProviderCapability{ProviderCapabilityRoles, ProviderCapabilityResources},
			expected:  false,
		},
		{
			name: "Empty capabilities list",
			setupCaps: func(pc *ProviderCapabilities) {
				pc.Roles = &RolesConfiguration{}
				pc.Roles.Enabled = true
			},
			checkCaps: []ProviderCapability{},
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := &ProviderCapabilities{}
			if tt.setupCaps != nil {
				tt.setupCaps(caps)
			}
			p := &BaseProvider{
				capabilities: caps,
			}
			result := p.HasAnyCapability(tt.checkCaps...)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestBaseProvider_DisableCapabilities(t *testing.T) {
	p := &BaseProvider{
		capabilities: newInitializedCapabilities(),
	}

	// Enable multiple capabilities
	p.EnableCapability(ProviderCapabilityRoles)
	p.EnableCapability(ProviderCapabilityResources)

	assert.True(t, p.HasCapability(ProviderCapabilityRoles))
	assert.True(t, p.HasCapability(ProviderCapabilityResources))

	// Disable them
	p.DisableCapabilities(ProviderCapabilityRoles, ProviderCapabilityResources)

	assert.False(t, p.HasCapability(ProviderCapabilityRoles))
	assert.False(t, p.HasCapability(ProviderCapabilityResources))
}

func TestBaseProvider_CapabilityConfigurationOverrides(t *testing.T) {
	t.Run("Cannot enable capability unsupported by provider", func(t *testing.T) {
		// Simulate a provider that does not support Roles (struct is nil)
		caps := &ProviderCapabilities{
			Roles: nil,
			Permissions: &PermissionsConfiguration{
				Enabled: true,
			},
		}
		p := &BaseProvider{capabilities: caps}

		// Attempt to enable an unsupported capability
		p.EnableCapability(ProviderCapabilityRoles)

		assert.False(t, p.HasCapability(ProviderCapabilityRoles),
			"Should not be able to enable a capability that the provider does not support (nil config)")
	})

	t.Run("Override synchronization settings", func(t *testing.T) {
		caps := &ProviderCapabilities{
			Roles: &RolesConfiguration{
				Enabled:        true,
				Synchronizable: true,
				Interval:       1,
			},
		}
		p := &BaseProvider{capabilities: caps}

		// Verify initial state
		assert.True(t, p.HasCapability(ProviderCapabilityRoles))
		assert.True(t, caps.Roles.Synchronizable)

		// Change interval for synchronizable task
		caps.Roles.Interval = 10
		assert.Equal(t, 10, caps.Roles.Interval)

		// Disable synchronization for the task
		caps.Roles.Synchronizable = false
		assert.False(t, caps.Roles.Synchronizable)

		// Disable the capability entirely (override enabled -> disabled)
		p.DisableCapability(ProviderCapabilityRoles)
		assert.False(t, p.HasCapability(ProviderCapabilityRoles))
	})
}

func TestProviderCapabilities_Update(t *testing.T) {
	t.Run("Update enabled capability", func(t *testing.T) {
		pc := NewProviderCapabilities()
		pc.Roles.Enabled = true
		pc.Roles.Synchronizable = true
		pc.Roles.Interval = 60

		updates := &ProviderCapabilities{
			Roles: &RolesConfiguration{
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
		pc := NewProviderCapabilities()
		pc.Roles.Enabled = false

		updates := &ProviderCapabilities{
			Roles: &RolesConfiguration{
				Enabled: true,
			},
		}

		pc.Update(updates)

		assert.False(t, pc.Roles.Enabled, "Should not be able to enable a disabled capability")
	})

	t.Run("Update interval only (disables because enabled=false default)", func(t *testing.T) {
		pc := NewProviderCapabilities()
		pc.Roles.Enabled = true

		updates := &ProviderCapabilities{
			Roles: &RolesConfiguration{
				Interval: 30,
			},
		}

		pc.Update(updates)

		assert.Equal(t, 30, pc.Roles.Interval)
		assert.False(t, pc.Roles.Enabled, "Implicitly disabled because enabled was missing/false in update")
	})

	t.Run("Update non-synchronizable capability", func(t *testing.T) {
		pc := NewProviderCapabilities()
		pc.Authorizer.Enabled = true

		updates := &ProviderCapabilities{
			Authorizer: &AuthorizerConfiguration{
				Enabled: false,
			},
		}

		pc.Update(updates)
		assert.False(t, pc.Authorizer.Enabled)
	})

	t.Run("Ignore update for nil capability in base", func(t *testing.T) {
		pc := NewProviderCapabilities()
		pc.Roles = nil // Simulate unsupported capability

		updates := &ProviderCapabilities{
			Roles: &RolesConfiguration{
				Enabled: true,
			},
		}

		pc.Update(updates)
		assert.Nil(t, pc.Roles)
	})

	t.Run("Update multiple capabilities", func(t *testing.T) {
		pc := NewProviderCapabilities()
		pc.Roles.Enabled = true
		pc.Users.Enabled = true
		pc.Users.Interval = 60

		updates := &ProviderCapabilities{
			Roles: &RolesConfiguration{Enabled: false},
			Users: &UsersConfiguration{Interval: 120, Enabled: true},
		}

		pc.Update(updates)
		assert.False(t, pc.Roles.Enabled)
		assert.True(t, pc.Users.Enabled)
		assert.Equal(t, 120, pc.Users.Interval)
	})

	t.Run("Update synchronization state", func(t *testing.T) {
		pc := NewProviderCapabilities()
		pc.Roles.Enabled = true
		pc.Roles.Synchronizable = true

		// Disable sync
		updates := &ProviderCapabilities{
			Roles: &RolesConfiguration{
				Enabled:        true,
				Synchronizable: false,
			},
		}
		pc.Update(updates)
		assert.False(t, pc.Roles.Synchronizable)
		assert.True(t, pc.Roles.Enabled)

		// Enable sync
		updates2 := &ProviderCapabilities{
			Roles: &RolesConfiguration{
				Enabled:        true,
				Synchronizable: true,
			},
		}
		pc.Update(updates2)
		assert.True(t, pc.Roles.Synchronizable)
	})
}
