package runner

import (
	"context"
	"errors"
	"fmt"
	"time"

	swctx "github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	utils "github.com/serverlessworkflow/sdk-go/v3/impl/utils"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/sdk/workflows/config"
	"github.com/thand-io/agent/sdk/workflows/functions"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"github.com/thand-io/agent/sdk/workflows/tasks"
	"go.temporal.io/sdk/temporal"
)

// ResumableWorkflowRunner implements a workflow runner that can pause and resume
type ResumableWorkflowRunner struct {
	config config.RunnerConfig
}

func NewResumableWorkflowRunner(
	config config.RunnerConfig,
) *ResumableWorkflowRunner {
	return &ResumableWorkflowRunner{
		config: config,
	}
}

func (r *ResumableWorkflowRunner) GetConfig() config.Config {
	return r.config.GetConfig()
}

func (r *ResumableWorkflowRunner) GetFunctions() *functions.FunctionRegistry {
	return r.GetConfig().GetFunctionRegistry()
}

func (r *ResumableWorkflowRunner) GetFunction(name string) (functions.Function, bool) {
	return r.GetFunctions().GetFunction(name)
}

func (r *ResumableWorkflowRunner) GetTasks() *tasks.TaskRegistry {
	return r.GetConfig().GetTaskRegistry()
}

func (r *ResumableWorkflowRunner) CreateRunner(sdkM sdkWorkflowsModel.WorkflowTaskSupport) config.RunnerConfig {
	return r.config.GetConfig().CreateRunner(sdkM)
}

func (r *ResumableWorkflowRunner) GetTaskHandler(taskItem *model.TaskItem) (tasks.Task, bool) {
	return r.GetTasks().GetTaskHandler(taskItem)
}

func (r *ResumableWorkflowRunner) GetContext() context.Context {
	ctx := r.GetWorkflowTask().GetContext()
	return sdkWorkflowsModel.WithWorkflowContext(ctx, r.GetWorkflowTask())
}

func (r *ResumableWorkflowRunner) GetWorkflowTask() sdkWorkflowsModel.WorkflowTaskSupport {
	return r.config.GetWorkflowTask()
}

func (r *ResumableWorkflowRunner) GetLogger() *sdkWorkflowsModel.LogBuilder {
	return r.GetWorkflowTask().GetLogger()
}

func (r *ResumableWorkflowRunner) GetTaskList() *model.TaskList {
	return r.GetWorkflowTask().GetTaskList()
}

func (m *ResumableWorkflowRunner) GetWorkflow() *model.Workflow {
	return m.GetWorkflowTask().GetWorkflowDef()
}

// Run executes the workflow synchronously.
func (wr *ResumableWorkflowRunner) Run(input any) (output any, err error) {

	if wr.GetWorkflow() == nil {
		return nil, fmt.Errorf("workflow definition is nil")
	}

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

	log.WithFields(logrus.Fields{
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

	log.WithFields(logrus.Fields{
		"processedOutput": output,
	}).Info("Output processing completed")

	wr.GetWorkflowTask().SetOutput(output)
	wr.GetWorkflowTask().SetStatus(swctx.CompletedStatus)

	return output, nil
}

// wrapWorkflowError ensures workflow errors have a proper instance reference.
func (wr *ResumableWorkflowRunner) wrapWorkflowError(err error) error {

	taskReference := wr.GetWorkflowTask().GetTaskReference()

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
