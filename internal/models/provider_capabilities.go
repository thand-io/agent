package models

import (
	"fmt"
	"slices"
	"strings"
)

type ProviderCapability string

type CapabilityRequirement struct {
	Capability     ProviderCapability
	Synchronizable bool
	Required       bool
}

func (c *CapabilityRequirement) CanSynchronize() bool {
	return c.Synchronizable
}

type ProviderCapabilitySet struct {
	Capabilities []CapabilityRequirement
}

const (
	// Identity synchronization capabilities
	ProviderCapabilityPrincipals ProviderCapability = "principals" // Primary capability
	ProviderCapabilityIdentities ProviderCapability = "identities"
	ProviderCapabilityUsers      ProviderCapability = "users"
	ProviderCapabilityGroups     ProviderCapability = "groups"

	// RBAC capabilities
	ProviderCapabilityRBAC          ProviderCapability = "rbac" // Primary capability
	ProviderCapabilityRoles         ProviderCapability = "roles"
	ProviderCapabilityPermissions   ProviderCapability = "permissions"
	ProviderCapabilityResources     ProviderCapability = "resources"
	ProviderCapabilityAuthorizeRole ProviderCapability = "authorize_role"
	ProviderCapabilityRevokeRole    ProviderCapability = "revoke_role"

	// Authorizer capabilities
	ProviderCapabilityAuthorizer       ProviderCapability = "authorizer" // Primary capability
	ProviderCapabilityAuthorizeSession ProviderCapability = "authorize_session"

	// Notifier capabilities
	ProviderCapabilityNotifier          ProviderCapability = "notifier" // Primary capability
	ProviderCapabilitySendNotifications ProviderCapability = "send_notifications"
)

type RolesConfiguration struct {
	SynchronizableConfiguration
}

type PermissionsConfiguration struct {
	SynchronizableConfiguration
}

type ResourcesConfiguration struct {
	SynchronizableConfiguration
}

type IdentitiesConfiguration struct {
	SynchronizableConfiguration
}

type UsersConfiguration struct {
	SynchronizableConfiguration
}

type GroupsConfiguration struct {
	SynchronizableConfiguration
}

type SynchronizableConfiguration struct {
	Synchronizable bool `json:"synchronizable,omitempty"`
	Interval       int  `json:"interval,omitempty"` // in minutes
	Disable        bool `json:"disable,omitempty"`
}

func (sc *SynchronizableConfiguration) CanSynchronize() bool {
	return sc.Synchronizable && !sc.Disable
}

func (sc *SynchronizableConfiguration) Enable() {
	sc.Disable = false
	sc.Synchronizable = true
}

func (sc *SynchronizableConfiguration) DisableConfig() {
	sc.Disable = true
}

type AuthorizeSessionConfiguration struct {
	DisablableConfiguration
}

type SendNotificationsConfiguration struct {
	DisablableConfiguration
}

type DisablableConfiguration struct {
	Disable bool `json:"disable,omitempty"`
}

func (dc *DisablableConfiguration) CanSynchronize() bool {
	return !dc.Disable
}

func (dc *DisablableConfiguration) Enable() {
	dc.Disable = false
}

func (dc *DisablableConfiguration) DisableConfig() {
	dc.Disable = true
}

type ProviderCapabilties struct {
	Roles       RolesConfiguration       `json:"roles,omitempty"`
	Permissions PermissionsConfiguration `json:"permissions,omitempty"`
	Resources   ResourcesConfiguration   `json:"resources,omitempty"`

	Identities IdentitiesConfiguration `json:"identities,omitempty"`
	Users      UsersConfiguration      `json:"users,omitempty"`
	Groups     GroupsConfiguration     `json:"groups,omitempty"`

	AuthorizeSession  AuthorizeSessionConfiguration  `json:"authorize_session,omitempty"`
	SendNotifications SendNotificationsConfiguration `json:"send_notifications,omitempty"`
}

func (pc *ProviderCapabilties) IsCapabilityEnabled(capability ProviderCapability) bool {
	switch capability {
	case ProviderCapabilityRoles:
		return !pc.Roles.Disable
	case ProviderCapabilityPermissions:
		return !pc.Permissions.Disable
	case ProviderCapabilityResources:
		return !pc.Resources.Disable
	case ProviderCapabilityIdentities:
		return !pc.Identities.Disable
	case ProviderCapabilityUsers:
		return !pc.Users.Disable
	case ProviderCapabilityGroups:
		return !pc.Groups.Disable
	case ProviderCapabilityAuthorizeSession:
		return !pc.AuthorizeSession.Disable
	case ProviderCapabilitySendNotifications:
		return !pc.SendNotifications.Disable
	default:
		return false
	}
}

