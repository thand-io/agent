package runner

import (
	"fmt"

	"github.com/serverlessworkflow/sdk-go/v3/model"
)

// executeRaiseTask handles error raising
func (r *ResumableWorkflowRunner) executeRaiseTask(
	taskName string,
	raise *model.RaiseTask,
	input any,
) (output any, err error) {

	if err := resolveErrorDefinition(raise, r.GetWorkflow()); err != nil {
		return nil, err
	}

	if raise.Raise.Error.Definition == nil {
		return nil, model.NewErrValidation(
			fmt.Errorf("no raise configuration provided for RaiseTask %s", taskName), taskName)
	}

	taskSupport := r.GetWorkflowTask()

	output = input

	// TODO: make this an external func so we can call it after getting the reference? Or we can get the reference from the workflow definition
	var detailResult any
	detailResult, err = taskSupport.TraverseAndEvaluateObj(
		raise.Raise.Error.Definition.Detail.AsObjectOrRuntimeExpr(), input, taskName)
	if err != nil {
		return nil, err
	}

	var titleResult any
	titleResult, err = taskSupport.TraverseAndEvaluateObj(
		raise.Raise.Error.Definition.Title.AsObjectOrRuntimeExpr(), input, taskName)
	if err != nil {
		return nil, err
	}

	instance := taskSupport.GetTaskReference()

	var raiseErr *model.Error
	if raiseErrF, ok := raiseErrFuncMapping[raise.Raise.Error.Definition.Type.String()]; ok {
		raiseErr = raiseErrF(fmt.Errorf("%v", detailResult), instance)
	} else {
		raiseErr = raise.Raise.Error.Definition
		raiseErr.Detail = model.NewStringOrRuntimeExpr(fmt.Sprintf("%v", detailResult))
		raiseErr.Instance = &model.JsonPointerOrRuntimeExpression{Value: instance}
	}

	raiseErr.Title = model.NewStringOrRuntimeExpr(fmt.Sprintf("%v", titleResult))
	err = raiseErr

	return output, err
}

var raiseErrFuncMapping = map[string]func(error, string) *model.Error{
	model.ErrorTypeAuthentication: model.NewErrAuthentication,
	model.ErrorTypeValidation:     model.NewErrValidation,
	model.ErrorTypeCommunication:  model.NewErrCommunication,
	model.ErrorTypeAuthorization:  model.NewErrAuthorization,
	model.ErrorTypeConfiguration:  model.NewErrConfiguration,
	model.ErrorTypeExpression:     model.NewErrExpression,
	model.ErrorTypeRuntime:        model.NewErrRuntime,
	model.ErrorTypeTimeout:        model.NewErrTimeout,
}

// TODO: can e refactored to a definition resolver callable from the context
func resolveErrorDefinition(t *model.RaiseTask, workflowDef *model.Workflow) error {
	if workflowDef != nil && t.Raise.Error.Ref != nil {
		notFoundErr := model.NewErrValidation(fmt.Errorf("%v error definition not found in 'uses'", t.Raise.Error.Ref), "")
		if workflowDef.Use != nil && workflowDef.Use.Errors != nil {
			definition, ok := workflowDef.Use.Errors[*t.Raise.Error.Ref]
			if !ok {
				return notFoundErr
			}
			t.Raise.Error.Definition = definition
			return nil
		}
		return notFoundErr
	}
	return nil
}
