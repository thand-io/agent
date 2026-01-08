package runner

import (
	"reflect"
	"strings"

	"github.com/serverlessworkflow/sdk-go/v3/model"
)

const (
	forTaskDefaultEach = "$item"
	forTaskDefaultAt   = "$index"
)

// executeForTask handles for loops
func (r *ResumableWorkflowRunner) executeForTask(
	taskName string,
	forTask *model.ForTask,
	input any,
) (any, error) {
	// Simplified for loop - in real implementation, you'd handle iteration

	taskSupport := r.GetWorkflowTask()

	defer func() {
		// clear local variables
		taskSupport.RemoveLocalExprVars(forTask.For.Each, forTask.For.At)
	}()

	sanitizeFor(forTask)

	in, err := taskSupport.TraverseAndEvaluate(forTask.For.In, input)
	if err != nil {
		return nil, err
	}

	forOutput := input
	rv := reflect.ValueOf(in)
	switch rv.Kind() {
	case reflect.Slice, reflect.Array:
		for i := 0; i < rv.Len(); i++ {
			item := rv.Index(i).Interface()

			if forOutput, err = r.processForItem(forTask, i, item, forOutput); err != nil {
				return nil, err
			}
			if len(forTask.While) > 0 {
				whileIsTrue, err := taskSupport.TraverseAndEvaluateBool(forTask.While, forOutput)
				if err != nil {
					return nil, err
				}
				if !whileIsTrue {
					break
				}
			}
		}
	case reflect.Invalid:
		return input, nil
	default:
		if forOutput, err = r.processForItem(forTask, 0, in, forOutput); err != nil {
			return nil, err
		}
	}

	return forOutput, nil
}

func (r *ResumableWorkflowRunner) processForItem(forTask *model.ForTask, idx int, item any, forOutput any) (any, error) {

	taskSupport := r.GetWorkflowTask()

	forVars := map[string]any{
		forTask.For.At:   idx,
		forTask.For.Each: item,
	}
	// Instead of Set, we Add since other tasks in this very same context might be adding variables to the context
	taskSupport.AddLocalExprVars(forVars)
	// output from previous iterations are merged together

	var err error

	// NewDoTaskRunner.Run() = executeTaskList
	forOutput, err = r.executeTaskList(forTask.Do, forOutput)
	if err != nil {
		return nil, err
	}

	return forOutput, nil
}

func sanitizeFor(forTask *model.ForTask) {
	forTask.For.Each = strings.TrimSpace(forTask.For.Each)
	forTask.For.At = strings.TrimSpace(forTask.For.At)

	if len(forTask.For.Each) == 0 {
		forTask.For.Each = forTaskDefaultEach
	}
	if len(forTask.For.At) == 0 {
		forTask.For.At = forTaskDefaultAt
	}

	if !strings.HasPrefix(forTask.For.Each, "$") {
		forTask.For.Each = "$" + forTask.For.Each
	}
	if !strings.HasPrefix(forTask.For.At, "$") {
		forTask.For.At = "$" + forTask.For.At
	}
}
