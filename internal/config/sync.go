package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"

	jsonpatch "github.com/evanphx/json-patch"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

const maxMergeConfigurationRetries = 3

type configPatchSnapshot struct {
	generation              uint64
	request                 ConfigPatchRequest
	data                    []byte
	roleDefinitionsJSON     []byte
	workflowDefinitionsJSON []byte
	providerDefinitionsJSON []byte
}

type buildMergedConfigResult struct {
	config        ConfigPatchRequest
	outgoingPatch []byte
}

type normalizedDefinitionsPatch struct {
	roleDefinitions            map[string]models.Role
	roleDefinitionsJSON        []byte
	roleDefinitionsChanged     bool
	workflowDefinitions        map[string]models.Workflow
	workflowDefinitionsJSON    []byte
	workflowDefinitionsChanged bool
	providerDefinitions        map[string]models.ProviderConfig
	providerDefinitionsJSON    []byte
	providerDefinitionsChanged bool
}

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

	outgoingPatch, err := c.applyMergedConfigWithRetries(func(snapshot *configPatchSnapshot) (*buildMergedConfigResult, error) {
		// Apply the incoming changes over the existing configurations.
		newData, err := jsonpatch.MergePatch(snapshot.data, incomingData)
		if err != nil {
			logrus.WithError(err).Errorln("Failed to create merge patch for configuration diffing")
			return nil, err
		}

		// Convert the merged configuration back to a struct so the in-memory
		// config reflects the fully merged remote+local state rather than a
		// sparse diff payload. Unmarshaling the sparse merge patch here would
		// collapse omitted fields to zero values in typed structs.
		var mergedConfig ConfigPatchRequest
		err = json.Unmarshal(newData, &mergedConfig)
		if err != nil {
			logrus.WithError(err).Errorln("Failed to unmarshal merged configuration")
			return nil, err
		}

		// Now we need to figure out what changes exist on the local system that need to
		// be sent back to the server
		outgoingPatch, err := jsonpatch.CreateMergePatch(incomingData, snapshot.data)
		if err != nil {
			logrus.WithError(err).Errorln("Failed to create merge patch for configuration diffing")
			return nil, err
		}

		return &buildMergedConfigResult{
			config:        mergedConfig,
			outgoingPatch: outgoingPatch,
		}, nil
	})
	if err != nil {
		return err
	}

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

func (c *Config) applyMergedConfigWithRetries(build func(snapshot *configPatchSnapshot) (*buildMergedConfigResult, error)) ([]byte, error) {
	for attempt := range maxMergeConfigurationRetries {
		snapshot, err := c.snapshotConfigPatch()
		if err != nil {
			logrus.WithError(err).Errorln("Failed to marshal existing configuration for diffing")
			return nil, err
		}

		result, err := build(snapshot)
		if err != nil {
			return nil, err
		}

		applied, err := c.applyMergedConfigWithSnapshot(snapshot, result.config)
		if err != nil {
			logrus.WithError(err).Errorln("Failed to apply incoming merged configuration")
			return nil, err
		}
		if applied {
			return result.outgoingPatch, nil
		}

		logrus.WithField("attempt", attempt+1).Infoln("Configuration changed during merged sync apply, retrying")
	}

	logrus.WithField("attempts", maxMergeConfigurationRetries).Warnln("Configuration changed during every merged sync attempt")
	return nil, fmt.Errorf("configuration changed during merge after %d attempts", maxMergeConfigurationRetries)
}

func (c *Config) snapshotConfigPatch() (*configPatchSnapshot, error) {
	c.mu.RLock()
	snapshot := ConfigPatchRequest{
		RoleConfig: &RoleConfig{
			Path:        c.Roles.Path,
			URL:         c.Roles.URL,
			Vault:       c.Roles.Vault,
			Definitions: c.Roles.Definitions,
		},
		WorkflowConfig: &WorkflowConfig{
			Path:        c.Workflows.Path,
			URL:         c.Workflows.URL,
			Vault:       c.Workflows.Vault,
			Plugins:     c.Workflows.Plugins,
			Definitions: c.Workflows.Definitions,
		},
		ProviderConfig: &ProviderDefinitionsConfig{
			Path:        c.Providers.Path,
			URL:         c.Providers.URL,
			Vault:       c.Providers.Vault,
			Plugins:     c.Providers.Plugins,
			Definitions: c.Providers.Definitions,
		},
	}
	generation := c.configGeneration

	data, err := json.Marshal(snapshot)
	c.mu.RUnlock()
	if err != nil {
		return nil, err
	}

	// Keep sync retries isolated from in-place nested mutations by detaching the
	// snapshot through the same JSON representation used for merge-patch diffing.
	var detached ConfigPatchRequest
	if err := json.Unmarshal(data, &detached); err != nil {
		return nil, err
	}

	roleDefinitionsJSON, err := marshalJSON(detachedRoleDefinitions(detached.RoleConfig))
	if err != nil {
		return nil, err
	}

	workflowDefinitionsJSON, err := marshalJSON(detachedWorkflowDefinitions(detached.WorkflowConfig))
	if err != nil {
		return nil, err
	}

	providerDefinitionsJSON, err := marshalJSON(detachedProviderDefinitions(detached.ProviderConfig))
	if err != nil {
		return nil, err
	}

	return &configPatchSnapshot{
		generation:              generation,
		request:                 detached,
		data:                    data,
		roleDefinitionsJSON:     roleDefinitionsJSON,
		workflowDefinitionsJSON: workflowDefinitionsJSON,
		providerDefinitionsJSON: providerDefinitionsJSON,
	}, nil
}

