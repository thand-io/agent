package config

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
	"go.temporal.io/api/enums/v1"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/worker"
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

	// When worker versioning is enabled, pin the workflow to this worker's
	// deployment version at start time. Without an override, the server has
	// no deployment routing info on the WorkflowExecutionStarted event and
	// the first workflow task can sit unscheduled until the deployment
	// becomes "current" — which blocks search-attribute upserts and update
	// handlers from ever running. This mirrors the pattern used in
	// sdk/workflows/manager/manager.go.
	if !temporalService.IsVersioningDisabled() {
		startOptions.VersioningOverride = &client.PinnedVersioningOverride{
			Version: worker.WorkerDeploymentVersion{
				DeploymentName: sdkConstants.TemporalDeploymentName,
				BuildID:        common.GetBuildIdentifier(),
			},
		}
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

		agentIdentities := []string{
			"hugh@thand.io",
		}

		startInput := AgentWorkflowStart{
			ThandSystemStart: ThandSystemStart{
				Identities: agentIdentities,
			},
		}

		logrus.WithFields(logrus.Fields{
			"workflowId":       systemID.String(),
			"taskQueue":        temporalService.GetTaskQueue(),
			"identities":       agentIdentities,
			"identitiesCount":  len(agentIdentities),
			"conflictPolicy":   startOptions.WorkflowIDConflictPolicy.String(),
			"reusePolicy":      startOptions.WorkflowIDReusePolicy.String(),
			"startInputStruct": fmt.Sprintf("%+v", startInput),
		}).Info("Starting agent workflow with identities")

		run, err := temporalClient.ExecuteWorkflow(
			ctx,
			startOptions,
			"agent-workflow",
			startInput,
		)
		if err != nil {
			logrus.WithError(err).Errorf("Failed to start agent workflow: %v", err)
			return fmt.Errorf("failed to start agent workflow: %w", err)
		}

		logrus.WithFields(logrus.Fields{
			"workflowId": run.GetID(),
			"runId":      run.GetRunID(),
			"identities": agentIdentities,
		}).Info("Agent workflow start request accepted; pushing identities via update to handle USE_EXISTING reuse")

		// Because WorkflowIDConflictPolicy is USE_EXISTING, the start args
		// above are ignored when an agent workflow with the same ID is
		// already running. Send an update so the running workflow always
		// reflects the latest identities.
		//
		// IMPORTANT: this must run asynchronously. registerTemporalWorkflows
		// is called BEFORE StartTemporalWorkers, so the local worker isn't
		// polling yet. A synchronous UpdateWorkflow call (WaitForStage
		// Completed/Accepted) blocks waiting for a workflow task to be
		// processed, which never happens — causing a startup deadlock and
		// preventing the worker from ever starting.
		go func(workflowID, runID string, identities []string) {
			updateCtx := context.Background()
			updateHandle, err := temporalClient.UpdateWorkflow(updateCtx, client.UpdateWorkflowOptions{
				WorkflowID:   workflowID,
				RunID:        runID,
				UpdateName:   models.TemporalSystemUpdateIdentitiesUpdateName,
				Args:         []any{identities},
				WaitForStage: client.WorkflowUpdateStageCompleted,
			})
			if err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"workflowId": workflowID,
					"runId":      runID,
					"identities": identities,
				}).Warn("Failed to send updateIdentities to agent workflow")
				return
			}
			var updatedIdentities []string
			if err := updateHandle.Get(updateCtx, &updatedIdentities); err != nil {
				logrus.WithError(err).WithFields(logrus.Fields{
					"workflowId": workflowID,
					"runId":      runID,
				}).Warn("updateIdentities completed with error")
				return
			}
			logrus.WithFields(logrus.Fields{
				"workflowId":        workflowID,
				"runId":             runID,
				"updatedIdentities": updatedIdentities,
			}).Info("updateIdentities completed successfully")
		}(run.GetID(), run.GetRunID(), agentIdentities)
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
