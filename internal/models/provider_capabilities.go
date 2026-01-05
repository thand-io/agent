package models

import (
	"fmt"
	"slices"
	"strings"
)

type ProviderCapability string

const (
	// Identity synchronization capabilities
	ProviderCapabilityIdentities ProviderCapability = "identities"
	ProviderCapabilityUsers      ProviderCapability = "users"
	ProviderCapabilityGroups     ProviderCapability = "groups"

	// RBAC capabilities
	ProviderCapabilityRoles       ProviderCapability = "roles"
	ProviderCapabilityPermissions ProviderCapability = "permissions"
	ProviderCapabilityResources   ProviderCapability = "resources"
	// Provisioning provides authorization and revocation of roles
	ProviderCapabilityProvisioning ProviderCapability = "provisioning"

	// Authorizer capabilities
	ProviderCapabilityAuthorizer ProviderCapability = "authorizer" // Primary capability

	// Notifier capabilities
	ProviderCapabilityNotifier ProviderCapability = "notifier" // Outbound notifications
	ProviderCapabilityWebhook  ProviderCapability = "webhook"  // Webhook / inbound notifications

	// Tenant discovery capability
	ProviderCapabilityTenants ProviderCapability = "tenants"
)

type RolesConfiguration = SynchronizableConfiguration
type PermissionsConfiguration = SynchronizableConfiguration
type ResourcesConfiguration = SynchronizableConfiguration
type IdentitiesConfiguration = SynchronizableConfiguration
type UsersConfiguration = SynchronizableConfiguration
type GroupsConfiguration = SynchronizableConfiguration
type TenantsConfiguration = SynchronizableConfiguration

type SynchronizableConfiguration struct {
	Synchronizable bool `json:"synchronizable,omitempty"`
	Interval       int  `json:"interval,omitempty" validate:"omitempty,min=1,max=43200"` // in minutes (max 30 days)
	Enabled        bool `json:"enabled,omitempty"`
}

type SynchronizableConfigurationImpl interface {
	IsSynchronizable() bool
	GetSynchronizable() bool
	EnableSynchronization()
	DisableSynchronization()

	GetInterval() int
	SetInterval(interval int)

	// Methods to enable/disable
	IsEnabled() bool
	Enable()
	Disable()
}

func (sc *SynchronizableConfiguration) IsSynchronizable() bool {
	return sc.Synchronizable && sc.Enabled
}

func (sc *SynchronizableConfiguration) GetSynchronizable() bool {
	return sc.Synchronizable
}

func (sc *SynchronizableConfiguration) EnableSynchronization() {
	sc.Synchronizable = true
}

func (sc *SynchronizableConfiguration) DisableSynchronization() {
	sc.Synchronizable = false
}

func (sc *SynchronizableConfiguration) GetInterval() int {
	return sc.Interval
}

func (sc *SynchronizableConfiguration) SetInterval(interval int) {
	sc.Interval = interval
}

func (sc *SynchronizableConfiguration) IsEnabled() bool {
	return sc.Enabled
}

func (sc *SynchronizableConfiguration) Enable() {
	sc.Enabled = true
}

func (sc *SynchronizableConfiguration) Disable() {
	sc.Enabled = false
}

type AuthorizerConfiguration = ProviderConfiguration
type NotifierConfiguration = ProviderConfiguration
type WebhookConfiguration = ProviderConfiguration
type ProvisioningConfiguration = ProviderConfiguration

type ProviderConfiguration struct {
	Enabled bool `json:"enabled,omitempty"`
}

type ProviderConfigurationImpl interface {
	IsEnabled() bool
	Enable()
	Disable()
}

func (dc *ProviderConfiguration) IsEnabled() bool {
	return dc.Enabled
}

func (dc *ProviderConfiguration) Enable() {
	dc.Enabled = true
}

func (dc *ProviderConfiguration) Disable() {
	dc.Enabled = false
}

type ProviderCapabilities struct {

	// Identity management capabilities
	Identities *IdentitiesConfiguration `json:"identities,omitempty"`
	Users      *UsersConfiguration      `json:"users,omitempty"`
	Groups     *GroupsConfiguration     `json:"groups,omitempty"`

	// SSO Capabilities
	Authorizer *AuthorizerConfiguration `json:"authorizer,omitempty"`

	// Notifier
	Notifier *NotifierConfiguration `json:"notifier,omitempty"`
	Webhook  *NotifierConfiguration `json:"webhook,omitempty"`

	// Rbac capabilities
	Provisioning *ProvisioningConfiguration `json:"provisioning,omitempty"`
	Roles        *RolesConfiguration        `json:"roles,omitempty"`
	Permissions  *PermissionsConfiguration  `json:"permissions,omitempty"`
	Resources    *ResourcesConfiguration    `json:"resources,omitempty"`

	// Tenant discovery capability
	Tenants *TenantsConfiguration `json:"tenants,omitempty"`
}

