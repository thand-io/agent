package config

import (
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

// LoadWorkflows loads workflows from a file or URL
func (c *Config) LoadWorkflows() (map[string]models.Workflow, error) {

	vaultData := ""

	if len(c.Workflows.Vault) > 0 {

		if !c.HasVault() {
			return nil, fmt.Errorf("vault configuration is missing. Cannot load roles from vault")
		}

		logrus.Debugln("Loading workflows from vault: ", c.Workflows.Vault)

		// Load workflows from Vault
		data, err := c.GetVault().GetSecret(c.Workflows.Vault)
		if err != nil {
			logrus.WithError(err).Errorln("Error loading workflows from vault")
			return nil, fmt.Errorf("failed to get secret from vault: %w", err)
		}

		logrus.Debugln("Loaded workflows from vault: ", len(data), " bytes")

		vaultData = string(data)
	}

	foundWorkflows, err := loadDataFromSource(
		c.Workflows.Path,
		c.Workflows.URL,
		vaultData,
		WorkflowDefinitions{},
	)
	if err != nil {
		logrus.WithError(err).Errorln("Failed to load workflows data")
		return nil, fmt.Errorf("failed to load workflows data: %w", err)
	}

	defs := make(map[string]models.Workflow)

	logrus.Debugln("Processing loaded workflows: ", len(foundWorkflows))

	for _, workflow := range foundWorkflows {
		for workflowKey, p := range workflow.Workflows {

			if !p.Enabled {
				logrus.Infoln("Workflow disabled:", workflowKey)
				continue
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
