package runner

import (
	"encoding/json"
	"fmt"

	utils "github.com/serverlessworkflow/sdk-go/v3/impl/utils"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
)

func (d *esumableWorkflowRunner) shouldRunTask(input any, task *model.TaskItem) (bool, error) {

	if task.GetBase().If != nil {
		output, err := d.GetWorkflowTask().TraverseAndEvaluateBool(task.GetBase().If.String(), input)
		if err != nil {
			return false, model.NewErrExpression(err, task.Key)
		}
		return output, nil
	}
	return true, nil
}

// processTaskInput processes task input validation and transformation.
func (d *esumableWorkflowRunner) processTaskInput(task *model.TaskBase, taskInput any, taskName string) (output any, err error) {

	if task.Input == nil {
		return taskInput, nil
	}

	log := d.GetLogger()

	if err = utils.ValidateSchema(taskInput, task.Input.Schema, taskName); err != nil {

		log.WithFields(logrus.Fields{
			"task": taskName,
		}).WithError(err).Error("Failed to validate task input schema")

		return nil, err
	}

	if output, err = d.GetWorkflowTask().TraverseAndEvaluateObj(task.Input.From, taskInput, taskName); err != nil {

		log.WithFields(logrus.Fields{
			"task": taskName,
		}).WithError(err).Error("Failed to process task input")

		return nil, err
	}

	return output, nil
}

// processTaskOutput processes task output validation and transformation.
func (d *esumableWorkflowRunner) processTaskOutput(task *model.TaskBase, taskOutput any, taskName string) (output any, err error) {

	if task.Output == nil {
		return taskOutput, nil
	}

	log := d.GetLogger()

	if output, err = d.GetWorkflowTask().TraverseAndEvaluateObj(task.Output.As, taskOutput, taskName); err != nil {

		log.WithFields(logrus.Fields{
			"task": taskName,
		}).WithError(err).Error("Failed to process task output")

		return nil, err
	}

	if err = utils.ValidateSchema(output, task.Output.Schema, taskName); err != nil {

		log.WithFields(logrus.Fields{
			"task": taskName,
		}).WithError(err).Error("Failed to validate task output schema")

		return nil, err
	}

	return output, nil
}

func (d *esumableWorkflowRunner) processTaskExport(task *model.TaskBase, taskOutput any, taskName string) (err error) {

	taskSupport := d.GetWorkflowTask()
	log := d.GetLogger()

	if task.Export == nil {
		return nil
	}

	output, err := d.GetWorkflowTask().TraverseAndEvaluateObj(task.Export.As, taskOutput, taskName)
	if err != nil {

		data, _ := json.Marshal(taskOutput)

		fmt.Println(string(data))

		log.WithFields(logrus.Fields{
			"task": taskName,
		}).WithError(err).Error("Failed to process task export")

		return err
	}

	if err = utils.ValidateSchema(output, task.Export.Schema, taskName); err != nil {

		log.WithFields(logrus.Fields{
			"task": taskName,
		}).WithError(err).Error("Failed to validate task export schema")

		return nil
	}

	taskSupport.SetWorkflowInstanceCtx(output)

	return nil
}