func (pc *ProviderCapabilities) WithDefaultRolesConfiguration() *ProviderCapabilities {
	pc.Roles = NewSynchronizableCapability()
	return pc
}

func (pc *ProviderCapabilities) WithDefaultPermissionsConfiguration() *ProviderCapabilities {
	pc.Permissions = NewSynchronizableCapability()
	return pc
}

func (pc *ProviderCapabilities) WithDefaultResourcesConfiguration() *ProviderCapabilities {
	pc.Resources = NewSynchronizableCapability()
	return pc
}

func (pc *ProviderCapabilities) WithDefaultIdentitiesConfiguration() *ProviderCapabilities {
	pc.Identities = NewSynchronizableCapability()
	return pc
}

func (pc *ProviderCapabilities) WithDefaultUsersConfiguration() *ProviderCapabilities {
	pc.Users = NewSynchronizableCapability()
	return pc
}

func (pc *ProviderCapabilities) WithDefaultGroupsConfiguration() *ProviderCapabilities {
	pc.Groups = NewSynchronizableCapability()
	return pc
}

func (pc *ProviderCapabilities) WithDefaultAuthorizerConfiguration() *ProviderCapabilities {
	pc.Authorizer = NewCapability()
	return pc
}

func (pc *ProviderCapabilities) WithDefaultNotifierConfiguration() *ProviderCapabilities {
	pc.Notifier = NewCapability()
	return pc
}

func (pc *ProviderCapabilities) WithDefaultWebhookConfiguration() *ProviderCapabilities {
	pc.Webhook = NewCapability()
	return pc
}

func (pc *ProviderCapabilities) WithDefaultProvisioningConfiguration() *ProviderCapabilities {
	pc.Provisioning = NewCapability()
	return pc
}

func (pc *ProviderCapabilities) WithDefaultTenantsConfiguration() *ProviderCapabilities {
	pc.Tenants = NewSynchronizableCapability()
	return pc
}

func (pc *ProviderCapabilities) WithRolesConfiguration(config RolesConfiguration) *ProviderCapabilities {
	pc.Roles = &config
	return pc
}

func (pc *ProviderCapabilities) WithPermissionsConfiguration(config PermissionsConfiguration) *ProviderCapabilities {
	pc.Permissions = &config
	return pc
}

func (pc *ProviderCapabilities) WithResourcesConfiguration(config ResourcesConfiguration) *ProviderCapabilities {
	pc.Resources = &config
	return pc
}

func (pc *ProviderCapabilities) WithIdentitiesConfiguration(config IdentitiesConfiguration) *ProviderCapabilities {
	pc.Identities = &config
	return pc
}

func (pc *ProviderCapabilities) WithUsersConfiguration(config UsersConfiguration) *ProviderCapabilities {
	pc.Users = &config
	return pc
}

func (pc *ProviderCapabilities) WithGroupsConfiguration(config GroupsConfiguration) *ProviderCapabilities {
	pc.Groups = &config
	return pc
}

func (pc *ProviderCapabilities) WithAuthorizerConfiguration(config AuthorizerConfiguration) *ProviderCapabilities {
	pc.Authorizer = &config
	return pc
}

func (pc *ProviderCapabilities) WithNotifierConfiguration(config NotifierConfiguration) *ProviderCapabilities {
	pc.Notifier = &config
	return pc
}

func (pc *ProviderCapabilities) WithWebhookConfiguration(config NotifierConfiguration) *ProviderCapabilities {
	pc.Webhook = &config
	return pc
}

func (pc *ProviderCapabilities) WithProvisioningConfiguration(config ProvisioningConfiguration) *ProviderCapabilities {
	pc.Provisioning = &config
	return pc
}

func (pc *ProviderCapabilities) WithTenantsConfiguration(config TenantsConfiguration) *ProviderCapabilities {
	pc.Tenants = &config
	return pc
}

func (pc *ProviderCapabilities) getCapabilities() map[ProviderCapability]ProviderConfigurationImpl {
	configMap := make(map[ProviderCapability]ProviderConfigurationImpl)

	if pc.Roles != nil {
		configMap[ProviderCapabilityRoles] = pc.Roles
	}
	if pc.Permissions != nil {
		configMap[ProviderCapabilityPermissions] = pc.Permissions
	}
	if pc.Resources != nil {
		configMap[ProviderCapabilityResources] = pc.Resources
	}
	if pc.Identities != nil {
		configMap[ProviderCapabilityIdentities] = pc.Identities
	}
	if pc.Users != nil {
		configMap[ProviderCapabilityUsers] = pc.Users
	}
	if pc.Groups != nil {
		configMap[ProviderCapabilityGroups] = pc.Groups
	}
	if pc.Authorizer != nil {
		configMap[ProviderCapabilityAuthorizer] = pc.Authorizer
	}
	if pc.Notifier != nil {
		configMap[ProviderCapabilityNotifier] = pc.Notifier
	}
	if pc.Webhook != nil {
		configMap[ProviderCapabilityWebhook] = pc.Webhook
	}
	if pc.Provisioning != nil {
		configMap[ProviderCapabilityProvisioning] = pc.Provisioning
	}
	if pc.Tenants != nil {
		configMap[ProviderCapabilityTenants] = pc.Tenants
	}

	return configMap
}

