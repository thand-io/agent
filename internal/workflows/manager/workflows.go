package manager

import (
	"context"
	"errors"
	"fmt"
	"time"

	swctx "github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	models "github.com/thand-io/agent/internal/models"
	thandModel "github.com/thand-io/agent/internal/workflows/tasks/model"
	thandTask "github.com/thand-io/agent/internal/workflows/tasks/providers/thand"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

func (m *WorkflowManager) registerWorkflows() error {
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
		m.createPrimaryWorkflowHandler(),
		workflow.RegisterOptions{
			Name:               models.TemporalExecuteElevationWorkflowName,
			VersioningBehavior: workflow.VersioningBehaviorPinned,
		},
	)

	return nil
}

// createPrimaryWorkflowHandler creates the main workflow handler function
func (m *WorkflowManager) createPrimaryWorkflowHandler() func(workflow.Context, *models.WorkflowTask) (*models.WorkflowTask, error) {
	return func(rootCtx workflow.Context, workflowTask *models.WorkflowTask) (outputTask *models.WorkflowTask, outputError error) {

		log := workflow.GetLogger(rootCtx)
		log.Info("Primary workflow execution started")

		// Get workflow info including the BuildID set by the worker
		workflowInfo := workflow.GetInfo(rootCtx)
		log.Info("Primary workflow started.",
			"WorkflowID", workflowInfo.WorkflowExecution.ID,
			"RunID", workflowInfo.WorkflowExecution.RunID,
			"BuildID", workflowInfo.GetCurrentBuildID(),
		)

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
		if err := m.setupIsApprovedQueryHandler(cancelCtx, workflowTask); err != nil {
			log.Error("Failed to set query handler", "Error", err)
			return nil, err
		}

		// Setup get workflow task query handler
		if err := m.setupGetWorkflowTaskQueryHandler(cancelCtx, workflowTask); err != nil {
			log.Error("Failed to set get workflow task query handler", "Error", err)
			return nil, err
		}
		// Setup signal channels and handlers
		resumeSignal, terminateSignal := m.setupSignalChannels(cancelCtx)
		m.setupTerminationHandler(rootCtx, terminateSignal, cancelHandler, &terminationRequest)

		// Setup workflow selector
		workflowSelector := m.setupWorkflowSelector(
			cancelCtx, resumeSignal, workflowTask)
		workflowSelector.Select(cancelCtx)

		log.Info("Starting main workflow execution loop")

		// Execute main workflow loop
		return m.executeWorkflowLoop(cancelCtx, workflowSelector, workflowTask)
	}
}