func (c *Config) applyPatch(diff ConfigPatchRequest) error {
	// applyPatch is the partial-patch helper: merge the incoming section diff
	// with the current live section, then normalize and persist the result.
	if diff.RoleConfig != nil {
		err := c.updateRoles(diff.RoleConfig)
		if err != nil {
			logrus.WithError(err).Errorln("Failed to apply role configuration patch")
			return err
		}
	}

	if diff.WorkflowConfig != nil {
		err := c.updateWorkflows(diff.WorkflowConfig)
		if err != nil {
			logrus.WithError(err).Errorln("Failed to apply workflow configuration patch")
			return err
		}
	}

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
	snapshot, err := c.snapshotConfigPatch()
	if err != nil {
		return err
	}

	applied, err := c.applyMergedConfigWithSnapshot(snapshot, config)
	if err != nil {
		return err
	}
	if !applied {
		return fmt.Errorf("configuration changed while applying merged configuration")
	}

	return nil
}

func (c *Config) applyMergedConfigWithSnapshot(snapshot *configPatchSnapshot, config ConfigPatchRequest) (bool, error) {
	normalized, err := c.normalizeMergedConfig(snapshot, config)
	if err != nil {
		return false, err
	}

	return c.commitMergedDefinitions(normalized, snapshot.generation), nil
}

func (c *Config) normalizeMergedConfig(snapshot *configPatchSnapshot, config ConfigPatchRequest) (*normalizedDefinitionsPatch, error) {
	normalized := &normalizedDefinitionsPatch{}

	if config.RoleConfig != nil {
		mergedRoleDefinitionsJSON, err := marshalJSON(detachedRoleDefinitions(config.RoleConfig))
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(snapshot.roleDefinitionsJSON, mergedRoleDefinitionsJSON) {
			defs, defsJSON, err := c.normalizeRoleDefinitions(detachedRoleDefinitions(config.RoleConfig))
			if err != nil {
				logrus.WithError(err).Errorln("Failed to normalize merged role configuration")
				return nil, err
			}
			normalized.roleDefinitionsJSON = defsJSON
			normalized.roleDefinitionsChanged = !bytes.Equal(snapshot.roleDefinitionsJSON, defsJSON)
			if normalized.roleDefinitionsChanged {
				normalized.roleDefinitions = defs
			}
		}
	}

	if config.WorkflowConfig != nil {
		mergedWorkflowDefinitionsJSON, err := marshalJSON(detachedWorkflowDefinitions(config.WorkflowConfig))
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(snapshot.workflowDefinitionsJSON, mergedWorkflowDefinitionsJSON) {
			defs, defsJSON, err := c.normalizeWorkflowDefinitions(detachedWorkflowDefinitions(config.WorkflowConfig))
			if err != nil {
				logrus.WithError(err).Errorln("Failed to normalize merged workflow configuration")
				return nil, err
			}
			normalized.workflowDefinitionsJSON = defsJSON
			normalized.workflowDefinitionsChanged = !bytes.Equal(snapshot.workflowDefinitionsJSON, defsJSON)
			if normalized.workflowDefinitionsChanged {
				normalized.workflowDefinitions = defs
			}
		}
	}

	if config.ProviderConfig != nil {
		mergedProviderDefinitionsJSON, err := marshalJSON(detachedProviderDefinitions(config.ProviderConfig))
		if err != nil {
			return nil, err
		}
		if !bytes.Equal(snapshot.providerDefinitionsJSON, mergedProviderDefinitionsJSON) {
			defs, defsJSON, err := c.normalizeProviderDefinitions(detachedProviderDefinitions(config.ProviderConfig))
			if err != nil {
				logrus.WithError(err).Errorln("Failed to normalize merged provider configuration")
				return nil, err
			}
			normalized.providerDefinitionsJSON = defsJSON
			normalized.providerDefinitionsChanged = !bytes.Equal(snapshot.providerDefinitionsJSON, defsJSON)
			if normalized.providerDefinitionsChanged {
				normalized.providerDefinitions = defs
			}
		}
	}

	return normalized, nil
}

