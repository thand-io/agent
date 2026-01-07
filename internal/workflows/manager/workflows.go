package manager

import (
	"context"
	"errors"
	"fmt"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	sdkWorkflows "github.com/thand-io/agent/sdk/workflows/manager"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"

	models "github.com/thand-io/agent/internal/models"
	thandModel "github.com/thand-io/agent/internal/workflows/tasks/model"
	thandTask "github.com/thand-io/agent/internal/workflows/tasks/providers/thand"
)

func (m *ThandWorkflowManager) registerThandWorkflows() error {

	if !m.HasTemporal() {
		return fmt.Errorf("temporal service not configured")
	}

	temporalService := m.GetTemporal()
	if temporalService == nil {
		return fmt.Errorf("temporal service not available")
	}

	if !temporalService.HasWorker() {
		return fmt.Errorf("temporal worker not configured")
	}

	worker := temporalService.GetWorker()

	// Register the primary workflow with Pinned versioning behavior
	//
	// This ensures that existing workflows continue to run with the code they started with
	// preventing non-determinism errors when code changes occur
	worker.RegisterWorkflowWithOptions(
		m.createElevationWorkflowHandler(),
		workflow.RegisterOptions{
			Name:               models.TemporalExecuteElevationWorkflowName,
			VersioningBehavior: workflow.VersioningBehaviorPinned,
		},
	)

	return nil
}

// createPrimaryWorkflowHandler creates the main workflow handler function
func (m *ThandWorkflowManager) createElevationWorkflowHandler() func(workflow.Context, *models.ThandWorkflowTask) (*models.ThandWorkflowTask, error) {
	return func(
		rootCtx workflow.Context,
		workflowTask *models.ThandWorkflowTask,
	) (outputTask *models.ThandWorkflowTask, outputError error) {

		log := workflow.GetLogger(rootCtx)
		log.Info("Primary workflow execution started")

		// Get workflow info including the BuildID set by the worker
		workflowInfo := workflow.GetInfo(rootCtx)
		log.Info("Primary workflow started.",
			"WorkflowID", workflowInfo.WorkflowExecution.ID,
			"RunID", workflowInfo.WorkflowExecution.RunID,
			"BuildID", workflowInfo.GetCurrentBuildID(),
		)

		serverlessWorkflow := sdkWorkflows.NewServerlessWorkflow(m.workflowManager.GetConfig())

		cancelCtx, cancelHandler := workflow.WithCancel(rootCtx)

		// Variable to store termination request, accessible to both goroutine and defer
		var terminationRequest *models.TemporalTerminationRequest

		// Setup cleanup handler
		defer func() {

			// Handle workflow panic
			if r := recover(); r != nil {
				outputError = fmt.Errorf("workflow failed: %s", r)
				return
			}

			cleanupErr := m.runCleanup(rootCtx, workflowTask, terminationRequest)

			outputTask = workflowTask

			if cleanupErr != nil {
				log.Error("Cleanup activity failed", "Error", cleanupErr)
				outputError = cleanupErr
			} else if cancelCtx.Err() != nil && (errors.Is(cancelCtx.Err(), context.Canceled) || temporal.IsCanceledError(cancelCtx.Err())) {
				// Suppress cancellation errors - workflow completed normally
				outputError = nil
			}
			log.Info("Workflow cleanup completed.")

		}()

		// Setup query handler
		if err := SetupIsApprovedQueryHandler(cancelCtx, workflowTask); err != nil {
			log.Error("Failed to set query handler", "Error", err)
			return nil, err
		}

		// Setup get workflow task query handler
		if err := sdkWorkflows.SetupGetWorkflowTaskQueryHandler(cancelCtx, workflowTask); err != nil {
			log.Error("Failed to set get workflow task query handler", "Error", err)
			return nil, err
		}

		// Setup signal channels and handlers
		resumeSignal, terminateSignal := sdkWorkflows.SetupSignalChannels(cancelCtx)
		sdkWorkflows.SetupTerminationHandler(rootCtx, terminateSignal, cancelHandler, &terminationRequest)

		// Setup workflow selector
		workflowSelector := sdkWorkflows.SetupWorkflowSelector(
			cancelCtx, resumeSignal, workflowTask)
		workflowSelector.Select(cancelCtx)

		log.Info("Starting main workflow execution loop")

		// Execute main workflow loop
		result, err := serverlessWorkflow.Run(cancelCtx, workflowSelector, workflowTask)

		if err != nil {
			return nil, err
		}

		return models.NewThandWorkflowTask(result), nil

	}
}

