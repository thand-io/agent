package config

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/workflow"
)

// Register temporal workflows and activities
func (c *Config) registerTemporalWorkflows() error {
	if c.servicesClient == nil || c.servicesClient.GetTemporal() == nil {
		return fmt.Errorf("temporal service is not initialized")
	}

	temporalService := c.servicesClient.GetTemporal()
	temporalClient := temporalService.GetClient()
	temporalWorker := temporalService.GetWorker()

	if temporalWorker == nil {
		return fmt.Errorf("temporal worker is not initialized")
	}

	// Use the system id as the workflow id to ensure no other workflows
	// use the same id.
	systemID := common.GetClientIdentifier()

	ctx := context.Background()

	startOptions := client.StartWorkflowOptions{
		ID:                       systemID.String(),
		TaskQueue:                temporalService.GetTaskQueue(),
		WorkflowIDReusePolicy:    enums.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE,
		WorkflowIDConflictPolicy: enums.WORKFLOW_ID_CONFLICT_POLICY_USE_EXISTING,
	}

	if c.IsServer() {

		logrus.Infoln("Registering server workflow", "workflowId", systemID.String())

		temporalWorker.RegisterWorkflowWithOptions(
			CreateServerWorkflow(),
			workflow.RegisterOptions{
				Name:               "server-workflow",
				VersioningBehavior: workflow.VersioningBehaviorPinned,
			},
		)

		if _, err := temporalClient.ExecuteWorkflow(
			ctx,
			startOptions,
			"server-workflow",
			ServerWorkflowStart{
				ThandSystemStart: ThandSystemStart{
					Identities: []string{},
				},
			},
		); err != nil {
			logrus.WithError(err).Errorf("Failed to start server workflow: %v", err)
			return fmt.Errorf("failed to start server workflow: %w", err)
		}

	} else if c.IsAgent() || c.IsClient() {

		logrus.Infoln("Registering agent workflow", "workflowId", systemID.String())

		// Get the registered identities on the system and bind them

		temporalWorker.RegisterWorkflowWithOptions(
			CreateAgentWorkflow(),
			workflow.RegisterOptions{
				Name:               "agent-workflow",
				VersioningBehavior: workflow.VersioningBehaviorPinned,
			},
		)

		if _, err := temporalClient.ExecuteWorkflow(
			ctx,
			startOptions,
			"agent-workflow",
			AgentWorkflowStart{
				ThandSystemStart: ThandSystemStart{
					Identities: []string{
						"hugh@thand.io",
					},
				},
			},
		); err != nil {
			logrus.WithError(err).Errorf("Failed to start agent workflow: %v", err)
			return fmt.Errorf("failed to start agent workflow: %w", err)
		}

	}

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