func NewCapability() *SynchronizableConfiguration {
	return &SynchronizableConfiguration{
		Synchronizable: false,
		Interval:       60,
		Disable:        false,
	}
}

func NewSynchronizableCapability() *DisablableConfiguration {
	return &DisablableConfiguration{
		Disable: false,
	}
}

type ProviderCapabilityImpl interface {
	CanSynchronize() bool
	Enable()
	DisableConfig()
}

var (
	// If a provider has ALL of the capabilites marked as "required"
	// and ONE of the optional capabilities then it is considered to
	// have that set of capabilities
	ProviderCapabilitySetRBAC = ProviderCapabilitySet{
		Capabilities: []CapabilityRequirement{
			{ProviderCapabilityRoles, true, false},        // Optional
			{ProviderCapabilityPermissions, true, false},  // Optional
			{ProviderCapabilityResources, true, false},    // Optional
			{ProviderCapabilityAuthorizeRole, true, true}, // Required
			{ProviderCapabilityRevokeRole, true, true},    // Required
		},
	}
	ProviderCapabilitySetAuthorizer = ProviderCapabilitySet{
		Capabilities: []CapabilityRequirement{
			{ProviderCapabilityAuthorizeSession, false, true}, // Required
		},
	}
	ProviderCapabilitySetNotifier = ProviderCapabilitySet{
		Capabilities: []CapabilityRequirement{
			{ProviderCapabilitySendNotifications, false, true}, // Required
		},
	}
	ProviderCapabilitySetIdentities = ProviderCapabilitySet{
		Capabilities: []CapabilityRequirement{
			{ProviderCapabilityIdentities, true, false}, // Optional
			{ProviderCapabilityUsers, true, false},      // Optional
			{ProviderCapabilityGroups, true, false},     // Optional
		},
	}
)

func GetCapabilityFromString(cap string) (ProviderCapability, error) {
	switch strings.ToLower(cap) {
	case string(ProviderCapabilityRBAC):
		return ProviderCapabilityRBAC, nil
	case string(ProviderCapabilityAuthorizer):
		return ProviderCapabilityAuthorizer, nil
	case string(ProviderCapabilityNotifier):
		return ProviderCapabilityNotifier, nil
	case string(ProviderCapabilityIdentities):
		return ProviderCapabilityIdentities, nil
	default:
		return "", fmt.Errorf("unknown capability: %s", cap)
	}
}

func (p *BaseProvider) GetCapabilities() *ProviderCapabilties {
	return p.capabilities
}

func (p *BaseProvider) HasCapability(capability ProviderCapability) bool {

	// Check if capability is enabled in the capabilities struct
	if p.GetCapabilities().IsCapabilityEnabled(capability) {
		return true
	}

	// Check top-level capabilities by their requirement sets
	var capabilitySet *ProviderCapabilitySet
	switch capability {
	case ProviderCapabilityRBAC:
		capabilitySet = &ProviderCapabilitySetRBAC
	case ProviderCapabilityAuthorizer:
		capabilitySet = &ProviderCapabilitySetAuthorizer
	case ProviderCapabilityNotifier:
		capabilitySet = &ProviderCapabilitySetNotifier
	case ProviderCapabilityIdentities:
		capabilitySet = &ProviderCapabilitySetIdentities
	default:
		return false
	}

	// Check if all required capabilities are present
	for _, req := range capabilitySet.Capabilities {
		if req.Required && !p.GetCapabilities().IsCapabilityEnabled(req.Capability) {
			return false
		}
	}

	// For capabilities with optional requirements, check if at least one is present
	// This applies to RBAC and Identities which have optional capabilities
	hasOptional := false
	hasOptionalCaps := false
	for _, req := range capabilitySet.Capabilities {
		if !req.Required {
			hasOptionalCaps = true
			if p.GetCapabilities().IsCapabilityEnabled(req.Capability) {
				hasOptional = true
				break
			}
		}
	}

	// If capability set has optional capabilities, at least one must be present
	if hasOptionalCaps {
		return hasOptional
	}

	// For capability sets with only required capabilities (Authorizer, Notifier),
	// having all required capabilities is sufficient
	return true
}

func (p *BaseProvider) HasAnyCapability(capabilities ...ProviderCapability) bool {
	return slices.ContainsFunc(capabilities, p.HasCapability)
}

