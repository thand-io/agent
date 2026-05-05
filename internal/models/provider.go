package models

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/interpolate"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
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
	// Lifecycle methods
	Initialize(identifier string, provider ProviderConfig) error

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
	RegisterWorkflows(runtime sdkConstants.Mode) any
	RegisterActivities(runtime sdkConstants.Mode) any

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

	// Synchronization readiness — providers signal readiness after their
	// initial role/permission data has been loaded.
	SetPending()
	SetReady()
	IsReady() bool
	Ready() <-chan struct{}

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