// getCapabilityConfig returns the configuration for a given capability
func (pc *ProviderCapabilities) getCapabilityConfig(capability ProviderCapability) ProviderConfigurationImpl {
	configMap := pc.getCapabilities()
	return configMap[capability]
}

func (pc *ProviderCapabilities) IsCapabilityEnabled(capability ProviderCapability) bool {
	config := pc.getCapabilityConfig(capability)
	if config == nil {
		return false
	}
	// Otherwise, it's a DisablableConfiguration
	return config.IsEnabled()
}

func (pc *ProviderCapabilities) EnableCapability(capability ProviderCapability) {
	config := pc.getCapabilityConfig(capability)
	if config != nil {
		config.Enable()
	}
}

func (pc *ProviderCapabilities) Update(updates *ProviderCapabilities) {

	if updates == nil {
		return
	}

	// We first need to get a map of the existing capabilities
	// and check to see whats enabled. Then if the incoming
	// updates have a capability disabled then we need to disable it.
	// DO NOT enable anything that is already disabled.
	// We also want to update the existing configuration values

	updateSync := func(
		curr SynchronizableConfigurationImpl,
		upd SynchronizableConfigurationImpl,
	) {
		if curr == nil || upd == nil {
			return
		}

		// If the capability is disabled by the provider, we cannot enable it
		if !curr.IsEnabled() {
			return
		}

		if upd.GetInterval() != 0 {
			curr.SetInterval(upd.GetInterval())
		}

		if upd.GetSynchronizable() {
			curr.EnableSynchronization()
		} else {
			curr.DisableSynchronization()
		}

		if !upd.IsEnabled() {
			curr.Disable()
		}
	}

	updateDisable := func(
		curr ProviderConfigurationImpl,
		upd ProviderConfigurationImpl,
	) {
		if curr == nil || upd == nil {
			return
		}

		// If the capability is disabled by the provider, we cannot enable it
		if !curr.IsEnabled() {
			return
		}

		if !upd.IsEnabled() {
			curr.Disable()
		}
	}

	if pc.Roles != nil && updates.Roles != nil {
		updateSync(pc.Roles, updates.Roles)
	}
	if pc.Permissions != nil && updates.Permissions != nil {
		updateSync(pc.Permissions, updates.Permissions)
	}
	if pc.Resources != nil && updates.Resources != nil {
		updateSync(pc.Resources, updates.Resources)
	}
	if pc.Identities != nil && updates.Identities != nil {
		updateSync(pc.Identities, updates.Identities)
	}
	if pc.Users != nil && updates.Users != nil {
		updateSync(pc.Users, updates.Users)
	}
	if pc.Groups != nil && updates.Groups != nil {
		updateSync(pc.Groups, updates.Groups)
	}
	if pc.Tenants != nil && updates.Tenants != nil {
		updateSync(pc.Tenants, updates.Tenants)
	}

	if pc.Authorizer != nil && updates.Authorizer != nil {
		updateDisable(pc.Authorizer, updates.Authorizer)
	}
	if pc.Notifier != nil && updates.Notifier != nil {
		updateDisable(pc.Notifier, updates.Notifier)
	}
	if pc.Webhook != nil && updates.Webhook != nil {
		updateDisable(pc.Webhook, updates.Webhook)
	}
	if pc.Provisioning != nil && updates.Provisioning != nil {
		updateDisable(pc.Provisioning, updates.Provisioning)
	}
}

func (pc *ProviderCapabilities) RemoveCapability(capability ProviderCapability) {
	config := pc.getCapabilityConfig(capability)
	if config != nil {
		config.Disable()
	}
}

// NewSynchronizableCapability creates a new enabled synchronizable capability configuration
func NewSynchronizableCapability() *SynchronizableConfiguration {
	return &SynchronizableConfiguration{
		Synchronizable: true,
		// Every 6 hours by default. Interval is in minutes
		Interval: 6 * 60,
		Enabled:  true,
	}
}

// NewCapability creates a new enabled capability configuration
func NewCapability() *ProviderConfiguration {
	return &ProviderConfiguration{
		Enabled: true,
	}
}

