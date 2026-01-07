package manager

import (
	"context"
	"errors"
	"fmt"
	"time"

	models "github.com/thand-io/agent/internal/models"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
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
		m.CreateServerlessWorkflowHandler(),
		workflow.RegisterOptions{
			Name:               sdkWorkflowsModel.TemporalExecuteServerlessWorkflowName,
			VersioningBehavior: workflow.VersioningBehaviorPinned,
		},
	)

	return nil
}

// createPrimaryWorkflowHandler creates the main workflow handler function
func (m *WorkflowManager) CreateServerlessWorkflowHandler() func(workflow.Context, *sdkWorkflowsModel.WorkflowTask) (*sdkWorkflowsModel.WorkflowTask, error) {
	return func(rootCtx workflow.Context, workflowTask *sdkWorkflowsModel.WorkflowTask) (outputTask *sdkWorkflowsModel.WorkflowTask, outputError error) {

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

		serverlessWorkflow := NewServerlessWorkflow(m.config)

		// Setup cleanup handler
		defer func() {

			// Handle workflow panic
			if r := recover(); r != nil {
				outputError = fmt.Errorf("workflow failed: %s", r)
				return
			}

			outputTask = workflowTask

			if cancelCtx.Err() != nil && (errors.Is(cancelCtx.Err(), context.Canceled) || temporal.IsCanceledError(cancelCtx.Err())) {
				// Suppress cancellation errors - workflow completed normally
				outputError = nil
			}
			log.Info("Workflow cleanup completed.")

		}()

		// Setup get workflow task query handler
		if err := SetupGetWorkflowTaskQueryHandler(cancelCtx, workflowTask); err != nil {
			log.Error("Failed to set get workflow task query handler", "Error", err)
			return nil, err
		}

		// Setup signal channels and handlers
		resumeSignal, terminateSignal := SetupSignalChannels(cancelCtx)
		SetupTerminationHandler(rootCtx, terminateSignal, cancelHandler, &terminationRequest)

		// Setup workflow selector
		workflowSelector := SetupWorkflowSelector(
			cancelCtx, resumeSignal, workflowTask)
		workflowSelector.Select(cancelCtx)

		log.Info("Starting main workflow execution loop")

		// Execute main workflow loop
		return serverlessWorkflow.Run(cancelCtx, workflowSelector, workflowTask)

	}
}

func SetupGetWorkflowTaskQueryHandler(
	ctx workflow.Context,
	workflowTask *sdkWorkflowsModel.WorkflowTask,
) error {
	return workflow.SetQueryHandler(ctx, models.TemporalGetWorkflowTaskQueryName, func() (*sdkWorkflowsModel.WorkflowTask, error) {
		log := workflow.GetLogger(ctx)
		log.Info("GetWorkflowTask query received",
			"WorkflowID", workflowTask.GetWorkflowID(),
		)
		return workflowTask, nil
	})
}

// SetupSignalChannels creates and returns the signal channels
func SetupSignalChannels(ctx workflow.Context) (workflow.ReceiveChannel, workflow.ReceiveChannel) {
	resumeSignal := workflow.GetSignalChannel(ctx, sdkWorkflowsModel.TemporalResumeSignalName)
	terminateSignal := workflow.GetSignalChannel(ctx, sdkWorkflowsModel.TemporalTerminateSignalName)
	return resumeSignal, terminateSignal
}

// setupTerminationHandler sets up the background termination handler
func SetupTerminationHandler(
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
			HandleTerminationRequest(ctx, *terminationRequest)
		}

		cancelHandler()
		log.Info("Workflow cancellation initiated due to terminate signal")
	})
}

// setupWorkflowSelector creates and configures the workflow selector
func SetupWorkflowSelector(
	ctx workflow.Context,
	resumeSignal workflow.ReceiveChannel,
	workflowTask *sdkWorkflowsModel.WorkflowTask,
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

// HandleTerminationRequest processes the termination request
func HandleTerminationRequest(
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
