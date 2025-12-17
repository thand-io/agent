package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBaseProvider_HasCapability(t *testing.T) {
	tests := []struct {
		name         string
		capabilities []ProviderCapability
		checkCap     ProviderCapability
		expected     bool
	}{
		// Direct capability checks
		{
			name:         "Direct capability present",
			capabilities: []ProviderCapability{ProviderCapabilitySynchronizeRoles},
			checkCap:     ProviderCapabilitySynchronizeRoles,
			expected:     true,
		},
		{
			name:         "Direct capability not present",
			capabilities: []ProviderCapability{ProviderCapabilitySynchronizeRoles},
			checkCap:     ProviderCapabilitySynchronizePermissions,
			expected:     false,
		},

		// RBAC capability checks (requires AuthorizeRole + RevokeRole + at least one optional)
		{
			name: "RBAC with all required and one optional",
			capabilities: []ProviderCapability{
				ProviderCapabilityAuthorizeRole,
				ProviderCapabilityRevokeRole,
				ProviderCapabilitySynchronizeRoles,
			},
			checkCap: ProviderCapabilityRBAC,
			expected: true,
		},
		{
			name: "RBAC with all required and multiple optional",
			capabilities: []ProviderCapability{
				ProviderCapabilityAuthorizeRole,
				ProviderCapabilityRevokeRole,
				ProviderCapabilitySynchronizeRoles,
				ProviderCapabilitySynchronizePermissions,
			},
			checkCap: ProviderCapabilityRBAC,
			expected: true,
		},
		{
			name: "RBAC missing required authorize_role",
			capabilities: []ProviderCapability{
				ProviderCapabilityRevokeRole,
				ProviderCapabilitySynchronizeRoles,
			},
			checkCap: ProviderCapabilityRBAC,
			expected: false,
		},
		{
			name: "RBAC missing required revoke_role",
			capabilities: []ProviderCapability{
				ProviderCapabilityAuthorizeRole,
				ProviderCapabilitySynchronizeRoles,
			},
			checkCap: ProviderCapabilityRBAC,
			expected: false,
		},
		{
			name: "RBAC missing all required but has optional",
			capabilities: []ProviderCapability{
				ProviderCapabilitySynchronizeRoles,
			},
			checkCap: ProviderCapabilityRBAC,
			expected: false,
		},
		{
			name: "RBAC has required but missing all optional",
			capabilities: []ProviderCapability{
				ProviderCapabilityAuthorizeRole,
				ProviderCapabilityRevokeRole,
			},
			checkCap: ProviderCapabilityRBAC,
			expected: false,
		},

		// Authorizer capability checks (requires AuthorizeSession)
		{
			name:         "Authorizer with required capability",
			capabilities: []ProviderCapability{ProviderCapabilityAuthorizeSession},
			checkCap:     ProviderCapabilityAuthorizer,
			expected:     true,
		},
		{
			name:         "Authorizer missing required capability",
			capabilities: []ProviderCapability{ProviderCapabilitySendNotifications},
			checkCap:     ProviderCapabilityAuthorizer,
			expected:     false,
		},

		// Notifier capability checks (requires SendNotifications)
		{
			name:         "Notifier with required capability",
			capabilities: []ProviderCapability{ProviderCapabilitySendNotifications},
			checkCap:     ProviderCapabilityNotifier,
			expected:     true,
		},
		{
			name:         "Notifier missing required capability",
			capabilities: []ProviderCapability{ProviderCapabilityAuthorizeSession},
			checkCap:     ProviderCapabilityNotifier,
			expected:     false,
		},

		// Identities capability checks (requires at least one optional)
		{
			name:         "Identities with synchronize_identities",
			capabilities: []ProviderCapability{ProviderCapabilitySynchronizeIdentities},
			checkCap:     ProviderCapabilityIdentities,
			expected:     true,
		},
		{
			name:         "Identities with synchronize_users",
			capabilities: []ProviderCapability{ProviderCapabilitySynchronizeUsers},
			checkCap:     ProviderCapabilityIdentities,
			expected:     true,
		},
		{
			name:         "Identities with synchronize_groups",
			capabilities: []ProviderCapability{ProviderCapabilitySynchronizeGroups},
			checkCap:     ProviderCapabilityIdentities,
			expected:     true,
		},
		{
			name: "Identities with all optional capabilities",
			capabilities: []ProviderCapability{
				ProviderCapabilitySynchronizeIdentities,
				ProviderCapabilitySynchronizeUsers,
				ProviderCapabilitySynchronizeGroups,
			},
			checkCap: ProviderCapabilityIdentities,
			expected: true,
		},
		{
			name:         "Identities with no optional capabilities",
			capabilities: []ProviderCapability{ProviderCapabilityAuthorizeSession},
			checkCap:     ProviderCapabilityIdentities,
			expected:     false,
		},

		// Unknown capability
		{
			name:         "Unknown capability",
			capabilities: []ProviderCapability{ProviderCapabilitySynchronizeRoles},
			checkCap:     ProviderCapability("unknown"),
			expected:     false,
		},

		// Empty capabilities
		{
			name:         "Empty capabilities list",
			capabilities: []ProviderCapability{},
			checkCap:     ProviderCapabilityRBAC,
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &BaseProvider{
				capabilities: tt.capabilities,
			}
			result := p.HasCapability(tt.checkCap)
			assert.Equal(t, tt.expected, result, "HasCapability(%s) returned unexpected result", tt.checkCap)
		})
	}
}
func TestBaseProvider_EnableCapability_TopLevel(t *testing.T) {
	tests := []struct {
		name             string
		capabilityToAdd  ProviderCapability
		expectedCapNames []ProviderCapability
		shouldHaveCap    ProviderCapability
	}{
		{
			name:            "Enable RBAC adds all RBAC sub-capabilities",
			capabilityToAdd: ProviderCapabilityRBAC,
			expectedCapNames: []ProviderCapability{
				ProviderCapabilitySynchronizeRoles,
				ProviderCapabilitySynchronizePermissions,
				ProviderCapabilitySynchronizeResources,
				ProviderCapabilityAuthorizeRole,
				ProviderCapabilityRevokeRole,
			},
			shouldHaveCap: ProviderCapabilityRBAC,
		},
		{
			name:            "Enable Authorizer adds all Authorizer sub-capabilities",
			capabilityToAdd: ProviderCapabilityAuthorizer,
			expectedCapNames: []ProviderCapability{
				ProviderCapabilityAuthorizeSession,
			},
			shouldHaveCap: ProviderCapabilityAuthorizer,
		},
		{
			name:            "Enable Notifier adds all Notifier sub-capabilities",
			capabilityToAdd: ProviderCapabilityNotifier,
			expectedCapNames: []ProviderCapability{
				ProviderCapabilitySendNotifications,
			},
			shouldHaveCap: ProviderCapabilityNotifier,
		},
		{
			name:            "Enable Identities adds all Identities sub-capabilities",
			capabilityToAdd: ProviderCapabilityIdentities,
			expectedCapNames: []ProviderCapability{
				ProviderCapabilitySynchronizeIdentities,
				ProviderCapabilitySynchronizeUsers,
				ProviderCapabilitySynchronizeGroups,
			},
			shouldHaveCap: ProviderCapabilityIdentities,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &BaseProvider{
				capabilities: make([]ProviderCapability, 0),
			}

			p.EnableCapability(tt.capabilityToAdd)

			// Verify all expected sub-capabilities are present
			for _, expectedCap := range tt.expectedCapNames {
				assert.True(t, p.HasCapability(expectedCap),
					"Expected sub-capability %s to be present after enabling %s",
					expectedCap, tt.capabilityToAdd)
			}

			// Verify the top-level capability check works
			assert.True(t, p.HasCapability(tt.shouldHaveCap),
				"Expected to have capability %s", tt.shouldHaveCap)
		})
	}
}