func (c *Config) commitMergedDefinitions(diff *normalizedDefinitionsPatch, expectedGeneration uint64) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.configGeneration != expectedGeneration {
		return false
	}

	changed := false
	if diff.roleDefinitionsChanged {
		c.Roles.Definitions = diff.roleDefinitions
		changed = true
	}
	if diff.workflowDefinitionsChanged {
		c.Workflows.Definitions = diff.workflowDefinitions
		changed = true
	}
	if diff.providerDefinitionsChanged {
		c.Providers.Definitions = diff.providerDefinitions
		changed = true
	}
	if changed {
		c.configGeneration++
	}

	return true
}

func marshalJSON(value any) ([]byte, error) {
	return json.Marshal(value)
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
	defs, defsJSON, err := c.normalizeRoleDefinitions(definitions)
	if err != nil {
		return err
	}

	return c.commitRoleDefinitions(defs, defsJSON)
}

func (c *Config) storeWorkflowDefinitions(definitions map[string]models.Workflow) error {
	defs, defsJSON, err := c.normalizeWorkflowDefinitions(definitions)
	if err != nil {
		return err
	}

	return c.commitWorkflowDefinitions(defs, defsJSON)
}

func (c *Config) storeProviderDefinitions(definitions map[string]models.ProviderConfig) error {
	defs, defsJSON, err := c.normalizeProviderDefinitions(definitions)
	if err != nil {
		return err
	}

	return c.commitProviderDefinitions(defs, defsJSON)
}

func (c *Config) normalizeRoleDefinitions(definitions map[string]models.Role) (map[string]models.Role, []byte, error) {
	defs, err := (&Config{}).ApplyRoles([]*models.RoleDefinitions{{
		Roles: definitions,
	}})
	if err != nil {
		return nil, nil, err
	}

	defsJSON, err := marshalJSON(defs)
	if err != nil {
		return nil, nil, err
	}

	return defs, defsJSON, nil
}

func (c *Config) normalizeWorkflowDefinitions(definitions map[string]models.Workflow) (map[string]models.Workflow, []byte, error) {
	defs, err := (&Config{mode: c.mode}).ApplyWorkflows([]*models.WorkflowDefinitions{{
		Workflows: definitions,
	}})
	if err != nil {
		return nil, nil, err
	}

	defsJSON, err := marshalJSON(defs)
	if err != nil {
		return nil, nil, err
	}

	return defs, defsJSON, nil
}

func (c *Config) normalizeProviderDefinitions(definitions map[string]models.ProviderConfig) (map[string]models.ProviderConfig, []byte, error) {
	defs, err := (&Config{}).ApplyProviders([]*models.ProviderDefinitions{{
		Providers: definitions,
	}})
	if err != nil {
		return nil, nil, err
	}

	defsJSON, err := marshalJSON(defs)
	if err != nil {
		return nil, nil, err
	}

	return defs, defsJSON, nil
}

func (c *Config) commitRoleDefinitions(defs map[string]models.Role, defsJSON []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	currentJSON, err := marshalJSON(c.Roles.Definitions)
	if err != nil {
		return err
	}
	if bytes.Equal(currentJSON, defsJSON) {
		return nil
	}

	c.Roles.Definitions = defs
	c.configGeneration++
	return nil
}

func (c *Config) commitWorkflowDefinitions(defs map[string]models.Workflow, defsJSON []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	currentJSON, err := marshalJSON(c.Workflows.Definitions)
	if err != nil {
		return err
	}
	if bytes.Equal(currentJSON, defsJSON) {
		return nil
	}

	c.Workflows.Definitions = defs
	c.configGeneration++
	return nil
}

func (c *Config) commitProviderDefinitions(defs map[string]models.ProviderConfig, defsJSON []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	currentJSON, err := marshalJSON(c.Providers.Definitions)
	if err != nil {
		return err
	}
	if bytes.Equal(currentJSON, defsJSON) {
		return nil
	}

	c.Providers.Definitions = defs
	c.configGeneration++
	return nil
}

func detachedRoleDefinitions(config *RoleConfig) map[string]models.Role {
	if config == nil {
		return nil
	}
	return config.Definitions
}

func detachedWorkflowDefinitions(config *WorkflowConfig) map[string]models.Workflow {
	if config == nil {
		return nil
	}
	return config.Definitions
}

func detachedProviderDefinitions(config *ProviderDefinitionsConfig) map[string]models.ProviderConfig {
	if config == nil {
		return nil
	}
	return config.Definitions
}
