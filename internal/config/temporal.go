package config

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/workflow"
)

// Register temporal workflows and activities
func (c *Config) registerTemporalWorkflows() error {
	if c.servicesClient == nil || c.servicesClient.GetTemporal() == nil {
		return fmt.Errorf("temporal service is not initialized")
	}

	temporalClient := c.servicesClient.GetTemporal().GetClient()
	temporalWorker := c.servicesClient.GetTemporal().GetWorker()

	if temporalWorker == nil {
		return fmt.Errorf("temporal worker is not initialized")
	}

	// Use the system id as the workflow id to ensure no other workflows
	// use the same id.
	systemID := common.GetClientIdentifier()

	if c.IsServer() {

		temporalWorker.RegisterWorkflowWithOptions(
			CreateServerWorkflow(c, ServerWorkflowStart{
				ThandSystemStart: ThandSystemStart{
					Identities: []string{
						systemID.String(),
					},
				},
			}),
			workflow.RegisterOptions{
				Name:               systemID.String(),
				VersioningBehavior: workflow.VersioningBehaviorPinned,
			},
		)

	} else if c.IsAgent() {

		// Get the registered identities on the system and bind them

		temporalWorker.RegisterWorkflowWithOptions(
			CreateAgentWorkflow(c, AgentWorkflowStart{
				ThandSystemStart: ThandSystemStart{
					Identities: []string{
						systemID.String(),
					},
				},
			}),
			workflow.RegisterOptions{
				Name:               systemID.String(),
				VersioningBehavior: workflow.VersioningBehaviorPinned,
			},
		)
	}

	// Signal the workflow with a heartbeat
	temporalClient.SignalWorkflow(
		context.Background(),
		systemID.String(),
		"",
		"heartbeat",
		map[string]string{},
	)

	return nil

}

func (c *Config) registerTemporalActivities() error {
	if c.servicesClient == nil || c.servicesClient.GetTemporal() == nil {
		return fmt.Errorf("temporal service is not initialized")
	}

	temporalWorker := c.servicesClient.GetTemporal().GetWorker()

	if temporalWorker == nil {
		return fmt.Errorf("temporal worker is not initialized")
	}

	thandActivities := &thandActivities{
		config: c,
	}

	logrus.Info("Registering system identifier lookup")

	temporalWorker.RegisterActivityWithOptions(
		thandActivities.LookupSystemIdentifier,
		activity.RegisterOptions{
			Name: models.TemporalLookupSystemIdentifierActivityName,
		},
	)

	if c.HasThandService() {

		logrus.Info("Registering upstream patching activities for Thand service")

		temporalWorker.RegisterActivityWithOptions(
			thandActivities.PatchProviderUpstream,
			activity.RegisterOptions{
				Name: models.TemporalPatchProviderUpstreamActivityName,
			},
		)

	} else {

		logrus.Info("Registering dummy upstream patching activities (Thand service not configured)")

		temporalWorker.RegisterActivityWithOptions(
			thandActivities.PatchProviderUpstreamDummy,
			activity.RegisterOptions{
				Name: models.TemporalPatchProviderUpstreamActivityName,
			},
		)
	}

	return nil

}
