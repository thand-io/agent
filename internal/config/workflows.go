package config

import (
	"fmt"

	"github.com/hashicorp/go-version"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/config/environment"
	"github.com/thand-io/agent/internal/models"
)

// LoadWorkflows loads workflows from a file or URL
func (c *Config) LoadWorkflows() (map[string]models.Workflow, error) {

	vaultData, err := c.loadWorkflowsVaultData()

	if err != nil {
		return nil, err
	}

	foundWorkflows := []*models.WorkflowDefinitions{}

	if len(vaultData) > 0 || len(c.Workflows.Path) > 0 || c.Workflows.URL != nil {

		importedWorkflows, err := loadDataFromSource(
			c.Workflows.Path,
			c.Workflows.URL,
			vaultData,
			models.WorkflowDefinitions{},
		)

		if err != nil {
			logrus.WithError(err).Errorln("Failed to load workflows data")
			return nil, fmt.Errorf("failed to load workflows data: %w", err)
		}

		foundWorkflows = importedWorkflows

	}

	if len(foundWorkflows) == 0 {
		logrus.Warningln("No workflows found from any source, loading defaults")
		foundWorkflows, err = environment.GetDefaultWorkflows(c.Environment.Platform)
		if err != nil {
			return nil, fmt.Errorf("failed to load default workflows: %w", err)
		}
		logrus.Infoln("Loaded default workflows:", len(foundWorkflows))
	}

	return c.ApplyWorkflows(foundWorkflows)
}

func (c *Config) ApplyWorkflows(foundWorkflows []*models.WorkflowDefinitions) (map[string]models.Workflow, error) {

	// Add workflows defined directly in config
	if len(c.Workflows.Definitions) > 0 {

		logrus.Debugln("Adding workflows defined directly in config: ", len(c.Workflows.Definitions))

		defaultVersion := version.Must(version.NewVersion("1.0"))

		for workflowKey, workflow := range c.Workflows.Definitions {
			foundWorkflows = append(foundWorkflows, &models.WorkflowDefinitions{
				Version: defaultVersion,
				Workflows: map[string]models.Workflow{
					workflowKey: workflow,
				},
			})
		}
	}

	defs := make(map[string]models.Workflow)

	logrus.Debugln("Processing loaded workflows: ", len(foundWorkflows))

	for _, workflow := range foundWorkflows {
		for workflowKey, p := range workflow.Workflows {

			if !p.Enabled {
				logrus.Infoln("Workflow disabled:", workflowKey)
				continue
			}

			if p.Version == nil {
				p.Version = workflow.Version
			}

			if len(p.Name) == 0 {
				p.Name = workflowKey
			}

			if _, exists := defs[workflowKey]; exists {
				logrus.Warningln("Duplicate workflow key found, skipping:", workflowKey)
				continue
			}

			defs[workflowKey] = p
		}
	}

	return defs, nil
}

// loadWorkflowsVaultData loads workflow data from vault if configured
func (c *Config) loadWorkflowsVaultData() (string, error) {

	if len(c.Workflows.Vault) == 0 {
		return "", nil
	}

	if !c.HasVault() {
		return "", fmt.Errorf("vault configuration is missing. Cannot load workflows from vault")
	}

	logrus.Debugln("Loading workflows from vault: ", c.Workflows.Vault)

	// Load workflows from Vault
	data, err := c.GetVault().GetSecret(c.Workflows.Vault)
	if err != nil {
		logrus.WithError(err).Errorln("Error loading workflows from vault")
		return "", fmt.Errorf("failed to get secret from vault: %w", err)
	}

	logrus.Debugln("Loaded workflows from vault: ", len(data), " bytes")

	return string(data), nil
}