// setupQueryHandler sets up the query handler for the workflow
func SetupIsApprovedQueryHandler(
	ctx workflow.Context, workflowTask *models.ThandWorkflowTask) error {
	return workflow.SetQueryHandler(ctx, models.TemporalIsApprovedQueryName, func() (*bool, error) {
		log := workflow.GetLogger(ctx)
		log.Info("IsApproved query received",
			"WorkflowID", workflowTask.WorkflowID,
		)
		return workflowTask.IsApproved(), nil
	})
}

// runCleanup executes the cleanup activity and returns any cleanup-specific errors
func (m *ThandWorkflowManager) runCleanup(
	rootCtx workflow.Context,
	workflowTask *models.ThandWorkflowTask,
	terminationRequest *models.TemporalTerminationRequest,
) error {

	log := workflow.GetLogger(rootCtx)

	log.Info("Starting cleanup activity...")

	// Log termination request if present
	if terminationRequest != nil {
		log.Info("Cleanup running with termination request",
			"Reason", terminationRequest.Reason,
			"EntryPoint", terminationRequest.EntryPoint,
			"ScheduledAt", terminationRequest.ScheduledAt,
		)
	}

	if approved := workflowTask.IsApproved(); approved == nil || !*approved {
		log.Info("Workflow not approved, skipping cleanup activity.")
		return nil
	}

	// Check if a user or role is associated with the workflow
	elevationRequest, err := workflowTask.GetContextAsElevationRequest()
	if err != nil || !elevationRequest.IsValid() {
		log.Info("No valid elevation context found, skipping cleanup activity.")
		return nil
	}

	// Use a disconnected context for cleanup to ensure it runs even if workflow is cancelled
	newCtx, _ := workflow.NewDisconnectedContext(rootCtx)
	workflowTask = workflowTask.WithTemporalContext(newCtx).(*models.ThandWorkflowTask)

	// If a termination request with entrypoint is provided then we need to use it
	if terminationRequest != nil && len(terminationRequest.EntryPoint) > 0 {

		// Resume the workflow task with the specified entrypoint
		workflowTask.SetEntrypoint(terminationRequest.EntryPoint)

		result, err := m.ResumeWorkflowTask(
			workflowTask,
		)

		if err != nil {
			log.Error("Failed to resume workflow for cleanup with termination entrypoint",
				"Error", err,
			)
			return err
		}

		log.Info("Workflow resumed for cleanup with termination entrypoint",
			"Status", result.GetStatus(),
		)

	} else {

		// Get the taskItem from the workflow spec or create a synthetic one
		revocationTask := &model.TaskItem{
			Key: "$cleanup",
			Task: &thandModel.ThandTask{
				Thand: thandTask.ThandRevokeTask,
				With:  nil,
			},
		}

		// Run the revocation task
		revokeTask, foundTask := m.GetTaskRegistry().GetTaskHandler(revocationTask)

		if !foundTask {
			log.Error("Failed to get revoke task handler for cleanup")
			return errors.New("failed to get revoke task handler for cleanup")
		}

		_, err = revokeTask.Execute(
			workflowTask,
			revocationTask,
			nil,
		)

		if err != nil {
			log.Error("Cleanup activity failed", "Error", err)
			return err
		}

	}

	log.Info("Cleanup completed successfully")
	return nil
}
