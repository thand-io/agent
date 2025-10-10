package manager

import (
	"context"
	"errors"
	"fmt"
	"time"

	swctx "github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"github.com/sirupsen/logrus"
	models "github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

func (m *WorkflowManager) registerWorkflows() error {
	if !m.config.GetServices().HasTemporal() {
		return fmt.Errorf("temporal service not configured")
	}

	temporalService := m.config.GetServices().GetTemporal()
	if temporalService == nil {
		return fmt.Errorf("temporal service not available")
	}

	if !temporalService.HasWorker() {
		return fmt.Errorf("temporal worker not configured")
	}

	worker := temporalService.GetWorker()

	// Register the primary workflow
	worker.RegisterWorkflowWithOptions(
		m.createPrimaryWorkflowHandler(temporalService),
		workflow.RegisterOptions{
			Name: models.TemporalExecuteElevationWorkflowName,
		},
	)

	return nil
}

// createPrimaryWorkflowHandler creates the main workflow handler function
func (m *WorkflowManager) createPrimaryWorkflowHandler(
	temporalService interface{ GetTaskQueue() string },
) func(workflow.Context, *models.WorkflowTask) (*models.WorkflowTask, error) {
	return func(rootCtx workflow.Context, workflowTask *models.WorkflowTask) (outputTask *models.WorkflowTask, outputError error) {

		logrus.WithFields(logrus.Fields{
			"WorkflowID": workflowTask.WorkflowID,
			"TaskName":   workflowTask.WorkflowName,
			"StartTime":  workflow.Now(rootCtx),
		}).Info("Primary workflow started.")

		cancelCtx, cancelHandler := workflow.WithCancel(rootCtx)

		// Setup cleanup handler
		defer func() {
			cleanupErr := m.runCleanup(rootCtx, workflowTask, temporalService)

			outputTask = workflowTask

			if cleanupErr != nil {
				logrus.WithError(cleanupErr).Error("Cleanup activity failed")
				outputError = cleanupErr
			} else if cancelCtx.Err() != nil && (errors.Is(cancelCtx.Err(), context.Canceled) || temporal.IsCanceledError(cancelCtx.Err())) {
				// Suppress cancellation errors - workflow completed normally
				outputError = nil
			}
			logrus.Info("Workflow cleanup completed.")

		}()

		// Setup query handler
		if err := m.setupIsApprovedQueryHandler(cancelCtx, workflowTask); err != nil {
			logrus.Error("Failed to set query handler", "Error", err)
			return nil, err
		}

		// Setup get workflow task query handler
		if err := m.setupGetWorkflowTaskQueryHandler(cancelCtx, workflowTask); err != nil {
			logrus.Error("Failed to set get workflow task query handler", "Error", err)
			return nil, err
		}

		// Setup signal channels and handlers
		resumeSignal, terminateSignal := m.setupSignalChannels(cancelCtx)
		m.setupTerminationHandler(rootCtx, terminateSignal, cancelHandler)

		// Setup workflow selector
		workflowSelector := m.setupWorkflowSelector(
			cancelCtx, resumeSignal, workflowTask)
		workflowSelector.Select(cancelCtx)

		logrus.Info("Starting main workflow execution loop")

		// Execute main workflow loop
		return m.executeWorkflowLoop(cancelCtx, workflowSelector, workflowTask)
	}
}

// runCleanup executes the cleanup activity and returns any cleanup-specific errors
func (m *WorkflowManager) runCleanup(
	rootCtx workflow.Context,
	workflowTask *models.WorkflowTask,
	temporalService interface{ GetTaskQueue() string },
) error {
	// Check if a user or role is associated with the workflow
	elevationRequest, err := workflowTask.GetContextAsElevationRequest()
	if err != nil || !elevationRequest.IsValid() {
		logrus.Info("No valid elevation context found, skipping cleanup activity.")
		return nil
	}

	// Use a disconnected context for cleanup to ensure it runs even if workflow is cancelled
	newCtx, _ := workflow.NewDisconnectedContext(rootCtx)

	ao := workflow.ActivityOptions{
		TaskQueue:              temporalService.GetTaskQueue(),
		ScheduleToCloseTimeout: time.Minute * 5,
		RetryPolicy: &temporal.RetryPolicy{
			MaximumAttempts: 3,
		},
		WaitForCancellation: true,
	}

	cleanupCtx := workflow.WithActivityOptions(newCtx, ao)

	err = workflow.ExecuteActivity(
		cleanupCtx,
		models.TemporalCleanupActivityName,
		workflowTask,
	).Get(cleanupCtx, nil)

	if err != nil {
		logrus.Error("Cleanup activity failed", "error", err)
		return fmt.Errorf("cleanup failed: %w", err)
	}

	logrus.Info("Cleanup completed successfully")
	return nil
}

// setupQueryHandler sets up the query handler for the workflow
func (m *WorkflowManager) setupIsApprovedQueryHandler(
	ctx workflow.Context, workflowTask *models.WorkflowTask) error {
	return workflow.SetQueryHandler(ctx, models.TemporalIsApprovedQueryName, func() (bool, error) {
		logrus.WithFields(logrus.Fields{
			"WorkflowID": workflowTask.WorkflowID,
		}).Debug("IsApproved query received")
		return workflowTask.IsApproved(), nil
	})
}

func (m *WorkflowManager) setupGetWorkflowTaskQueryHandler(
	ctx workflow.Context, workflowTask *models.WorkflowTask) error {
	return workflow.SetQueryHandler(ctx, models.TemporalGetWorkflowTaskQueryName, func() (*models.WorkflowTask, error) {
		logrus.WithFields(logrus.Fields{
			"WorkflowID": workflowTask.WorkflowID,
		}).Debug("GetWorkflowTask query received")
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
	cancelHandler workflow.CancelFunc) {
	var terminationRequest *models.TemporalTerminationRequest

	terminateSelector := workflow.NewSelector(rootCtx)
	terminateSelector.AddReceive(terminateSignal, func(c workflow.ReceiveChannel, more bool) {
		c.Receive(rootCtx, &terminationRequest)
		logrus.Info("Terminate Signal Received")
	})

	workflow.Go(rootCtx, func(ctx workflow.Context) {
		logrus.Info("Listening for terminate signal in background goroutine")
		terminateSelector.Select(ctx)

		if terminationRequest != nil {
			m.handleTerminationRequest(ctx, terminationRequest)
		}

		cancelHandler()
		logrus.Info("Workflow cancellation initiated due to terminate signal")
	})
}

// handleTerminationRequest processes the termination request
func (m *WorkflowManager) handleTerminationRequest(
	ctx workflow.Context,
	terminationRequest *models.TemporalTerminationRequest,
) {
	logrus.Info("Processing termination request", "Reason", terminationRequest.Reason, "ScheduledAt", terminationRequest.ScheduledAt)

	var timerDuration time.Duration
	if !terminationRequest.ScheduledAt.IsZero() {
		delay := time.Until(terminationRequest.ScheduledAt)
		timerDuration = max(delay, 0)
	}

	if timerDuration > 0 {
		timer := workflow.NewTimer(ctx, timerDuration)
		timer.Get(ctx, nil)
		logrus.Info("Termination timer completed", "Duration", timerDuration)
	}
}

// setupWorkflowSelector creates and configures the workflow selector
func (m *WorkflowManager) setupWorkflowSelector(
	ctx workflow.Context,
	resumeSignal workflow.ReceiveChannel,
	workflowTask *models.WorkflowTask,
) workflow.Selector {
	workflowSelector := workflow.NewSelector(ctx)

	workflowSelector.AddReceive(resumeSignal, func(c workflow.ReceiveChannel, more bool) {
		c.Receive(ctx, &workflowTask)
		logrus.Info("Resume Signal Received")
	})

	workflowSelector.AddFuture(workflow.NewTimer(ctx, 0), func(f workflow.Future) {
		logrus.Info("Timer triggered for context cancellation check")
		if ctx.Err() != nil {
			logrus.Info("Context cancellation detected via timer")
		}
	})

	return workflowSelector
}

// executeWorkflowLoop executes the main workflow execution loop
func (m *WorkflowManager) executeWorkflowLoop(
	cancelCtx workflow.Context,
	workflowSelector workflow.Selector,
	workflowTask *models.WorkflowTask,
) (*models.WorkflowTask, error) {

	for {

		logrus.Info("Waiting for signal...")

		if err := m.waitForSignal(cancelCtx, workflowSelector); err != nil {
			return nil, err
		}

		if cancelCtx.Err() != nil {
			if errors.Is(cancelCtx.Err(), context.Canceled) {
				logrus.Info("Workflow context cancelled, exiting main loop")
				break
			}
			logrus.Error("Error while waiting for signal", "Error", cancelCtx.Err())
			return nil, cancelCtx.Err()
		}

		workflowSelector.Select(cancelCtx)

		if workflowTask == nil {
			continue
		}

		logrus.WithFields(logrus.Fields{
			"WorkflowID": workflowTask.WorkflowID,
			"Status":     workflowTask.GetStatus(),
		}).Info("Resuming ...")

		// Execute workflow step
		result, err := m.executeWorkflowStep(cancelCtx, workflowTask)

		// Check if the context was cancelled during execution
		if cancelCtx.Err() != nil {
			if errors.Is(cancelCtx.Err(), context.Canceled) {
				if result != nil {
					result.SetStatus(swctx.CancelledStatus)
				}
				logrus.Info("Workflow context cancelled during execution, exiting main loop")
				return result, nil
			}
			logrus.Error("Error while executing workflow step", "Error", cancelCtx.Err())
			return result, cancelCtx.Err()
		}

		return result, err
	}

	return workflowTask, fmt.Errorf("workflow terminated")
}

// waitForSignal waits for any signals to be available
func (m *WorkflowManager) waitForSignal(cancelCtx workflow.Context, workflowSelector workflow.Selector) error {
	return workflow.Await(cancelCtx, func() bool {
		if cancelCtx.Err() != nil {
			logrus.Info("Context error", "Error", cancelCtx.Err())
			if errors.Is(cancelCtx.Err(), context.Canceled) {
				logrus.Info("Context was cancelled")
			}
			return true
		}

		pending := workflowSelector.HasPending()
		logrus.Info("Signal pending", "Pending", pending)
		return pending
	})
}

// executeWorkflowStep executes a single workflow step and handles the result
func (m *WorkflowManager) executeWorkflowStep(
	ctx workflow.Context, workflowTask *models.WorkflowTask) (*models.WorkflowTask, error) {
	logrus.Info("Starting workflow execution")

	f := m.StartWorkflow(ctx, workflowTask)
	err := f.Get(ctx, &workflowTask)

	if err != nil {
		logrus.Error("Workflow execution failed", "Error", err)
		workflowTask.SetStatus(swctx.FaultedStatus)
		return workflowTask, err
	}

	logrus.Info("Workflow execution step completed", "Status", workflowTask.GetStatus())

	return m.handleWorkflowStatus(workflowTask)
}

// handleWorkflowStatus handles different workflow status cases
func (m *WorkflowManager) handleWorkflowStatus(workflowTask *models.WorkflowTask) (*models.WorkflowTask, error) {
	switch workflowTask.GetStatus() {
	case swctx.RunningStatus:
		logrus.Info("Workflow is still running", "workflow_id", workflowTask.WorkflowID, "task_name", workflowTask.GetTaskName())
		return nil, nil // Continue loop

	case swctx.CompletedStatus:
		logrus.Info("Workflow completed successfully.")
		return workflowTask, nil

	case swctx.FaultedStatus:
		logrus.Error("Workflow failed.")
		return nil, fmt.Errorf("workflow failed")

	case swctx.WaitingStatus:
		logrus.Info("Workflow is waiting, pausing execution.")
		return workflowTask, fmt.Errorf("workflow terminated") // Break loop

	case swctx.PendingStatus:
		logrus.Info("Workflow is pending, pausing execution.")
		return nil, nil // Continue loop

	default:
		logrus.Error("Workflow ended in unknown state.")
		return nil, fmt.Errorf("workflow ended in unknown state: %s", workflowTask.GetStatus())
	}
}

func (m *WorkflowManager) StartWorkflow(ctx workflow.Context, workflowTask *models.WorkflowTask) workflow.Future {

	logrus.Info("Starting workflow execution loop", "WorkflowID", workflowTask.WorkflowID)

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

		logrus.Info("Workflow resumed", "Status", workflowTask.GetStatus())

	})

	return future

}