func (p *BaseProvider) getCapabilityImpl(capability ProviderCapability) ProviderCapabilityImpl {
	caps := p.GetCapabilities()
	switch capability {
	case ProviderCapabilityRoles:
		return &caps.Roles
	case ProviderCapabilityPermissions:
		return &caps.Permissions
	case ProviderCapabilityResources:
		return &caps.Resources
	case ProviderCapabilityIdentities:
		return &caps.Identities
	case ProviderCapabilityUsers:
		return &caps.Users
	case ProviderCapabilityGroups:
		return &caps.Groups
	case ProviderCapabilityAuthorizeSession:
		return &caps.AuthorizeSession
	case ProviderCapabilitySendNotifications:
		return &caps.SendNotifications
	default:
		return nil
	}
}

func (p *BaseProvider) EnableCapability(capability ProviderCapability) {
	// For top-level capabilities, enable all their sub-capabilities
	switch capability {
	case ProviderCapabilityRBAC:
		p.EnableCapability(ProviderCapabilityRoles)
		p.EnableCapability(ProviderCapabilityPermissions)
		p.EnableCapability(ProviderCapabilityResources)
		p.EnableCapability(ProviderCapabilityAuthorizeRole)
		p.EnableCapability(ProviderCapabilityRevokeRole)
	case ProviderCapabilityAuthorizer:
		p.EnableCapability(ProviderCapabilityAuthorizeSession)
	case ProviderCapabilityNotifier:
		p.EnableCapability(ProviderCapabilitySendNotifications)
	case ProviderCapabilityPrincipals:
		p.EnableCapability(ProviderCapabilityIdentities)
		p.EnableCapability(ProviderCapabilityUsers)
		p.EnableCapability(ProviderCapabilityGroups)
	default:
		// Use the interface implementation for individual capabilities
		if impl := p.getCapabilityImpl(capability); impl != nil {
			impl.Enable()
		}
	}
}

func (p *BaseProvider) DisableCapability(capability ProviderCapability) {
	// For top-level capabilities, disable all their sub-capabilities
	switch capability {
	case ProviderCapabilityRBAC:
		p.DisableCapability(ProviderCapabilityRoles)
		p.DisableCapability(ProviderCapabilityPermissions)
		p.DisableCapability(ProviderCapabilityResources)
		p.DisableCapability(ProviderCapabilityAuthorizeRole)
		p.DisableCapability(ProviderCapabilityRevokeRole)
	case ProviderCapabilityAuthorizer:
		p.DisableCapability(ProviderCapabilityAuthorizeSession)
	case ProviderCapabilityNotifier:
		p.DisableCapability(ProviderCapabilitySendNotifications)
	case ProviderCapabilityPrincipals:
		p.DisableCapability(ProviderCapabilityIdentities)
		p.DisableCapability(ProviderCapabilityUsers)
		p.DisableCapability(ProviderCapabilityGroups)
	default:
		// Use the interface implementation for individual capabilities
		if impl := p.getCapabilityImpl(capability); impl != nil {
			impl.DisableConfig()
		}
	}

func (p *BaseProvider) CanSynchronizeRoles() bool {
	if !p.HasCapability(ProviderCapabilityRoles) {
		return false
	}
	return p.GetCapabilities().Roles.Synchronizable && !p.GetCapabilities().Roles.Disable
}

func (p *BaseProvider) CanSynchronizePermissions() bool {
	if !p.HasCapability(ProviderCapabilityPermissions) {
		return false
	}
	return p.GetCapabilities().Permissions.Synchronizable && !p.GetCapabilities().Permissions.Disable
}

func (p *BaseProvider) CanSynchronizeUsers() bool {
	if !p.HasCapability(ProviderCapabilityUsers) {
		return false
	}
	return p.GetCapabilities().Users.Synchronizable && !p.GetCapabilities().Users.Disable
}

func (p *BaseProvider) CanSynchronizeGroups() bool {
	if !p.HasCapability(ProviderCapabilityGroups) {
		return false
	}
	return p.GetCapabilities().Groups.Synchronizable && !p.GetCapabilities().Groups.Disable
}

func (p *BaseProvider) CanSynchronizeIdentities() bool {
	if !p.HasCapability(ProviderCapabilityIdentities) {
		return false
	}
	return p.GetCapabilities().Identities.Synchronizable && !p.GetCapabilities().Identities.Disable
}

func (p *BaseProvider) CanSynchronizeResources() bool {
	if !p.HasCapability(ProviderCapabilityResources) {
		return false
	}
	return p.GetCapabilities().Resources.Synchronizable && !p.GetCapabilities().Resources.Disable
}
