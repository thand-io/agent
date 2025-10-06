package runner

import (
	"context"
	"fmt"
	"sync"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/workflow"
)

/*
Allows workflows to execute multiple subtasks concurrently, enabling parallel processing and improving
the overall efficiency of the workflow. By defining a set of subtasks to perform concurrently, the Fork
task facilitates the execution of complex operations in parallel, ensuring that multiple tasks can be
executed simultaneously.
*/
func (r *ResumableWorkflowRunner) executeForkTask(
	taskName string,
	task *model.ForkTask,
	input any,
) (any, error) {

	if task == nil || task.Fork.Branches == nil {
		return nil, model.NewErrValidation(fmt.Errorf("invalid Fork task %s", taskName), taskName)
	}

	taskList := *task.Fork.Branches
	parentWF := r.GetWorkflowTask()

	// Check if we're in a Temporal workflow context
	if parentWF.HasTemporalContext() {
		return r.executeForkTaskTemporal(taskName, task, input, taskList)
	} else {
		return r.executeForkTaskStandard(taskName, task, input, taskList)
	}
}

// executeForkTaskTemporal handles fork execution within a Temporal workflow context
func (r *ResumableWorkflowRunner) executeForkTaskTemporal(
	taskName string,
	task *model.ForkTask,
	input any,
	taskList []*model.TaskItem,
) (any, error) {

	parentWF := r.GetWorkflowTask()
	ctx := parentWF.GetTemporalContext()

	if ctx == nil {
		return nil, fmt.Errorf("failed to get temporal context for fork task %s", taskName)
	}

	n := len(taskList)
	results := make([]any, n)

	// Create workflow.Go goroutines for each branch
	type branchResult struct {
		index  int
		result any
		err    error
	}

	resultCh := workflow.NewChannel(ctx)

	for i, taskItem := range taskList {

		taskItem := taskItem // capture loop variable

		workflow.Go(ctx, func(ctx workflow.Context) {
			// Clone WorkflowTask for this branch
			clonedWF := parentWF.Clone()

			// Cast back to *WorkflowTask and set temporal context
			childWF, ok := clonedWF.(*models.WorkflowTask)
			if !ok {
				resultCh.Send(ctx, branchResult{index: i, err: fmt.Errorf("failed to cast cloned workflow to *WorkflowTask")})
				return
			}

			childWF = childWF.WithTemporalContext(ctx)

			// Create a new runner instance for this branch
			branchRunner := &ResumableWorkflowRunner{
				config:       r.config,
				functions:    r.functions,
				workflowTask: childWF,
			}

			// NewTaskRunner.Run() = executeTask
			out, err := branchRunner.executeTask(taskItem, input)
			resultCh.Send(ctx, branchResult{index: i, result: out, err: err})
		})
	}

	/*
		Indicates whether or not the concurrent tasks are racing against each other,
		with a single possible winner, which sets the composite task's output.
		If set to false, the task returns an array that includes the outputs from
		each branch, preserving the order in which the branches are declared.
		If to true, the task returns only the output of the winning branch.
		Defaults to false.
	*/
	// Collect results based on compete mode
	completed := 0
	var lastError error

	for completed < n {
		var result branchResult
		resultCh.Receive(ctx, &result)
		completed++

		if result.err != nil {
			lastError = result.err
		} else {
			// Successful result
			if task.Fork.Compete {
				// Return first successful result immediately
				return result.result, nil
			}
			// Store result for parallel mode
			results[result.index] = result.result
		}
	}

	// For parallel mode, return error if any occurred, otherwise return all results
	if !task.Fork.Compete {
		if lastError != nil {
			return nil, lastError
		}
		return results, nil
	}

	// If we reach here in compete mode, all branches failed
	return nil, lastError
}

// executeForkTaskStandard handles fork execution in standard Go context (non-Temporal)
func (r *ResumableWorkflowRunner) executeForkTaskStandard(
	taskName string,
	task *model.ForkTask,
	input any,
	taskList []*model.TaskItem,
) (any, error) {

	cancelCtx, cancel := context.WithCancel(r.GetContext())
	defer cancel()

	n := len(taskList)
	results := make([]any, n)
	errs := make(chan error, n)
	done := make(chan struct{})
	resultCh := make(chan any, 1)

	var (
		wg   sync.WaitGroup
		once sync.Once
	)

	parentWF := r.GetWorkflowTask() // shared parent; do NOT use in branches

	for i, taskItem := range taskList {
		wg.Add(1)
		go func(i int, taskItem *model.TaskItem) {
			defer wg.Done()

			select {
			case <-cancelCtx.Done():
				return
			default:
			}

			// Clone WorkflowTask for this branch and attach it to a branched context
			childWF := parentWF.Clone()
			branchedCtx := models.WithWorkflowContext(cancelCtx, childWF)

			// Clone the runner with the branched context
			// The tasks to perform concurrently.
			branchSupport := r.CloneWithContext(branchedCtx)

			// NewTaskRunner.Run() = executeTask
			out, err := branchSupport.executeTask(taskItem, input)
			if err != nil {
				errs <- err
				return
			}
			results[i] = out

			if task.Fork.Compete {
				once.Do(func() {
					select {
					case resultCh <- out:
					default:
					}
					cancel() // signal cancellation to other branches
					close(done)
				})
			}
		}(i, taskItem)
	}

	/*
		Indicates whether or not the concurrent tasks are racing against each other,
		with a single possible winner, which sets the composite task's output.
		If set to false, the task returns an array that includes the outputs from
		each branch, preserving the order in which the branches are declared.
		If to true, the task returns only the output of the winning branch.
		Defaults to false.
	*/
	if task.Fork.Compete {
		select {
		case <-done:
			return <-resultCh, nil
		case err := <-errs:
			return nil, err
		}
	}

	wg.Wait()
	select {
	case err := <-errs:
		return nil, err
	default:
	}
	return results, nil
}
