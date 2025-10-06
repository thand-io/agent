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

	taskSupport := r.GetWorkflowTask()

	// Execute the function call

	functionHandler, exists := r.functions.GetFunction(call.Call)

	if !exists {
		return nil, fmt.Errorf("function %s not found", call.Call)
	}

	// Interpolate the call.With parameters using the workflow input
	interpolatedCall := *call // Create a copy
	if call.With != nil {

		interpolatedWith, err := taskSupport.TraverseAndEvaluate(
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

	workflowTask := r.GetWorkflowTask()
	serviceClient := r.config.GetServices()

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
			taskSupport,
			taskName,
			interpolatedCall, // Use the interpolated call
			input,
		)

		var output any

		err := fut.Get(ctx, &output)

		if err != nil {
			return nil, fmt.Errorf("failed to execute Set task activity: %w", err)
		}

		return output, nil

	} else {

		// Validate input using the interpolated call
		err := functionHandler.ValidateRequest(
			taskSupport,
			&interpolatedCall,
			input,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to validate function %s: %w", call.Call, err)
		}

		result, err := functionHandler.Execute(
			taskSupport,
			&interpolatedCall, // Use the interpolated call
			input,
		)

		if err != nil {
			return nil, fmt.Errorf("failed to execute function %s: %w", call.Call, err)
		}

		return result, nil

	}
}
