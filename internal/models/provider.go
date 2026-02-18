package models

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/interpolate"
)

var ErrNotImplemented = errors.New("not implemented")

/*
	name: aws-prod
	description: Production AWS environment
	provider: aws
	capabilities:

	  rbac:
		can_synchronize_roles: true
		can_synchronize_permissions: true
		can_synchronize_resources: false
	  authorizer:
		can_authorize_session: true
	  notifier:
		can_send_notifications: true
	  identities:
		can_synchronize_identities: true
		can_synchronize_users: true
		can_synchronize_groups: true

	config:

		region: us-east-1
		account_id: "123456789012"

	enabled: true
*/

type ProviderConfig struct {
	Version      *version.Version      `json:"version,omitempty"`
	Name         string                `json:"name" validate:"required,min=1,max=100"`
	Description  string                `json:"description" validate:"max=500"`
	Provider     string                `json:"provider" validate:"required,min=2,max=50,alphanum_hyphen"` // e.g. aws, gcp, azure
	Capabilities *ProviderCapabilities `json:"capabilities,omitempty"`                                    // Allows the user to specify what this provider can do
	Config       *BasicConfig          `json:"config,omitempty"`                                          // Provider-specific configuration
	Role         *Role                 `json:"role,omitempty"`                                            // The base role for this provider
	Enabled      bool                  `json:"enabled"`                                                   // Whether this provider is enabled
}

func (p *ProviderConfig) Validate() error {

	validate := common.GetValidator()

	if err := validate.Struct(p); err != nil {
		return fmt.Errorf("provider '%s' validation failed: %w", p.Name, err)
	}

	// Validate provider-specific config using provider registry (no initialization needed)
	if p.Config != nil {
		if err := ValidateProviderConfig(p.Provider, p.Config); err != nil {
			return fmt.Errorf("provider '%s': %w", p.Name, err)
		}
	}

	return nil
}

func (p *ProviderConfig) ResolveConfig(vars map[string]any) error {

	envs := os.Environ()

	for _, env := range envs {
		parts := strings.SplitN(env, "=", 2)
		if len(parts) == 2 {
			vars[parts[0]] = parts[1]
		}
	}

	newConfig, err := interpolate.NewTraverse(p.Config.AsMap(), vars, nil)

	if err != nil {
		return fmt.Errorf("failed to create traverse for provider config: %w", err)
	}

	if basicConfig, ok := newConfig.(map[string]any); ok {
		p.Config.Update(basicConfig)
		return nil
	}

	return fmt.Errorf("the traversed config was not a map")
}

/*
A user is assigned a role (e.g., "Manager").
This role has associated permissions (e.g., "approve reports," "view employee data").
These permissions, along with access to specific resources (e.g., "company financial reports"), constitute the user's entitlements.
*/

// Interface for provider implementations
type Provider interface {
	// Metadata methods (work without initialization)
	ValidateConfig(config *BasicConfig) error
	GetDefaultCapabilities() *ProviderCapabilities

	// Lifecycle methods
	Initialize(identifier string, provider ProviderConfig) error
	Validate() error // Validate provider configuration against schema

	// Form base provider
	GetConfig() *BasicConfig
	GetIdentifier() string // This is the global unique identifier for the provider. This is the provider key in the config
	GetName() string
	GetDescription() string
	GetProvider() string
	GetBaseRole() *Role
	HasPermission(user *User) bool

	Synchronize(ctx context.Context, temporalClient TemporalImpl, req *SynchronizeRequest) error

	// Temporal
	RegisterWorkflows(temporalClient TemporalImpl) error
	RegisterActivities(temporalClient TemporalImpl) error

	GetCapabilities() *ProviderCapabilities
	HasCapability(capability ProviderCapability) bool
	HasAnyCapability(capabilities ...ProviderCapability) bool

	// Let us know what this provider can synchronize
	CanSynchronizeRoles() bool
	CanSynchronizePermissions() bool
	CanSynchronizeResources() bool
	CanSynchronizeIdentities() bool
	CanSynchronizeTenants() bool
	CanSynchronizeUsers() bool
	CanSynchronizeGroups() bool

	// Sub-interfaces
	ProviderNotifier
	ProviderWebhook
	ProviderAuthorizor
	ProviderRoleBasedAccessControl
	ProviderIdentities
	ProviderTenants
}

type AuthorizeSessionResponse struct {
	Url string `json:"url"`
}

type RoleRequest struct {
	Tenant   string         `json:"tenant,omitempty"` // Optional tenant ID for multi-account providers
	User     *User          `json:"user"`
	Role     *Role          `json:"role"`
	Duration *time.Duration `json:"duration,omitempty"` // Optional duration for temporary access
}

// IsValid checks if any of the fields are nil
// if they are then it returns false
func (r *RoleRequest) IsValid() bool {
	return r.User != nil && r.Role != nil
}

func (r *RoleRequest) GetUser() *User {
	return r.User
}

func (r *RoleRequest) GetRole() *Role {
	return r.Role
}

func (r *RoleRequest) GetDuration() *time.Duration {
	return r.Duration
}

// ValidateProviderConfig is a variable holding the config validation function
// This is set by the providers package to avoid circular dependencies
var ValidateProviderConfig = func(providerName string, config *BasicConfig) error {
	// Default implementation: no validation
	// This will be overridden by providers.SetConfigValidator() in providers package
	return nil
}