func TestBaseProvider_EnableCapability_SubCapability(t *testing.T) {
	tests := []struct {
		name            string
		capabilityToAdd ProviderCapability
		shouldHaveCap   ProviderCapability
	}{
		{
			name:            "Enable individual sub-capability",
			capabilityToAdd: ProviderCapabilitySynchronizeRoles,
			shouldHaveCap:   ProviderCapabilitySynchronizeRoles,
		},
		{
			name:            "Enable another individual sub-capability",
			capabilityToAdd: ProviderCapabilityAuthorizeSession,
			shouldHaveCap:   ProviderCapabilityAuthorizeSession,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &BaseProvider{
				capabilities: make([]ProviderCapability, 0),
			}

			p.EnableCapability(tt.capabilityToAdd)

			assert.True(t, p.HasCapability(tt.shouldHaveCap),
				"Expected to have capability %s", tt.shouldHaveCap)
			assert.Len(t, p.capabilities, 1)
		})
	}
}

func TestBaseProvider_EnableCapability_NoDuplicates(t *testing.T) {
	p := &BaseProvider{
		capabilities: make([]ProviderCapability, 0),
	}

	// Enable RBAC first time
	p.EnableCapability(ProviderCapabilityRBAC)
	firstCount := len(p.capabilities)

	// Enable RBAC second time
	p.EnableCapability(ProviderCapabilityRBAC)
	secondCount := len(p.capabilities)

	assert.Equal(t, firstCount, secondCount, "Enabling same capability twice should not add duplicates")
	assert.True(t, p.HasCapability(ProviderCapabilityRBAC))
}

