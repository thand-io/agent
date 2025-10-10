package runner

import (
	"fmt"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	utils "github.com/serverlessworkflow/sdk-go/v3/impl/utils"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
)

func (r *ResumableWorkflowRunner) executeDoRunner(
	doTask *model.DoTask, input any) (any, error) {
	return r.resumeTasks(doTask.Do, 0, input)
}

func (r *ResumableWorkflowRunner) executeTaskList(
	taskList *model.TaskList, input any) (any, error) {
	return r.resumeTasks(taskList, 0, input)
}

func (d *ResumableWorkflowRunner) resumeTaskList(
	taskList *model.TaskList, idx int, input any) (output any, err error) {
	return d.resumeTasks(taskList, idx, input)
}

// runTasks runs all defined tasks sequentially.
func (d *ResumableWorkflowRunner) resumeTasks(
	taskList *model.TaskList, idx int, input any) (output any, err error) {

	if taskList == nil {
		return input, nil
	}

	taskSupport := d.GetWorkflowTask()

	output = input

	taskListRef := *taskList

	// check if index is valid
	if idx < 0 || idx >= len(taskListRef) {
		return output, fmt.Errorf("invalid task index %d", idx)
	}

	currentTask := taskListRef[idx]

	for currentTask != nil {
		if err = taskSupport.SetTaskDef(currentTask); err != nil {
			return nil, err
		}
		if err = taskSupport.SetTaskReferenceFromName(currentTask.Key); err != nil {
			return nil, err
		}

		if shouldRun, err := d.shouldRunTask(input, currentTask); err != nil {
			return output, err
		} else if !shouldRun {
			idx, currentTask = taskList.Next(idx)
			continue
		}

		taskSupport.SetTaskStatus(currentTask.Key, ctx.PendingStatus)

		// Check if this task is a SwitchTask and handle it
		if switchTask, ok := currentTask.Task.(*model.SwitchTask); ok {
			flowDirective, err := d.evaluateSwitchTask(input, currentTask.Key, switchTask)
			if err != nil {
				taskSupport.SetTaskStatus(currentTask.Key, ctx.FaultedStatus)
				return output, err
			}
			taskSupport.SetTaskStatus(currentTask.Key, ctx.CompletedStatus)

			// Process FlowDirective: update idx/currentTask accordingly
			idx, currentTask = taskList.KeyAndIndex(flowDirective.Value)
			if currentTask == nil {

				logrus.WithFields(logrus.Fields{
					"task":  currentTask,
					"error": err,
				}).Error("Flow directive target not found")

				return nil, fmt.Errorf(
					"flow directive target '%s' not found", flowDirective.Value)
			}
			continue
		}

		taskSupport.SetTaskStatus(currentTask.Key, ctx.RunningStatus)

		if output, err = d.runTaskItem(currentTask, input); err != nil {
			taskSupport.SetTaskStatus(currentTask.Key, ctx.FaultedStatus)
			return output, err
		}

		err := d.updateTemporalSearchAttributes()

		if err != nil {
			logrus.WithFields(logrus.Fields{
				"task":  currentTask,
				"error": err,
			}).Warn("Failed to update temporal search attributes")
		}

		taskSupport.SetTaskStatus(currentTask.Key, ctx.CompletedStatus)
		input = utils.DeepCloneValue(output)
		idx, currentTask = taskList.Next(idx)
	}

	return output, nil
}

// runTask executes an individual task.
func (d *ResumableWorkflowRunner) runTaskItem(
	task *model.TaskItem,
	input any,
) (output any, err error) {

	if task == nil || task.Task == nil {
		return nil, fmt.Errorf("invalid task")
	}

	taskName := task.Key

	taskSupport := d.GetWorkflowTask()

	taskSupport.SetTaskStartedAt(time.Now())
	taskSupport.SetTaskRawInput(input)
	taskSupport.SetTaskName(taskName)

	if task.GetBase().Input != nil {
		if input, err = d.processTaskInput(task.GetBase(), input, taskName); err == nil {
			taskSupport.SetTaskRawInput(input)
		} else {

			logrus.WithFields(logrus.Fields{
				"task": taskName,
			}).WithError(err).Error("Failed to process task input")

			return nil, err
		}
	}

	output, err = d.executeTask(task, input)

	if err != nil {

		logrus.WithFields(logrus.Fields{
			"task":  taskName,
			"error": err,
		}).Error("Task execution failed")

		return nil, err
	}

	taskSupport.SetTaskRawOutput(output)

	if output, err = d.processTaskOutput(task.GetBase(), output, taskName); err != nil {

		logrus.WithFields(logrus.Fields{
			"task": taskName,
		}).WithError(err).Error("Failed to process task output")

		return nil, err
	}

	if err = d.processTaskExport(task.GetBase(), output, taskName); err != nil {

		logrus.WithFields(logrus.Fields{
			"task": taskName,
		}).WithError(err).Error("Failed to process task export")

		return nil, err
	}

	return output, nil
}

func (r *ResumableWorkflowRunner) executeTask(
	task *model.TaskItem,
	input any,
) (any, error) {

	if task == nil || task.Task == nil {
		return nil, fmt.Errorf("invalid task")
	}

	return r.dispatchTaskExecution(task, input)
}

// dispatchTaskExecution handles the actual task execution logic using a type-based dispatcher
func (r *ResumableWorkflowRunner) dispatchTaskExecution(
	task *model.TaskItem,
	input any,
) (any, error) {
	taskName := task.Key

	switch t := task.Task.(type) {
	case *model.CallFunction:
		return r.executeCallFunction(taskName, task.AsCallFunctionTask(), input)
	case *model.CallHTTP:
		return r.executeHttpFunction(taskName, task.AsCallHTTPTask(), input)
	case *model.CallAsyncAPI:
		return r.executeAsyncFunction(taskName, task.AsCallAsyncAPITask(), input)
	case *model.CallOpenAPI:
		return r.executeOpenAPIFunction(taskName, task.AsCallOpenAPITask(), input)
	case *model.CallGRPC:
		return r.executeGRPCFunction(taskName, task.AsCallGRPCTask(), input)
	case *model.SetTask:
		return r.executeSetTask(taskName, task.AsSetTask(), input)
	case *model.ForTask:
		return r.executeForTask(taskName, task.AsForTask(), input)
	case *model.TryTask:
		return r.executeTryTask(taskName, task.AsTryTask(), input)
	case *model.WaitTask:
		return r.executeWaitTask(taskName, task.AsWaitTask(), input)
	case *model.ListenTask:
		return r.executeListenTask(taskName, task.AsListenTask(), input)
	case *model.RaiseTask:
		return r.executeRaiseTask(taskName, task.AsRaiseTask(), input)
	case *model.EmitTask:
		return r.executeEmitTask(taskName, task.AsEmitTask(), input)
	case *model.RunTask:
		return r.executeRunTask(taskName, task.AsRunTask(), input)
	case *model.ForkTask:
		return r.executeForkTask(taskName, task.AsForkTask(), input)
	case *model.DoTask:
		return r.executeDoRunner(task.AsDoTask(), input)
	default:
		return nil, fmt.Errorf("unsupported task type %T", t)
	}
}
