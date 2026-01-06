package runner

import (
	"context"
	"errors"
	"fmt"
	"time"

	swctx "github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	utils "github.com/serverlessworkflow/sdk-go/v3/impl/utils"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	models "github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/functions"
	"github.com/thand-io/agent/internal/workflows/tasks"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

// ResumableWorkflowRunner implements a workflow runner that can pause and resume
type ResumableWorkflowRunner struct {
	config       models.ConfigImpl
	functions    *functions.FunctionRegistry
	tasks        *tasks.TaskRegistry
	workflowTask *models.WorkflowTask
}

func (r *ResumableWorkflowRunner) GetContext() context.Context {
	ctx := r.workflowTask.GetContext()
	return models.WithWorkflowContext(ctx, r.workflowTask)
}

func (r *ResumableWorkflowRunner) CloneWithContext(ctx context.Context) *ResumableWorkflowRunner {
	// Try get a workflow task from the provided context, otherwise clone the current one
	var wf *models.WorkflowTask
	if ctx != nil {
		if wfc, err := models.GetWorkflowContext(ctx); err == nil {
			if t, ok := wfc.(*models.WorkflowTask); ok {
				wf = t
			}
		}
	}
	if wf == nil {
		wf = r.workflowTask.Clone().(*models.WorkflowTask)
	}
	// attach the provided context into the workflow task so GetContext() is coherent
	if ctx != nil {
		wf.SetInternalContext(ctx)
	}
	return &ResumableWorkflowRunner{
		config:       r.config,
		functions:    r.functions,
		tasks:        r.tasks,
		workflowTask: wf,
	}
}

func (r *ResumableWorkflowRunner) GetWorkflowTask() *models.WorkflowTask {
	return r.workflowTask
}

func (r *ResumableWorkflowRunner) GetLogger() *models.LogBuilder {
	return r.GetWorkflowTask().GetLogger()
}

func (r *ResumableWorkflowRunner) GetTaskList() *model.TaskList {
	return r.GetWorkflowTask().GetTaskList()
}

func (m *ResumableWorkflowRunner) GetWorkflow() *model.Workflow {
	return m.workflowTask.GetWorkflowDef()
}

// NewResumableRunner creates a new resumable workflow runner
func NewResumableRunner(config Config, workflow *models.WorkflowTask) *ResumableWorkflowRunner {
	return &ResumableWorkflowRunner{
		config:       config,
		workflowTask: workflow,
	}
}

// Run executes the workflow synchronously.
func (wr *ResumableWorkflowRunner) Run(input any) (output any, err error) {

	workflowTask := wr.GetWorkflowTask()
	log := wr.GetLogger()

	defer func() {

		// An "error" will be thrown if. the workflow needs to await an external event
		// In this case, we do not want to mark the workflow as Faulted
		// The workflow will be resumed later when the event is received
		// So we only mark it as Faulted if the error is not ErrAwaitingEvent
		if err != nil && errors.Is(err, ErrorAwaitSignal) {

			// Mark the workflow as Waiting
			workflowTask.SetStatus(swctx.WaitingStatus)
			err = nil

		} else if err != nil {

			// Wrap the error to ensure it has a proper instance reference
			workflowTask.SetStatus(swctx.FaultedStatus)
			err = wr.wrapWorkflowError(err)
		}

	}()

	workflowTask.SetRawInput(input)

	// Process input
	if input, err = wr.processInput(input); err != nil {
		return nil, err
	}

	workflowTask.SetInput(input)

	// Run tasks sequentially
	workflowTask.SetStatus(swctx.RunningStatus)
	workflowTask.SetStartedAt(time.Now())

	// Check if we have a valid state to resume from
	idx := 0

	// Do we need to resume from an entrypoint?
	// This only support root level entrypoints for now
	if workflowTask.HasEntrypoint() {

		foundIdx, err := workflowTask.GetEntrypointIndex()

		if err != nil {
			return nil, err
		}

		idx = foundIdx

	}

	output, err = wr.resumeTaskList(
		workflowTask.GetWorkflowDef().Do,
		idx,
		workflowTask.GetInput(),
	)

	log.WithFields(models.Fields{
		"resumeTaskListOutput": output,
		"resumeTaskListError":  err,
	}).Info("Task list execution completed")

	if err != nil {
		return nil, err
	}

	// Clear the local task context - post task execution
	workflowTask.ClearTaskContext()

	// Process output
	if output, err = wr.processOutput(output); err != nil {
		return nil, err
	}

	log.WithFields(models.Fields{
		"processedOutput": output,
	}).Info("Output processing completed")

	wr.workflowTask.SetOutput(output)
	wr.workflowTask.SetStatus(swctx.CompletedStatus)

	return output, nil
}

// wrapWorkflowError ensures workflow errors have a proper instance reference.
func (wr *ResumableWorkflowRunner) wrapWorkflowError(err error) error {

	taskReference := wr.workflowTask.GetTaskReference()

	if len(taskReference) == 0 {
		taskReference = "/"
	}

	if knownErr := model.AsError(err); knownErr != nil {
		return knownErr.WithInstanceRef(wr.GetWorkflow(), taskReference)
	}

	// First unwrap ActivityError if present, then check the underlying error type
	var activityErr *temporal.ActivityError
	unwrappedErr := err
	if errors.As(err, &activityErr) {
		if innerErr := errors.Unwrap(activityErr); innerErr != nil {
			unwrappedErr = innerErr
		}
	}

	// Handle Temporal ApplicationError
	if temporal.IsApplicationError(unwrappedErr) {
		var appErr *temporal.ApplicationError
		if errors.As(unwrappedErr, &appErr) {
			return model.NewErrRuntime(
				fmt.Errorf("workflow '%s', task '%s' error: %v",
					wr.GetWorkflow().Document.Name,
					taskReference,
					appErr.Error(),
				),
				taskReference,
			)
		}
	}

	return model.NewErrRuntime(
		fmt.Errorf("workflow '%s', task '%s': %w", wr.GetWorkflow().Document.Name, taskReference, err),
		taskReference,
	)
}

// processOutput applies output transformations.
func (wr *ResumableWorkflowRunner) processOutput(output any) (any, error) {

	workflow := wr.GetWorkflow()
	log := wr.GetLogger()

	if workflow.Output != nil {
		if workflow.Output.As != nil {
			var err error
			output, err = wr.GetWorkflowTask().
				TraverseAndEvaluateObj(workflow.Output.As, output, "/")
			if err != nil {

				log.WithError(err).Error("Failed to apply output 'as' transformation")

				return nil, err
			}
		}
		if workflow.Output.Schema != nil {

			log.WithField("workflow", workflow.Document.Name).Debug("Validating output against schema")

			if err := utils.ValidateSchema(output, workflow.Output.Schema, "/"); err != nil {

				log.WithError(err).Error("Output validation against schema failed")

				return nil, err
			}
		}
	}
	return output, nil
}

// processInput validates and transforms input if needed.
func (wr *ResumableWorkflowRunner) processInput(input any) (output any, err error) {

	workflow := wr.GetWorkflow()
	log := wr.GetLogger()

	if workflow.Input != nil {
		if workflow.Input.Schema != nil {
			if err = utils.ValidateSchema(input, workflow.Input.Schema, "/"); err != nil {

				log.WithError(err).Error("Input validation against schema failed")

				return nil, err
			}
		}

		if workflow.Input.From != nil {
			output, err = wr.GetWorkflowTask().TraverseAndEvaluateObj(workflow.Input.From, input, "/")
			if err != nil {

				log.WithError(err).Error("Failed to apply input 'from' transformation")

				return nil, err
			}
			return output, nil
		}
	}
	return input, nil
}

// updateTemporalSearchAttributes updates the workflow search attributes
func (wr *ResumableWorkflowRunner) updateTemporalSearchAttributes(
	currentTask *model.TaskItem,
	status swctx.StatusPhase,
) error {

	if !wr.workflowTask.HasTemporalContext() {
		return nil
	}

	workflowTask := wr.GetWorkflowTask()
	log := workflowTask.GetLogger()

	ctx := workflowTask.GetTemporalContext()

	updates := []temporal.SearchAttributeUpdate{
		models.TypedSearchAttributeStatus.ValueSet(string(status)),
	}

	isApproved := workflowTask.IsApproved()

	if isApproved != nil {
		updates = append(updates,
			models.TypedSearchAttributeApproved.ValueSet(*isApproved),
		)
	}

	if currentTask != nil && len(currentTask.Key) > 0 {
		updates = append(updates,
			models.TypedSearchAttributeTask.ValueSet(currentTask.Key),
		)
	}

	elevationRequest, err := workflowTask.GetContextAsElevationRequest()

	if err != nil {

		log.WithError(err).Warn("No valid elevation context found, skipping search attribute update.")

	} else {
		if elevationRequest.User != nil && len(elevationRequest.User.Email) > 0 {
			updates = append(updates,
				models.TypedSearchAttributeUser.ValueSet(elevationRequest.User.Email),
			)
		}

		if len(elevationRequest.Role.Name) > 0 {
			updates = append(updates,
				models.TypedSearchAttributeRole.ValueSet(elevationRequest.Role.Name),
			)
		}

		if len(elevationRequest.Workflow) > 0 {
			updates = append(updates,
				models.TypedSearchAttributeWorkflow.ValueSet(elevationRequest.Workflow),
			)
		}

		if len(elevationRequest.Providers) > 0 {
			updates = append(updates,
				models.TypedSearchAttributeProviders.ValueSet(elevationRequest.Providers),
			)
		}

		if len(elevationRequest.Identities) > 0 {
			updates = append(updates,
				models.TypedSearchAttributeIdentities.ValueSet(elevationRequest.Identities),
			)
		}

	}

	log.WithFields(models.Fields{
		"workflowID": workflowTask.WorkflowID,
	}).Info("Updating temporal search attributes")

	return workflow.UpsertTypedSearchAttributes(ctx, updates...)
}