func TestBaseProvider_DisableCapability_TopLevel(t *testing.T) {
	tests := []struct {
		name                string
		capabilityToRemove  ProviderCapability
		initialCapabilities []ProviderCapability
		shouldNotHaveCap    ProviderCapability
	}{
		{
			name:               "Disable RBAC removes all RBAC sub-capabilities",
			capabilityToRemove: ProviderCapabilityRBAC,
			initialCapabilities: []ProviderCapability{
				ProviderCapabilitySynchronizeRoles,
				ProviderCapabilitySynchronizePermissions,
				ProviderCapabilitySynchronizeResources,
				ProviderCapabilityAuthorizeRole,
				ProviderCapabilityRevokeRole,
			},
			shouldNotHaveCap: ProviderCapabilityRBAC,
		},
		{
			name:               "Disable Authorizer removes all Authorizer sub-capabilities",
			capabilityToRemove: ProviderCapabilityAuthorizer,
			initialCapabilities: []ProviderCapability{
				ProviderCapabilityAuthorizeSession,
			},
			shouldNotHaveCap: ProviderCapabilityAuthorizer,
		},
		{
			name:               "Disable Notifier removes all Notifier sub-capabilities",
			capabilityToRemove: ProviderCapabilityNotifier,
			initialCapabilities: []ProviderCapability{
				ProviderCapabilitySendNotifications,
			},
			shouldNotHaveCap: ProviderCapabilityNotifier,
		},
		{
			name:               "Disable Identities removes all Identities sub-capabilities",
			capabilityToRemove: ProviderCapabilityIdentities,
			initialCapabilities: []ProviderCapability{
				ProviderCapabilitySynchronizeIdentities,
				ProviderCapabilitySynchronizeUsers,
				ProviderCapabilitySynchronizeGroups,
			},
			shouldNotHaveCap: ProviderCapabilityIdentities,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &BaseProvider{
				capabilities: tt.initialCapabilities,
			}

			// Verify the capability is present before disabling
			assert.True(t, p.HasCapability(tt.shouldNotHaveCap),
				"Expected to have capability %s before disabling", tt.shouldNotHaveCap)

			p.DisableCapability(tt.capabilityToRemove)

			// Verify the capability is gone after disabling
			assert.False(t, p.HasCapability(tt.shouldNotHaveCap),
				"Expected to NOT have capability %s after disabling", tt.shouldNotHaveCap)
		})
	}
}

func TestBaseProvider_DisableCapability_SubCapability(t *testing.T) {
	p := &BaseProvider{
		capabilities: []ProviderCapability{
			ProviderCapabilitySynchronizeRoles,
			ProviderCapabilityAuthorizeRole,
			ProviderCapabilityRevokeRole,
		},
	}

	// Verify initial state - should have RBAC because all required caps + one optional are present
	assert.True(t, p.HasCapability(ProviderCapabilityRBAC))

	// Disable the only optional sub-capability
	p.DisableCapability(ProviderCapabilitySynchronizeRoles)

	// Verify the sub-capability is gone
	assert.False(t, p.HasCapability(ProviderCapabilitySynchronizeRoles))

	// Now RBAC should be gone (has required but no optional)
	assert.False(t, p.HasCapability(ProviderCapabilityRBAC))

	// But required capabilities should still be present
	assert.True(t, p.HasCapability(ProviderCapabilityAuthorizeRole))
	assert.True(t, p.HasCapability(ProviderCapabilityRevokeRole))

	// Add another optional capability back
	p.EnableCapability(ProviderCapabilitySynchronizePermissions)
	// Now RBAC should be valid again (has required + one optional)
	assert.True(t, p.HasCapability(ProviderCapabilityRBAC))
}

