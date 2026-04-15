package config

import (
	"encoding/json"
	"fmt"
	"net/http"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

type ConfigPatchRequest struct {
	RoleConfig     *RoleConfig                `json:"roles,omitempty"`
	WorkflowConfig *WorkflowConfig            `json:"workflows,omitempty"`
	ProviderConfig *ProviderDefinitionsConfig `json:"providers,omitempty"`
}

func (c *Config) MergeConfiguration(config *RegistrationResponse) error {

	incoming := ConfigPatchRequest{
		RoleConfig:     config.Roles,
		WorkflowConfig: config.Workflows,
		ProviderConfig: config.Providers,
	}

	incomingData, err := json.Marshal(incoming)

	if err != nil {
		logrus.WithError(err).Errorln("Failed to marshal incoming configuration for diffing")
		return err
	}

	roles := c.GetRolesConfig()
	workflows := c.GetWorkflowsConfig()
	providers := c.GetProvidersConfig()

	existing := ConfigPatchRequest{
		RoleConfig:     roles,
		WorkflowConfig: workflows,
		ProviderConfig: providers,
	}

	existingData, err := json.Marshal(existing)

	if err != nil {
		logrus.WithError(err).Errorln("Failed to marshal existing configuration for diffing")
		return err
	}

	// Apply the incoming changes over the existing configurations
	newData, err := jsonpatch.MergePatch(existingData, incomingData)

	if err != nil {
		logrus.WithError(err).Errorln("Failed to create merge patch for configuration diffing")
		return err
	}

	// Convert the merged configuration back to a struct so the in-memory
	// config reflects the fully merged remote+local state rather than a
	// sparse diff payload. Unmarshaling the sparse merge patch here would
	// collapse omitted fields to zero values in typed structs.
	var mergedConfig ConfigPatchRequest
	err = json.Unmarshal(newData, &mergedConfig)

	if err != nil {
		logrus.WithError(err).Errorln("Failed to unmarshal merged configuration")
		return err
	}

	// Apply the desired end state directly. applyPatch remains the helper for
	// callers that actually have partial section diffs to merge locally first.
	err = c.applyMergedConfig(mergedConfig)

	if err != nil {
		logrus.WithError(err).Errorln("Failed to apply incoming configuration patch")
		return err
	}

	// Now we need to figure out what changes exist on the local system that need to
	// be sent back to the server

	outgoingPatch, err := jsonpatch.CreateMergePatch(incomingData, existingData)

	if err != nil {
		logrus.WithError(err).Errorln("Failed to create merge patch for configuration diffing")
		return err
	}

	// Send the outgoing changes back to the server to update its configuration

	go func() {

		logrus.Debugln("Sending configuration updates back to server")

		url := fmt.Sprintf("%s/sync", c.DiscoverThandServerApiUrl())

		authentication := &model.ReferenceableAuthenticationPolicy{
			AuthenticationPolicy: &model.AuthenticationPolicy{
				Bearer: &model.BearerAuthenticationPolicy{
					Token: c.Thand.ApiKey,
				},
			},
		}

		resp, err := common.InvokeHttpRequest(&model.HTTPArguments{
			Method: http.MethodPatch,
			Endpoint: &model.Endpoint{
				EndpointConfig: &model.EndpointConfiguration{
					URI:            &model.LiteralUri{Value: url},
					Authentication: authentication,
				},
			},
			Body: outgoingPatch,
		})

		if err != nil {
			logrus.WithError(err).Errorln("Failed to send configuration updates to server")
			return
		}

		if resp.StatusCode() != http.StatusOK {
			logrus.WithField("status_code", resp.StatusCode()).Errorln("Failed to send configuration updates to server")
		} else {
			logrus.Infoln("Successfully sent configuration updates to server")
		}

	}()

	return nil

}

func (c *Config) applyPatch(diff ConfigPatchRequest) error {
	// applyPatch is the partial-patch helper: merge the incoming section diff
	// with the current live section, then normalize and persist the result.
	// Apply role changes
	if diff.RoleConfig != nil {
		err := c.updateRoles(diff.RoleConfig)
		if err != nil {
			logrus.WithError(err).Errorln("Failed to apply role configuration patch")
			return err
		}
	}

	// Apply workflow changes
	if diff.WorkflowConfig != nil {
		err := c.updateWorkflows(diff.WorkflowConfig)
		if err != nil {
			logrus.WithError(err).Errorln("Failed to apply workflow configuration patch")
			return err
		}
	}

	// Apply provider changes
	if diff.ProviderConfig != nil {
		err := c.updateProviders(diff.ProviderConfig)
		if err != nil {
			logrus.WithError(err).Errorln("Failed to apply provider configuration patch")
			return err
		}
	}

	return nil
}

// applyMergedConfig applies a fully merged server state. It normalizes each
// definitions map and stores it directly without re-merging the same sections.
func (c *Config) applyMergedConfig(config ConfigPatchRequest) error {
	if config.RoleConfig != nil {
		if err := c.storeRoleDefinitions(config.RoleConfig.Definitions); err != nil {
			logrus.WithError(err).Errorln("Failed to apply merged role configuration")
			return err
		}
	}

	if config.WorkflowConfig != nil {
		if err := c.storeWorkflowDefinitions(config.WorkflowConfig.Definitions); err != nil {
			logrus.WithError(err).Errorln("Failed to apply merged workflow configuration")
			return err
		}
	}

	if config.ProviderConfig != nil {
		if err := c.storeProviderDefinitions(config.ProviderConfig.Definitions); err != nil {
			logrus.WithError(err).Errorln("Failed to apply merged provider configuration")
			return err
		}
	}

	return nil
}

func mergeConfigSection(current any, incoming any, out any) error {
	currentData, err := json.Marshal(current)
	if err != nil {
		return err
	}

	incomingData, err := json.Marshal(incoming)
	if err != nil {
		return err
	}

	mergedData, err := jsonpatch.MergePatch(currentData, incomingData)
	if err != nil {
		return err
	}

	return json.Unmarshal(mergedData, out)
}

func (c *Config) updateRoles(roleConfig *RoleConfig) error {
	c.mu.RLock()
	current := RoleConfig{
		Path:        c.Roles.Path,
		URL:         c.Roles.URL,
		Vault:       c.Roles.Vault,
		Definitions: c.Roles.Definitions,
	}
	c.mu.RUnlock()

	var merged RoleConfig
	if err := mergeConfigSection(current, *roleConfig, &merged); err != nil {
		return err
	}

	return c.storeRoleDefinitions(merged.Definitions)
}

func (c *Config) updateWorkflows(workflowConfig *WorkflowConfig) error {
	c.mu.RLock()
	current := WorkflowConfig{
		Path:        c.Workflows.Path,
		URL:         c.Workflows.URL,
		Vault:       c.Workflows.Vault,
		Plugins:     c.Workflows.Plugins,
		Definitions: c.Workflows.Definitions,
	}
	c.mu.RUnlock()

	var merged WorkflowConfig
	if err := mergeConfigSection(current, *workflowConfig, &merged); err != nil {
		return err
	}

	return c.storeWorkflowDefinitions(merged.Definitions)
}

func (c *Config) updateProviders(providerConfig *ProviderDefinitionsConfig) error {
	c.mu.RLock()
	current := ProviderDefinitionsConfig{
		Path:        c.Providers.Path,
		URL:         c.Providers.URL,
		Vault:       c.Providers.Vault,
		Plugins:     c.Providers.Plugins,
		Definitions: c.Providers.Definitions,
	}
	c.mu.RUnlock()

	var merged ProviderDefinitionsConfig
	if err := mergeConfigSection(current, *providerConfig, &merged); err != nil {
		return err
	}

	return c.storeProviderDefinitions(merged.Definitions)
}

func (c *Config) storeRoleDefinitions(definitions map[string]models.Role) error {
	defs, err := c.ApplyRoles([]*models.RoleDefinitions{{
		Roles: definitions,
	}})
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.Roles.Definitions = defs
	c.mu.Unlock()

	return nil
}

func (c *Config) storeWorkflowDefinitions(definitions map[string]models.Workflow) error {
	defs, err := c.ApplyWorkflows([]*models.WorkflowDefinitions{{
		Workflows: definitions,
	}})
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.Workflows.Definitions = defs
	c.mu.Unlock()

	return nil
}

func (c *Config) storeProviderDefinitions(definitions map[string]models.ProviderConfig) error {
	defs, err := c.ApplyProviders([]*models.ProviderDefinitions{{
		Providers: definitions,
	}})
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.Providers.Definitions = defs
	c.mu.Unlock()

	return nil
}