// runCleanup executes the cleanup activity and returns any cleanup-specific errors
func (m *WorkflowManager) runCleanup(
	rootCtx workflow.Context,
	workflowTask *models.WorkflowTask,
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
	workflowTask = workflowTask.WithTemporalContext(newCtx)

	// If a termination request with entrypoint is provided then we need to use it
	if terminationRequest != nil && len(terminationRequest.EntryPoint) > 0 {

		// Resume the workflow task with the specified entrypoint
		workflowTask.SetEntrypoint(terminationRequest.EntryPoint)

		result, err := m.ResumeWorkflowTask(
			workflowTask.WithTemporalContext(newCtx),
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
		revokeTask, foundTask := m.config.GetTaskRegistry().GetTaskHandler(revocationTask)

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

// setupQueryHandler sets up the query handler for the workflow
func (m *WorkflowManager) setupIsApprovedQueryHandler(
	ctx workflow.Context, workflowTask *models.WorkflowTask) error {
	return workflow.SetQueryHandler(ctx, models.TemporalIsApprovedQueryName, func() (*bool, error) {
		log := workflow.GetLogger(ctx)
		log.Info("IsApproved query received",
			"WorkflowID", workflowTask.WorkflowID,
		)
		return workflowTask.IsApproved(), nil
	})
}

func (m *WorkflowManager) setupGetWorkflowTaskQueryHandler(
	ctx workflow.Context, workflowTask *models.WorkflowTask) error {
	return workflow.SetQueryHandler(ctx, models.TemporalGetWorkflowTaskQueryName, func() (*models.WorkflowTask, error) {
		log := workflow.GetLogger(ctx)
		log.Info("GetWorkflowTask query received",
			"WorkflowID", workflowTask.WorkflowID,
		)
		return workflowTask, nil
	})
}

// setupSignalChannels creates and returns the signal channels
func (m *WorkflowManager) setupSignalChannels(ctx workflow.Context) (workflow.ReceiveChannel, workflow.ReceiveChannel) {
	resumeSignal := workflow.GetSignalChannel(ctx, models.TemporalResumeSignalName)
	terminateSignal := workflow.GetSignalChannel(ctx, models.TemporalTerminateSignalName)
	return resumeSignal, terminateSignal
}

// setupTerminationHandler sets up the background termination handler
func (m *WorkflowManager) setupTerminationHandler(
	rootCtx workflow.Context,
	terminateSignal workflow.ReceiveChannel,
	cancelHandler workflow.CancelFunc,
	terminationRequest **models.TemporalTerminationRequest) {

	log := workflow.GetLogger(rootCtx)

	workflow.Go(rootCtx, func(ctx workflow.Context) {
		log.Info("Listening for terminate signal in background goroutine")

		terminateSelector := workflow.NewSelector(ctx)
		terminateSelector.AddReceive(terminateSignal, func(c workflow.ReceiveChannel, more bool) {
			var req models.TemporalTerminationRequest
			c.Receive(ctx, &req)
			*terminationRequest = &req
			log.Info("Terminate Signal Received")
		})

		terminateSelector.Select(ctx)

		if *terminationRequest != nil {
			m.handleTerminationRequest(ctx, *terminationRequest)
		}

		cancelHandler()
		log.Info("Workflow cancellation initiated due to terminate signal")
	})
}

// handleTerminationRequest processes the termination request
func (m *WorkflowManager) handleTerminationRequest(
	ctx workflow.Context,
	terminationRequest *models.TemporalTerminationRequest,
) {

	log := workflow.GetLogger(ctx)

	if terminationRequest == nil {
		log.Info("No termination request provided, skipping termination handling")
		return
	}

	log.Info("Processing termination request",
		"Reason", terminationRequest.Reason,
		"EntryPoint", terminationRequest.EntryPoint,
		"ScheduledAt", terminationRequest.ScheduledAt,
	)

	var timerDuration time.Duration
	if terminationRequest.ScheduledAt != nil && !terminationRequest.ScheduledAt.IsZero() {
		// Use workflow.Now() instead of time.Now() for deterministic time
		now := workflow.Now(ctx)
		delay := terminationRequest.ScheduledAt.Sub(now)
		timerDuration = max(delay, 0)
	}

	// New behavior: always create timer, but with minimum duration
	if timerDuration <= 0 {
		timerDuration = time.Nanosecond // Minimum timer duration
	}
	timer := workflow.NewTimer(ctx, timerDuration)
	timer.Get(ctx, nil)
	log.Info("Termination timer completed",
		"Duration", timerDuration,
	)

}

// setupWorkflowSelector creates and configures the workflow selector
func (m *WorkflowManager) setupWorkflowSelector(
	ctx workflow.Context,
	resumeSignal workflow.ReceiveChannel,
	workflowTask *models.WorkflowTask,
) workflow.Selector {
	workflowSelector := workflow.NewSelector(ctx)
	log := workflow.GetLogger(ctx)

	workflowSelector.AddReceive(resumeSignal, func(c workflow.ReceiveChannel, more bool) {
		c.Receive(ctx, &workflowTask)
		log.Info("Resume Signal Received")
	})

	workflowSelector.AddFuture(workflow.NewTimer(ctx, 0), func(f workflow.Future) {
		log.Debug("Timer triggered for context cancellation check")
		if ctx.Err() != nil {
			log.Debug("Context cancellation detected via timer")
		}
	})

	return workflowSelector
}

// shouldContinueAsNew checks if the workflow should perform Continue-As-New
// This allows upgrading to new worker versions and prevents event history size issues
func (m *WorkflowManager) shouldContinueAsNew(ctx workflow.Context) bool {

	log := workflow.GetLogger(ctx)

	// Check Temporal's built-in suggestion for Continue-As-New
	// This is triggered when event history approaches size limits
	if workflow.GetInfo(ctx).GetContinueAsNewSuggested() {
		log.Info("Continue-As-New suggested by Temporal (event history size)")
		return true
	}

	// TODO(hugh): Add custom Continue-As-New triggers here

	return false
}

// executeWorkflowLoop executes the main workflow execution loop
func (m *WorkflowManager) executeWorkflowLoop(
	cancelCtx workflow.Context,
	workflowSelector workflow.Selector,
	workflowTask *models.WorkflowTask,
) (*models.WorkflowTask, error) {

	log := workflow.GetLogger(cancelCtx)

	for {

		log.Info("Waiting for signal...")

		// Check if we should Continue-As-New before waiting for signal
		// This allows upgrading to new worker versions at safe checkpoints
		if m.shouldContinueAsNew(cancelCtx) {
			currentBuildID := workflow.GetInfo(cancelCtx).GetCurrentBuildID()
			log.Info("Continue-As-New suggested, upgrading workflow to latest version",
				"WorkflowID", workflowTask.WorkflowID,
				"CurrentBuildID", currentBuildID,
			)

			return workflowTask, workflow.NewContinueAsNewError(
				cancelCtx,
				models.TemporalExecuteElevationWorkflowName,
				workflowTask,
			)
		}

		if err := m.waitForSignal(cancelCtx, workflowSelector); err != nil {
			return nil, err
		}

		if cancelCtx.Err() != nil {
			if errors.Is(cancelCtx.Err(), context.Canceled) {
				log.Info("Workflow context cancelled, exiting main loop")
				break
			}
			log.Error("Error while waiting for signal", "Error", cancelCtx.Err())
			return nil, cancelCtx.Err()
		}

		workflowSelector.Select(cancelCtx)

		if workflowTask == nil {
			continue
		}

		log.Info("Resuming ...",
			"WorkflowID", workflowTask.WorkflowID,
			"Status", workflowTask.GetStatus(),
		)

		// Execute workflow step
		result, err := m.executeWorkflowStep(cancelCtx, workflowTask)

		// Check if the context was cancelled during execution
		if cancelCtx.Err() != nil {
			if errors.Is(cancelCtx.Err(), context.Canceled) {
				if result != nil {
					result.SetStatus(swctx.CancelledStatus)
				}
				log.Info("Workflow context cancelled during execution, exiting main loop")
				return result, nil
			}
			log.Error("Error while executing workflow step", "Error", cancelCtx.Err())
			return result, cancelCtx.Err()
		}

		// If execution completed or failed, return the result
		if err != nil || (result != nil && result.GetStatus() != swctx.RunningStatus) {
			return result, err
		}

		// Continue loop for running workflows
		workflowTask = result
	}

	// Loop exited due to cancellation
	return workflowTask, nil
}

// waitForSignal waits for any signals to be available
func (m *WorkflowManager) waitForSignal(cancelCtx workflow.Context, workflowSelector workflow.Selector) error {

	log := workflow.GetLogger(cancelCtx)

	log.Info("Waiting for signal or cancellation...")

	return workflow.Await(cancelCtx, func() bool {
		if cancelCtx.Err() != nil {
			log.Info("Context error", "Error", cancelCtx.Err())
			if errors.Is(cancelCtx.Err(), context.Canceled) {
				log.Info("Context was cancelled")
			}
			return true
		}

		pending := workflowSelector.HasPending()
		log.Info("Signal pending", "Pending", pending)
		return pending
	})
}

// executeWorkflowStep executes a single workflow step and handles the result
func (m *WorkflowManager) executeWorkflowStep(
	ctx workflow.Context, workflowTask *models.WorkflowTask) (*models.WorkflowTask, error) {
	log := workflow.GetLogger(ctx)

	log.Info("Starting workflow execution")

	f := m.StartWorkflow(ctx, workflowTask)
	err := f.Get(ctx, &workflowTask)

	if err != nil {
		log.Error("Workflow execution failed", "Error", err)
		workflowTask.SetStatus(swctx.FaultedStatus)
		return workflowTask, err
	}

	log.Info("Workflow execution step completed", "Status", workflowTask.GetStatus())

	return m.handleWorkflowStatus(workflowTask)
}

// handleWorkflowStatus handles different workflow status cases
func (m *WorkflowManager) handleWorkflowStatus(workflowTask *models.WorkflowTask) (*models.WorkflowTask, error) {

	log := workflowTask.GetLogger()

	switch workflowTask.GetStatus() {
	case swctx.RunningStatus:
		log.WithFields(models.Fields{
			"workflow_id": workflowTask.WorkflowID,
			"task_name":   workflowTask.GetTaskName(),
		}).Info("Workflow is still running")
		return workflowTask, nil // Continue loop

	case swctx.CompletedStatus:
		log.Info("Workflow completed successfully.")
		return workflowTask, nil

	case swctx.FaultedStatus:
		log.Error("Workflow failed.")
		return workflowTask, fmt.Errorf("workflow failed")

	case swctx.WaitingStatus:
		log.Info("Workflow is waiting, pausing execution.")
		return workflowTask, nil

	case swctx.PendingStatus:
		log.Info("Workflow is pending, pausing execution.")
		return workflowTask, nil

	default:
		log.WithField("status", workflowTask.GetStatus()).Error("Workflow ended in unknown state")
		return workflowTask, fmt.Errorf("workflow ended in unknown state: %s", workflowTask.GetStatus())
	}
}

func (m *WorkflowManager) StartWorkflow(ctx workflow.Context, workflowTask *models.WorkflowTask) workflow.Future {

	log := workflowTask.GetLogger()

	log.WithField("WorkflowID", workflowTask.WorkflowID).Info("Starting workflow execution loop")

	future, settable := workflow.NewFuture(ctx)

	workflow.Go(ctx, func(ctx workflow.Context) {

		// Continue to resume the workflow until it is completed, faulted, or waiting
		// This loop allows us to handle the workflow execution in a single Temporal workflow run
		// and manage its state transitions effectively

		// Resume the workflow task
		result, err := m.ResumeWorkflowTask(
			workflowTask.WithTemporalContext(ctx),
		)

		settable.Set(result, err)

		log.Info("Workflow resumed")

	})

	return future

}