func TestBaseProvider_EnableDisableCapability_Workflow(t *testing.T) {
	p := &BaseProvider{
		capabilities: make([]ProviderCapability, 0),
	}

	// Start with nothing
	assert.False(t, p.HasCapability(ProviderCapabilityRBAC))
	assert.False(t, p.HasCapability(ProviderCapabilityAuthorizer))

	// Enable RBAC
	p.EnableCapability(ProviderCapabilityRBAC)
	assert.True(t, p.HasCapability(ProviderCapabilityRBAC))
	assert.False(t, p.HasCapability(ProviderCapabilityAuthorizer))

	// Enable Authorizer
	p.EnableCapability(ProviderCapabilityAuthorizer)
	assert.True(t, p.HasCapability(ProviderCapabilityRBAC))
	assert.True(t, p.HasCapability(ProviderCapabilityAuthorizer))

	// Disable RBAC
	p.DisableCapability(ProviderCapabilityRBAC)
	assert.False(t, p.HasCapability(ProviderCapabilityRBAC))
	assert.True(t, p.HasCapability(ProviderCapabilityAuthorizer))

	// Disable Authorizer
	p.DisableCapability(ProviderCapabilityAuthorizer)
	assert.False(t, p.HasCapability(ProviderCapabilityRBAC))
	assert.False(t, p.HasCapability(ProviderCapabilityAuthorizer))
}

func TestBaseProvider_Authorizer_Workflow(t *testing.T) {
	p := &BaseProvider{
		capabilities: make([]ProviderCapability, 0),
	}

	// Start with no Authorizer capability
	assert.False(t, p.HasCapability(ProviderCapabilityAuthorizer))
	assert.False(t, p.HasCapability(ProviderCapabilityAuthorizeSession))

	// Enable Authorizer capability
	p.EnableCapability(ProviderCapabilityAuthorizer)
	assert.True(t, p.HasCapability(ProviderCapabilityAuthorizer))
	assert.True(t, p.HasCapability(ProviderCapabilityAuthorizeSession))

	// Disable the sub-capability directly
	p.DisableCapability(ProviderCapabilityAuthorizeSession)
	assert.False(t, p.HasCapability(ProviderCapabilityAuthorizer))
	assert.False(t, p.HasCapability(ProviderCapabilityAuthorizeSession))

	// Enable again and disable via top-level
	p.EnableCapability(ProviderCapabilityAuthorizer)
	assert.True(t, p.HasCapability(ProviderCapabilityAuthorizer))

	p.DisableCapability(ProviderCapabilityAuthorizer)
	assert.False(t, p.HasCapability(ProviderCapabilityAuthorizer))
}

func TestBaseProvider_Notifier_Workflow(t *testing.T) {
	p := &BaseProvider{
		capabilities: make([]ProviderCapability, 0),
	}

	// Start with no Notifier capability
	assert.False(t, p.HasCapability(ProviderCapabilityNotifier))
	assert.False(t, p.HasCapability(ProviderCapabilitySendNotifications))

	// Enable Notifier capability
	p.EnableCapability(ProviderCapabilityNotifier)
	assert.True(t, p.HasCapability(ProviderCapabilityNotifier))
	assert.True(t, p.HasCapability(ProviderCapabilitySendNotifications))

	// Disable the sub-capability directly
	p.DisableCapability(ProviderCapabilitySendNotifications)
	assert.False(t, p.HasCapability(ProviderCapabilityNotifier))
	assert.False(t, p.HasCapability(ProviderCapabilitySendNotifications))

	// Enable again and disable via top-level
	p.EnableCapability(ProviderCapabilityNotifier)
	assert.True(t, p.HasCapability(ProviderCapabilityNotifier))

	p.DisableCapability(ProviderCapabilityNotifier)
	assert.False(t, p.HasCapability(ProviderCapabilityNotifier))
}

