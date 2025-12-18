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
	RoleConfig     *RoleConfig     `json:"roles,omitempty"`
	WorkflowConfig *WorkflowConfig `json:"workflows,omitempty"`
	ProviderConfig *ProviderConfig `json:"providers,omitempty"`
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

	roles := c.GetRoles()
	workflows := c.GetWorkflows()
	providers := c.GetProviders()

	existing := ConfigPatchRequest{
		RoleConfig:     &roles,
		WorkflowConfig: &workflows,
		ProviderConfig: &providers,
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

	// Create a patch to see the differences between existing and new
	incomingPatch, err := jsonpatch.CreateMergePatch(existingData, newData)

	if err != nil {
		logrus.WithError(err).Errorln("Failed to create merge patch for configuration diffing")
		return err
	}

	// Convert patches back to structs - these are the NEW changes from the remote
	// server that we need to apply to our existing configuration
	var incomingDiff ConfigPatchRequest
	err = json.Unmarshal(incomingPatch, &incomingDiff)

	if err != nil {
		logrus.WithError(err).Errorln("Failed to unmarshal incoming patch")
		return err
	}

	// Add these new changes to our existing configuration
	err = c.applyPatch(incomingDiff)

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

func (c *Config) updateRoles(roleConfig *RoleConfig) error {
	_, err := c.ApplyRoles([]*models.RoleDefinitions{{
		Roles: roleConfig.Definitions,
	}})
	return err
}

func (c *Config) updateWorkflows(workflowConfig *WorkflowConfig) error {
	_, err := c.ApplyWorkflows([]*models.WorkflowDefinitions{{
		Workflows: workflowConfig.Definitions,
	}})
	return err
}

func (c *Config) updateProviders(providerConfig *ProviderConfig) error {
	_, err := c.ApplyProviders([]*models.ProviderDefinitions{{
		Providers: providerConfig.Definitions,
	}})
	return err
}
