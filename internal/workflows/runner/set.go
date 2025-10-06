package runner

import (
	"fmt"

	utils "github.com/serverlessworkflow/sdk-go/v3/impl/utils"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
)

/*
A task used to set data.

The Set task allows setting data using either:
1. A map/object with key-value pairs
2. A direct runtime expression that evaluates to the data

Examples:

	document:
	  dsl: '1.0.1'
	  namespace: default
	  name: set-example
	  version: '0.1.0'
	do:
	  - setShape:
	      set:
	        shape: circle
	        size: ${ .configuration.size }
	        fill: ${ .configuration.fill }
	  - setColor:
	      set: ${ .configuration.color }
*/
func (r *ResumableWorkflowRunner) executeSetTask(
	taskName string,
	task *model.SetTask,
	input any,
) (any, error) {

	logrus.WithFields(logrus.Fields{
		"task": taskName,
		"set":  task.Set,
	}).Info("Executing Set task")

	// Deep clone the set object to avoid modifying the original
	setObject := utils.DeepClone(task.Set)

	workflowTask := r.GetWorkflowTask()

	// Evaluate the set object which can be either a map or a runtime expression
	result, err := workflowTask.TraverseAndEvaluateObj(
		model.NewObjectOrRuntimeExpr(setObject), input, taskName)

	if err != nil {

		logrus.WithFields(logrus.Fields{
			"task":  taskName,
			"input": input,
		}).WithError(err).Error("Failed to evaluate set expression")

		return nil, model.NewErrRuntime(fmt.Errorf("failed to evaluate set expression: %w", err), taskName)
	}

	return result, nil
}
