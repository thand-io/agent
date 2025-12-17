package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBaseProvider_HasCapability(t *testing.T) {
	tests := []struct {
		name      string
		setupCaps func(*ProviderCapabilties)
		checkCap  ProviderCapability
		expected  bool
	}{
		// Roles
		{
			name: "Roles enabled",
			setupCaps: func(pc *ProviderCapabilties) {
				pc.Roles = &RolesConfiguration{}
				pc.Roles.Enabled = true
				pc.Roles.Synchronizable = true
			},
			checkCap: ProviderCapabilityRoles,
			expected: true,
		},
		{
			name: "Roles disabled",
			setupCaps: func(pc *ProviderCapabilties) {
				pc.Roles = &RolesConfiguration{}
				pc.Roles.Enabled = false
			},
			checkCap: ProviderCapabilityRoles,
			expected: false,
		},

		// Permissions
		{
			name: "Permissions enabled",
			setupCaps: func(pc *ProviderCapabilties) {
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
			setupCaps: func(pc *ProviderCapabilties) {
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
			setupCaps: func(pc *ProviderCapabilties) {
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
			setupCaps: func(pc *ProviderCapabilties) {
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
			setupCaps: func(pc *ProviderCapabilties) {
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
			setupCaps: func(pc *ProviderCapabilties) {
				pc.Authorizer = &AuthorizerConfiguration{}
				pc.Authorizer.Enabled = true
			},
			checkCap: ProviderCapabilityAuthorizer,
			expected: true,
		},

		// Notifier
		{
			name: "Notifier enabled",
			setupCaps: func(pc *ProviderCapabilties) {
				pc.Notifier = &NotifierConfiguration{}
				pc.Notifier.Enabled = true
			},
			checkCap: ProviderCapabilityNotifier,
			expected: true,
		},

		// AuthorizeRole
		{
			name: "AuthorizeRole enabled",
			setupCaps: func(pc *ProviderCapabilties) {
				pc.AuthorizeRole = &AuthorizeRoleConfiguration{}
				pc.AuthorizeRole.Enabled = true
			},
			checkCap: ProviderCapabilityAuthorizeRole,
			expected: true,
		},

		// RevokeRole
		{
			name: "RevokeRole enabled",
			setupCaps: func(pc *ProviderCapabilties) {
				pc.RevokeRole = &RevokeRoleConfiguration{}
				pc.RevokeRole.Enabled = true
			},
			checkCap: ProviderCapabilityRevokeRole,
			expected: true,
		},

		// Unknown capability
		{
			name: "Unknown capability",
			setupCaps: func(pc *ProviderCapabilties) {
				pc.Roles = &RolesConfiguration{}
				pc.Roles.Enabled = true
			},
			checkCap: ProviderCapability("unknown"),
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := &ProviderCapabilties{}
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
func newInitializedCapabilities() *ProviderCapabilties {
	return &ProviderCapabilties{
		Roles:         &RolesConfiguration{},
		Permissions:   &PermissionsConfiguration{},
		Resources:     &ResourcesConfiguration{},
		Identities:    &IdentitiesConfiguration{},
		Users:         &UsersConfiguration{},
		Groups:        &GroupsConfiguration{},
		Authorizer:    &AuthorizerConfiguration{},
		Notifier:      &NotifierConfiguration{},
		AuthorizeRole: &AuthorizeRoleConfiguration{},
		RevokeRole:    &RevokeRoleConfiguration{},
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
		{"AuthorizeRole", ProviderCapabilityAuthorizeRole},
		{"RevokeRole", ProviderCapabilityRevokeRole},
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
		{"AuthorizeRole", ProviderCapabilityAuthorizeRole},
		{"RevokeRole", ProviderCapabilityRevokeRole},
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
		setupCaps func(*ProviderCapabilties)
		checkCaps []ProviderCapability
		expected  bool
	}{
		{
			name: "Has one of the capabilities",
			setupCaps: func(pc *ProviderCapabilties) {
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
			setupCaps: func(pc *ProviderCapabilties) {
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
			setupCaps: func(pc *ProviderCapabilties) {
				pc.Roles = &RolesConfiguration{}
				pc.Roles.Enabled = true
			},
			checkCaps: []ProviderCapability{},
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			caps := &ProviderCapabilties{}
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
