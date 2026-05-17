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
	"go.temporal.io/sdk/workflow"
)

// registerTemporalWorkflows registers workflow types with the local worker.
//
// This only registers workflow definitions; it does NOT start any workflow
// runs. Workflow execution is deferred to startSystemWorkflow which must
// run AFTER StartTemporalWorkers — when worker versioning is enabled, the
// pinned deployment version is only "present" on the task queue once the
// worker has polled and Temporal has registered the version, so starting
// a pinned workflow before workers run fails with
// "Pinned version ... is not present in task queue".
func (c *Config) registerTemporalWorkflows() error {
	if c.servicesClient == nil || c.servicesClient.GetTemporal() == nil {
		return fmt.Errorf("temporal service is not initialized")
	}

	temporalService := c.servicesClient.GetTemporal()
	temporalWorker := temporalService.GetWorker()

	if temporalWorker == nil {
		return fmt.Errorf("temporal worker is not initialized")
	}

	systemID := common.GetClientIdentifier()

	if c.IsServer() {

		logrus.Infoln("Registering server workflow", "workflowId", systemID.String())

		// AutoUpgrade so the long-running per-system workflow transitions
		// to the deployment's current Build ID on the next workflow task
		// after a binary upgrade. Pinning would strand a running execution
		// on the previous BuildID and break ping/update once that worker
		// stops. Any non-deterministic change to the workflow body must be
		// guarded with workflow.GetVersion / patches.
		temporalWorker.RegisterWorkflowWithOptions(
			CreateServerWorkflow(),
			workflow.RegisterOptions{
				Name:               "server-workflow",
				VersioningBehavior: workflow.VersioningBehaviorAutoUpgrade,
			},
		)

	} else if c.IsAgent() || c.IsClient() {

		logrus.Infoln("Registering agent workflow", "workflowId", systemID.String())

		// AutoUpgrade: see server-workflow comment above.
		temporalWorker.RegisterWorkflowWithOptions(
			CreateAgentWorkflow(),
			workflow.RegisterOptions{
				Name:               "agent-workflow",
				VersioningBehavior: workflow.VersioningBehaviorAutoUpgrade,
			},
		)
	}

	return nil
}

// StartSystemWorkflow starts the long-running per-system server or agent
// workflow. Must be called AFTER StartTemporalWorkers so that, when worker
// versioning is enabled, the deployment version is registered with the
// Temporal server before we submit a workflow pinned to it. The temporal
// service's GetClient gates on version registration to make this safe.
func (c *Config) StartSystemWorkflow() error {
	if c.servicesClient == nil || c.servicesClient.GetTemporal() == nil {
		return nil
	}

	temporalService := c.servicesClient.GetTemporal()
	temporalClient := temporalService.GetClient()

	if temporalClient == nil {
		return fmt.Errorf("temporal client is not initialized")
	}

	systemID := common.GetClientIdentifier()

	ctx := context.Background()

	startOptions := client.StartWorkflowOptions{
		ID:                       systemID.String(),
		TaskQueue:                temporalService.GetTaskQueue(),
		WorkflowIDReusePolicy:    enums.WORKFLOW_ID_REUSE_POLICY_ALLOW_DUPLICATE,
		WorkflowIDConflictPolicy: enums.WORKFLOW_ID_CONFLICT_POLICY_USE_EXISTING,
	}

	// Note: the system workflows are registered with AutoUpgrade behaviour
	// (see registerTemporalWorkflows). We deliberately do NOT apply a
	// PinnedVersioningOverride here — that would strand the long-running
	// execution on the BuildID that started it and prevent future workers
	// (with newer BuildIDs) from serving its workflow tasks, queries, and
	// updates. The deployment's "current" version is promoted on worker
	// startup (see internal/config/services/temporal), which is what
	// AutoUpgrade workflows route to.

	if c.IsServer() {

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
		// reflects the latest identities. Dispatched asynchronously so
		// startup is not blocked on the round-trip update completion.
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

	/*
		Signal Workflow Activity
	*/
	temporalWorker.RegisterActivityWithOptions(
		thandActivities.SignalWorkflow,
		activity.RegisterOptions{
			Name: sdkConstants.TemporalSignalWorkflowActivityName,
		})

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
