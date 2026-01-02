package examples

import (
	"fmt"

	"github.com/thand-io/agent/internal/models"
)

type Config struct {
	// Workflows
	workflows map[string]*models.Workflow
	services  *Services
}

func NewConfig() *Config {
	return &Config{
		workflows: make(map[string]*models.Workflow),
		services:  &Services{},
	}
}

func (c *Config) RegisterWorkflow(name string, workflow *models.Workflow) error {
	if _, exists := c.workflows[name]; exists {
		return fmt.Errorf("workflow with name %s already exists", name)
	}
	c.workflows[name] = workflow
	return nil
}

// Core
func (c *Config) GetServices() models.ServicesClientImpl {
	return c.services
}

func (c *Config) GetEnvironment() models.EnvironmentConfig {
	return models.EnvironmentConfig{}
}

func (c *Config) GetResumeCallbackUrl(workflowTask *models.WorkflowTask) string {
	return ""
}

func (c *Config) GetAuthCallbackUrl(providerName string) string {
	return ""
}

func (c *Config) GetSignalCallbackUrl(workflowTask *models.WorkflowTask) string {
	return ""
}

func (c *Config) GetLoginServerUrl() string {
	return ""
}

func (c *Config) GetLocalServerUrl() string {
	return ""
}

// Roles
func (c *Config) GetCompositeRole(identity *models.Identity, baseRole *models.Role) (*models.Role, error) {
	return nil, nil
}

// Identities
func (c *Config) GetIdentity(byEmail string) (*models.Identity, error) {
	return nil, nil
}

// Tenants
func (c *Config) GetTenant(name string) (*models.ProviderTenant, error) {
	return nil, nil
}

// Workflows
func (c *Config) GetWorkflowByName(name string) (*models.Workflow, error) {
	workflow, exists := c.workflows[name]
	if !exists {
		return nil, fmt.Errorf("workflow with name %s not found", name)
	}
	return workflow, nil
}

func (c *Config) GetWorkflowFromElevationRequest(elevationRequest *models.ElevateRequest) (*models.Workflow, error) {
	return nil, nil
}

// Providers
func (c *Config) GetProviderByName(name string) (*models.Provider, error) {
	return nil, nil
}

func (c *Config) GetProvidersByCapability(capability ...models.ProviderCapability) map[string]models.Provider {
	return nil
}

func (c *Config) GetProvidersByCapabilityWithUser(user *models.User, capability ...models.ProviderCapability) map[string]models.Provider {
	return nil
}