func TestBaseProvider_Identities_Workflow(t *testing.T) {
	p := &BaseProvider{
		capabilities: make([]ProviderCapability, 0),
	}

	// Start with no Identities capability
	assert.False(t, p.HasCapability(ProviderCapabilityIdentities))

	// Enable Identities capability
	p.EnableCapability(ProviderCapabilityIdentities)
	assert.True(t, p.HasCapability(ProviderCapabilityIdentities))
	// Check that all sub-capabilities are present
	assert.True(t, p.HasCapability(ProviderCapabilitySynchronizeIdentities))
	assert.True(t, p.HasCapability(ProviderCapabilitySynchronizeUsers))
	assert.True(t, p.HasCapability(ProviderCapabilitySynchronizeGroups))

	// Disable one optional sub-capability
	p.DisableCapability(ProviderCapabilitySynchronizeIdentities)
	// Identities should still be true (has other optional caps)
	assert.True(t, p.HasCapability(ProviderCapabilityIdentities))
	assert.False(t, p.HasCapability(ProviderCapabilitySynchronizeIdentities))

	// Disable all other optional capabilities
	p.DisableCapability(ProviderCapabilitySynchronizeUsers)
	p.DisableCapability(ProviderCapabilitySynchronizeGroups)

	// Now Identities should be gone (no optional capabilities)
	assert.False(t, p.HasCapability(ProviderCapabilityIdentities))

	// Enable again and disable via top-level
	p.EnableCapability(ProviderCapabilityIdentities)
	assert.True(t, p.HasCapability(ProviderCapabilityIdentities))

	p.DisableCapability(ProviderCapabilityIdentities)
	assert.False(t, p.HasCapability(ProviderCapabilityIdentities))
	assert.False(t, p.HasCapability(ProviderCapabilitySynchronizeIdentities))
	assert.False(t, p.HasCapability(ProviderCapabilitySynchronizeUsers))
	assert.False(t, p.HasCapability(ProviderCapabilitySynchronizeGroups))
}

func TestBaseProvider_RBAC_Workflow(t *testing.T) {
	p := &BaseProvider{
		capabilities: make([]ProviderCapability, 0),
	}

	// Start with no RBAC capability
	assert.False(t, p.HasCapability(ProviderCapabilityRBAC))

	// Enable RBAC capability (adds required + optional)
	p.EnableCapability(ProviderCapabilityRBAC)
	assert.True(t, p.HasCapability(ProviderCapabilityRBAC))
	// Check that required sub-capabilities are present
	assert.True(t, p.HasCapability(ProviderCapabilityAuthorizeRole))
	assert.True(t, p.HasCapability(ProviderCapabilityRevokeRole))
	// Check that all optional capabilities are present
	assert.True(t, p.HasCapability(ProviderCapabilitySynchronizeRoles))
	assert.True(t, p.HasCapability(ProviderCapabilitySynchronizePermissions))
	assert.True(t, p.HasCapability(ProviderCapabilitySynchronizeResources))

	// Disable one optional sub-capability
	p.DisableCapability(ProviderCapabilitySynchronizeRoles)
	// RBAC should still be true (still has required + other optional caps)
	assert.True(t, p.HasCapability(ProviderCapabilityRBAC))

	// Disable all other optional capabilities
	p.DisableCapability(ProviderCapabilitySynchronizePermissions)
	p.DisableCapability(ProviderCapabilitySynchronizeResources)

	// Now RBAC should be gone (has required but no optional)
	assert.False(t, p.HasCapability(ProviderCapabilityRBAC))
	assert.False(t, p.HasCapability(ProviderCapabilitySynchronizeRoles))
	assert.False(t, p.HasCapability(ProviderCapabilitySynchronizePermissions))
	assert.False(t, p.HasCapability(ProviderCapabilitySynchronizeResources))
	// But required capabilities should still be there
	assert.True(t, p.HasCapability(ProviderCapabilityAuthorizeRole))
	assert.True(t, p.HasCapability(ProviderCapabilityRevokeRole))

	// Disable a required capability to test that requirement
	p.DisableCapability(ProviderCapabilityAuthorizeRole)
	assert.False(t, p.HasCapability(ProviderCapabilityRBAC))
	assert.False(t, p.HasCapability(ProviderCapabilityAuthorizeRole))
	assert.True(t, p.HasCapability(ProviderCapabilityRevokeRole))
}
