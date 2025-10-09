package runner

import (
	"fmt"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"go.temporal.io/sdk/workflow"
)

/*
Enables the execution of a specified function within a workflow,
allowing seamless integration with custom business logic or external services.

document:

	dsl: '1.0.0'
	namespace: test
	name: call-example
	version: '0.1.0'

do:
  - getPet:
    call: http
    with:
    method: get
    endpoint: https://petstore.swagger.io/v2/pet/{petId}
*/
func (r *ResumableWorkflowRunner) executeCallFunction(
	taskName string,
	call *model.CallFunction,
	input any,
) (any, error) {

	logrus.WithFields(logrus.Fields{
		"task": taskName,
		"call": call.Call,
	}).Info("Executing function call")

	workflowTask := r.GetWorkflowTask()

	// Execute the function call

	functionHandler, exists := r.functions.GetFunction(call.Call)

	if !exists {
		return nil, fmt.Errorf("function %s not found", call.Call)
	}

	// Interpolate the call.With parameters using the workflow input
	interpolatedCall := *call // Create a copy
	if call.With != nil {

		interpolatedWith, err := workflowTask.TraverseAndEvaluate(
			call.With, input)

		if err != nil {
			return nil, fmt.Errorf("failed to interpolate call.with: %w", err)
		}

		withMap, ok := interpolatedWith.(map[string]any)

		if !ok {
			return nil, fmt.Errorf("interpolated call.with is not a map[string]any")
		}

		interpolatedCall.With = withMap

	}

	// Validate input using the interpolated call
	err := functionHandler.ValidateRequest(
		workflowTask,
		&interpolatedCall,
		input,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to validate function %s: %w", call.Call, err)
	}

	serviceClient := r.config.GetServices()

	var output any

	if workflowTask.HasTemporalContext() && serviceClient.HasTemporal() {

		ctx := workflowTask.GetTemporalContext()

		if ctx == nil {
			return nil, fmt.Errorf("failed to get temporal context")
		}

		activityOptions := workflow.ActivityOptions{
			TaskQueue:           serviceClient.GetTemporal().GetTaskQueue(),
			StartToCloseTimeout: time.Minute * 5,
		}

		ctx = workflow.WithActivityOptions(ctx, activityOptions)

		/*
			workflowTask *models.WorkflowTask,
			callFunction *model.CallFunction,
			input any,
		*/
		fut := workflow.ExecuteActivity(
			ctx,
			call.Call, // Function name

			// args
			workflowTask,
			taskName,
			interpolatedCall, // Use the interpolated call
			input,
		)

		err := fut.Get(ctx, &output)

		if err != nil {
			return nil, fmt.Errorf("failed to execute Set task activity: %w", err)
		}

	} else {

		result, err := functionHandler.Execute(
			workflowTask,
			&interpolatedCall, // Use the interpolated call
			input,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to execute function %s: %w", call.Call, err)
		}

		output = result

	}

	if output != nil && call.Export == nil {

		// If no export is defined then lets check our calling function
		// to see if we want our output merged into the context

		exportHandler := functionHandler.GetExport()

		if exportHandler != nil {
			call.Export = exportHandler
		}

	}

	if output != nil && call.Output != nil {

		// If no export is defined then lets check our calling function
		// to see if we want our output merged into the context

		outputHandler := functionHandler.GetOutput()

		if outputHandler != nil {
			call.Output = outputHandler
		}
	}

	return output, nil

}