func NewProviderCapabilities() *ProviderCapabilities {
	return &ProviderCapabilities{
		Roles:        &RolesConfiguration{},
		Permissions:  &PermissionsConfiguration{},
		Resources:    &ResourcesConfiguration{},
		Identities:   &IdentitiesConfiguration{},
		Users:        &UsersConfiguration{},
		Groups:       &GroupsConfiguration{},
		Authorizer:   &AuthorizerConfiguration{},
		Notifier:     &NotifierConfiguration{},
		Webhook:      &WebhookConfiguration{},
		Tenants:      &TenantsConfiguration{},
		Provisioning: &ProvisioningConfiguration{},
	}
}

func GetCapabilityFromString(cap string) (ProviderCapability, error) {

	switch strings.ToLower(cap) {
	case string(ProviderCapabilityAuthorizer):
		return ProviderCapabilityAuthorizer, nil
	case string(ProviderCapabilityNotifier):
		return ProviderCapabilityNotifier, nil
	case string(ProviderCapabilityWebhook):
		return ProviderCapabilityWebhook, nil
	case string(ProviderCapabilityTenants):
		return ProviderCapabilityTenants, nil
	case string(ProviderCapabilityIdentities):
		return ProviderCapabilityIdentities, nil
	case string(ProviderCapabilityUsers):
		return ProviderCapabilityUsers, nil
	case string(ProviderCapabilityGroups):
		return ProviderCapabilityGroups, nil
	case string(ProviderCapabilityRoles):
		return ProviderCapabilityRoles, nil
	case string(ProviderCapabilityPermissions):
		return ProviderCapabilityPermissions, nil
	case string(ProviderCapabilityResources):
		return ProviderCapabilityResources, nil
	case string(ProviderCapabilityProvisioning):
		return ProviderCapabilityProvisioning, nil
	default:
		return "", fmt.Errorf("unknown capability: %s", cap)
	}
}

func (p *BaseProvider) GetCapabilities() *ProviderCapabilities {
	return p.capabilities
}

func (p *BaseProvider) HasCapability(capability ProviderCapability) bool {

	// Check if capability is enabled in the capabilities struct
	if p.GetCapabilities().IsCapabilityEnabled(capability) {
		return true
	}
	return false
}

func (p *BaseProvider) HasAnyCapability(capabilities ...ProviderCapability) bool {
	return slices.ContainsFunc(capabilities, p.HasCapability)
}

func (p *BaseProvider) getCapability(capability ProviderCapability) ProviderConfigurationImpl {
	return p.GetCapabilities().getCapabilityConfig(capability)
}

func (p *BaseProvider) getSynchronizableCapability(capability ProviderCapability) SynchronizableConfigurationImpl {
	config := p.GetCapabilities().getCapabilityConfig(capability)
	if syncConfig, ok := config.(SynchronizableConfigurationImpl); ok {
		return syncConfig
	}
	return nil
}

func (p *BaseProvider) EnableCapability(capability ProviderCapability) {
	// For top-level capabilities, enable all their sub-capabilities
	// Use the interface implementation for individual capabilities
	if impl := p.getCapability(capability); impl != nil {
		impl.Enable()
	}
}

func (p *BaseProvider) DisableCapabilities(capability ...ProviderCapability) {
	for _, cap := range capability {
		p.DisableCapability(cap)
	}
}

func (p *BaseProvider) DisableCapability(capability ProviderCapability) {
	// For top-level capabilities, disable all their sub-capabilities
	if impl := p.getCapability(capability); impl != nil {
		impl.Disable()
	}
}

func (p *BaseProvider) CanSynchronize(capability ProviderCapability) bool {
	if !p.HasCapability(capability) {
		return false
	}
	cap := p.getSynchronizableCapability(capability)
	if cap == nil {
		return false
	}
	return cap.IsSynchronizable()
}

func (p *BaseProvider) CanSynchronizeRoles() bool {
	return p.CanSynchronize(ProviderCapabilityRoles)
}

func (p *BaseProvider) CanSynchronizePermissions() bool {
	return p.CanSynchronize(ProviderCapabilityPermissions)
}

func (p *BaseProvider) CanSynchronizeUsers() bool {
	return p.CanSynchronize(ProviderCapabilityUsers)
}

func (p *BaseProvider) CanSynchronizeGroups() bool {
	return p.CanSynchronize(ProviderCapabilityGroups)
}

func (p *BaseProvider) CanSynchronizeIdentities() bool {
	return p.CanSynchronize(ProviderCapabilityIdentities)
}

func (p *BaseProvider) CanSynchronizeTenants() bool {
	return p.CanSynchronize(ProviderCapabilityTenants)
}

func (p *BaseProvider) CanSynchronizeResources() bool {
	return p.CanSynchronize(ProviderCapabilityResources)
}
