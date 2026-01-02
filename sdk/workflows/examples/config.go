package examples

import (
	"fmt"

	"github.com/thand-io/agent/internal/models"
)

// Config is a minimal implementation of models.ConfigImpl for testing and example workflows.
// It provides stub implementations of the ConfigImpl interface methods, allowing workflows
// to be tested in isolation without requiring a full Thand agent configuration.
// This is particularly useful for workflow development and unit testing.
type Config struct {
	// Workflows
	workflows map[string]*models.Workflow
	services  *Services
}

// NewConfig creates a new Config instance with an empty workflow registry
// and a basic services implementation. This is the entry point for creating
// a minimal configuration for testing or running example workflows.
func NewConfig() *Config {
	return &Config{
		workflows: make(map[string]*models.Workflow),
		services:  &Services{},
	}
}

// RegisterWorkflow adds a workflow to the configuration's workflow registry.
// It returns an error if a workflow with the same name already exists.
// This allows workflows to be dynamically registered for testing and execution.
func (c *Config) RegisterWorkflow(name string, workflow *models.Workflow) error {
	if _, exists := c.workflows[name]; exists {
		return fmt.Errorf("workflow with name %s already exists", name)
	}
	c.workflows[name] = workflow
	return nil
}

// Core

// GetServices returns the services client implementation that provides access to
// encryption, vault, scheduler, temporal, and LLM services. In this example implementation,
// it returns a basic Services stub.
func (c *Config) GetServices() models.ServicesClientImpl {
	return c.services
}

// GetEnvironment returns the environment configuration including platform type,
// cloud provider settings, and deployment metadata. This stub returns an empty config.
func (c *Config) GetEnvironment() models.EnvironmentConfig {
	return models.EnvironmentConfig{}
}

// GetResumeCallbackUrl returns the callback URL for resuming a paused workflow task.
// Workflows use this URL to signal task completion back to the workflow engine.
// This stub returns an empty string.
func (c *Config) GetResumeCallbackUrl(workflowTask *models.WorkflowTask) string {
	return ""
}

// GetAuthCallbackUrl returns the OAuth callback URL for a provider's authentication flow.
// After a user authenticates with a provider, they are redirected to this URL.
// This stub returns an empty string.
func (c *Config) GetAuthCallbackUrl(providerName string) string {
	return ""
}

// GetSignalCallbackUrl returns the callback URL for sending signals to a running workflow task.
// External systems can use this URL to send events or data to active workflows.
// This stub returns an empty string.
func (c *Config) GetSignalCallbackUrl(workflowTask *models.WorkflowTask) string {
	return ""
}

// GetLoginServerUrl returns the URL of the Thand login server for authentication.
// This is used when the agent needs to authenticate users against a central server.
// This stub returns an empty string.
func (c *Config) GetLoginServerUrl() string {
	return ""
}

// GetLocalServerUrl returns the URL of the local Thand agent server.
// This is used for callbacks and local API endpoints.
// This stub returns an empty string.
func (c *Config) GetLocalServerUrl() string {
	return ""
}

// Roles

// GetCompositeRole merges a base role with identity-specific permissions to create
// a composite role. This is used for role-based access control where user-specific
// attributes affect the final permissions. This stub returns nil.
func (c *Config) GetCompositeRole(identity *models.Identity, baseRole *models.Role) (*models.Role, error) {
	return nil, nil
}

// Identities

// GetIdentity retrieves a user identity by email address. Identities contain
// user information including authentication providers, roles, and permissions.
// This stub returns nil.
func (c *Config) GetIdentity(byEmail string) (*models.Identity, error) {
	return nil, nil
}

// Tenants

// GetTenant retrieves a provider tenant configuration by name. Tenants represent
// multi-tenant configurations where different organizations or teams have isolated
// access to providers. This stub returns nil.
func (c *Config) GetTenant(name string) (*models.ProviderTenant, error) {
	return nil, nil
}

// Workflows

// GetWorkflowByName retrieves a workflow from the registry by its name.
// This is the primary method for looking up workflows that have been registered
// with RegisterWorkflow. Returns an error if the workflow is not found.
func (c *Config) GetWorkflowByName(name string) (*models.Workflow, error) {
	workflow, exists := c.workflows[name]
	if !exists {
		return nil, fmt.Errorf("workflow with name %s not found", name)
	}
	return workflow, nil
}

// GetWorkflowFromElevationRequest determines which workflow should be executed
// based on an elevation request (access request). This maps user access requests
// to the appropriate approval and provisioning workflows. This stub returns nil.
func (c *Config) GetWorkflowFromElevationRequest(elevationRequest *models.ElevateRequest) (*models.Workflow, error) {
	return nil, nil
}

// Providers

// GetProviderByName retrieves a configured provider by its name.
// Providers represent integrations with external services like AWS, Azure, GCP,
// Okta, GitHub, etc. This stub returns nil.
func (c *Config) GetProviderByName(name string) (*models.Provider, error) {
	return nil, nil
}

// GetProvidersByCapability returns all providers that support the specified capabilities.
// Capabilities include features like authentication, user management, role management,
// resource access, etc. This allows workflows to discover available providers dynamically.
// This stub returns nil.
func (c *Config) GetProvidersByCapability(capability ...models.ProviderCapability) map[string]models.Provider {
	return nil
}

// GetProvidersByCapabilityWithUser returns providers filtered by capability and
// user-specific access. This ensures users only see providers they have permission
// to use, based on their identity and configured roles. This stub returns nil.
func (c *Config) GetProvidersByCapabilityWithUser(user *models.User, capability ...models.ProviderCapability) map[string]models.Provider {
	return nil
}
