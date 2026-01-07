package manager

import (
	"context"
	"errors"
	"fmt"

	swctx "github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"github.com/sirupsen/logrus"
	models "github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/sdk/workflows/config"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	runner "github.com/thand-io/agent/sdk/workflows/runner"
	"go.temporal.io/sdk/workflow"
)

type serverlessWorkflow struct {
	config config.Config
}

func NewServerlessWorkflow(config config.Config) *serverlessWorkflow {
	return &serverlessWorkflow{
		config: config,
	}
}

func (m *serverlessWorkflow) Run(cancelCtx workflow.Context, workflowSelector workflow.Selector, workflowTask *sdkWorkflowsModel.WorkflowTask) (outputTask *sdkWorkflowsModel.WorkflowTask, outputError error) {

	// Execute main workflow loop
	return m.executeWorkflowLoop(cancelCtx, workflowSelector, workflowTask)

}

// executeWorkflowLoop executes the main workflow execution loop
func (m *serverlessWorkflow) executeWorkflowLoop(
	cancelCtx workflow.Context,
	workflowSelector workflow.Selector,
	workflowTask *sdkWorkflowsModel.WorkflowTask,
) (*sdkWorkflowsModel.WorkflowTask, error) {

	log := workflow.GetLogger(cancelCtx)

	for {

		log.Info("Waiting for signal...")

		// Check if we should Continue-As-New before waiting for signal
		// This allows upgrading to new worker versions at safe checkpoints
		if m.shouldContinueAsNew(cancelCtx) {
			currentBuildID := workflow.GetInfo(cancelCtx).GetCurrentBuildID()
			log.Info("Continue-As-New suggested, upgrading workflow to latest version",
				"WorkflowID", workflowTask.GetWorkflowID(),
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
			"WorkflowID", workflowTask.GetWorkflowID(),
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
func (m *serverlessWorkflow) waitForSignal(cancelCtx workflow.Context, workflowSelector workflow.Selector) error {

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
func (m *serverlessWorkflow) executeWorkflowStep(
	ctx workflow.Context, workflowTask *sdkWorkflowsModel.WorkflowTask) (*sdkWorkflowsModel.WorkflowTask, error) {
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
func (m *serverlessWorkflow) handleWorkflowStatus(
	workflowTask *sdkWorkflowsModel.WorkflowTask,
) (*sdkWorkflowsModel.WorkflowTask, error) {

	log := workflowTask.GetLogger()

	switch workflowTask.GetStatus() {
	case swctx.RunningStatus:
		log.WithFields(logrus.Fields{
			"workflow_id": workflowTask.GetWorkflowID(),
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

func (m *serverlessWorkflow) StartWorkflow(
	ctx workflow.Context,
	workflowTask *sdkWorkflowsModel.WorkflowTask,
) workflow.Future {

	log := workflowTask.GetLogger()

	log.WithField("WorkflowID", workflowTask.GetWorkflowID()).Info("Starting workflow execution loop")
	future, settable := workflow.NewFuture(ctx)

	workflow.Go(ctx, func(ctx workflow.Context) {

		// Continue to resume the workflow until it is completed, faulted, or waiting
		// This loop allows us to handle the workflow execution in a single Temporal workflow run
		// and manage its state transitions effectively

		// Resume the workflow task
		result, err := ResumeWorkflowTask(
			m.config,
			workflowTask.WithTemporalContext(ctx),
		)

		settable.Set(result, err)

		log.Info("Workflow resumed")

	})

	return future

}

// shouldContinueAsNew checks if the workflow should perform Continue-As-New
// This allows upgrading to new worker versions and prevents event history size issues
func (m *serverlessWorkflow) shouldContinueAsNew(ctx workflow.Context) bool {

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

// ResumeWorkflowTask resumes a workflow task using the internal runner
// This maybe called as part of a temporal workflow or directly
func ResumeWorkflowTask(
	config config.Config,
	workflowTask sdkWorkflowsModel.WorkflowTaskSupport,
) (sdkWorkflowsModel.WorkflowTaskSupport, error) {

	// Hydrate the workflow task
	err := config.HydrateWorkflowTask(workflowTask)

	if err != nil {
		return nil, fmt.Errorf("failed to hydrate resumed workflow task: %w", err)
	}

	// Create a new task state if it does not exist
	// This is important as we might be in the middle of a workflow and
	// the state might not have been initialised yet
	if !workflowTask.HasState() {
		workflowTask.ClearTaskContext()
	}

	// Set status to pending if not already set
	if !workflowTask.HasStatus() {
		workflowTask.SetStatus(swctx.PendingStatus)
	}

	logrus.WithFields(logrus.Fields{
		"workflow_id": workflowTask.GetWorkflowID(),
	}).Info("Resuming workflow")

	// Create runner
	runner := runner.NewResumableRunner(config, workflowTask)

	// Resume from saved state
	_, err = runner.Run(workflowTask.GetInput())

	if err != nil {
		return nil, fmt.Errorf("failed to resume workflow: %w", err)
	}

	// Merge the output with the input based on any handlers

	return workflowTask, err
}
