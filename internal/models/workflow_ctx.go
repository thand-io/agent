package models

import (
	"context"
	"fmt"
	"maps"
	"time"

	swctx "github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"github.com/serverlessworkflow/sdk-go/v3/impl/utils"
	"github.com/serverlessworkflow/sdk-go/v3/model"
)

func NewWorkflowContext(workflow *Workflow) (*WorkflowTask, error) {

	workflowID := fmt.Sprintf("wf_%d", time.Now().UTC().UnixNano())

	workflowCtx := WorkflowTask{
		state:                  NewWorkflowTaskState(),
		Status:                 swctx.PendingStatus,
		AuthenticationProvider: workflow.GetAuthentication(),
		WorkflowID:             workflowID,
		WorkflowName:           workflow.GetName(),
		Workflow:               workflow.Workflow,
		Context:                map[string]any{},
		internalContext:        context.Background(),
	}

	return &workflowCtx, nil
}

// WithWorkflowContext adds the workflowContext to a parent context
func WithWorkflowContext(parent context.Context, wfCtx swctx.WorkflowContext) context.Context {
	return context.WithValue(parent, runnerCtxKey, wfCtx)
}

// GetWorkflowContext retrieves the workflowContext from a context
func GetWorkflowContext(ctx context.Context) (swctx.WorkflowContext, error) {
	wfCtx, ok := ctx.Value(runnerCtxKey).(*WorkflowTask)
	if !ok {
		return nil, swctx.ErrWorkflowContextNotFound
	}
	return wfCtx, nil
}

// Clone creates a deep copy of the WorkflowTask for safe concurrent use.
// Each clone gets its own mutex and state to prevent data races in forked workflows.
func (ctx *WorkflowTask) Clone() swctx.WorkflowContext {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	return &WorkflowTask{
		// Deep clone mutable fields
		Input:            utils.DeepCloneValue(ctx.Input),
		Output:           utils.DeepCloneValue(ctx.Output),
		Context:          utils.DeepCloneValue(ctx.Context),
		state:            ctx.cloneState(),
		localExprVars:    utils.DeepClone(ctx.localExprVars),
		StatusPhase:      append([]swctx.StatusPhaseLog(nil), ctx.StatusPhase...),
		TasksStatusPhase: ctx.cloneTasksStatusPhase(),

		// Copy read-only/shared fields
		AuthenticationProvider: ctx.AuthenticationProvider,
		WorkflowID:             ctx.WorkflowID,
		Workflow:               ctx.Workflow,
		internalContext:        ctx.internalContext,
	}
}

// cloneState creates a deep copy of the workflow task state.
func (ctx *WorkflowTask) cloneState() *WorkflowTaskState {
	if ctx.state == nil {
		return NewWorkflowTaskState()
	}

	// Copy struct by value to get new memory address
	cp := *ctx.state
	cp.Input = utils.DeepCloneValue(ctx.state.Input)
	cp.Output = utils.DeepCloneValue(ctx.state.Output)
	return &cp
}

// cloneTasksStatusPhase creates a deep copy of task status phases.
func (ctx *WorkflowTask) cloneTasksStatusPhase() map[string][]swctx.StatusPhaseLog {
	result := make(map[string][]swctx.StatusPhaseLog, len(ctx.TasksStatusPhase))
	for taskName, logs := range ctx.TasksStatusPhase {
		result[taskName] = append([]swctx.StatusPhaseLog(nil), logs...)
	}
	return result
}

func (ctx *WorkflowTask) SetStartedAt(t time.Time) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.StartedAt = t
}

func (ctx *WorkflowTask) SetRawInput(input any) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.state.Input = input
}

func (ctx *WorkflowTask) AddLocalExprVars(vars map[string]any) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.localExprVars == nil {
		ctx.localExprVars = map[string]any{}
	}
	maps.Copy(ctx.localExprVars, vars)
}

func (ctx *WorkflowTask) RemoveLocalExprVars(keys ...string) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	if ctx.localExprVars == nil {
		return
	}

	for _, k := range keys {
		delete(ctx.localExprVars, k)
	}
}

func (ctx *WorkflowTask) SetLocalExprVars(vars map[string]any) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.localExprVars = vars
}

// GetVars returns all available variables for expression evaluation
func (ctx *WorkflowTask) GetVars() map[string]any {
	workflow := ctx.getWorkflowDefAsMap()

	vars := map[string]any{
		varsInput:   ctx.GetInput(),
		varsOutput:  ctx.GetOutput(),
		varsContext: ctx.GetContextAsMap(),
		varsTask:    ctx.GetStateAsMap(),
		varsWorkflow: map[string]any{
			"id":         ctx.WorkflowID,
			"definition": workflow,
		},
		varsRuntime: map[string]any{
			"name":    runtimeName,
			"version": runtimeVersion,
		},
	}

	ctx.mu.Lock()
	maps.Copy(vars, ctx.localExprVars)
	ctx.mu.Unlock()

	return vars
}

// getWorkflowDefAsMap safely converts workflow definition to map
func (ctx *WorkflowTask) getWorkflowDefAsMap() map[string]any {
	if wkflw := ctx.GetWorkflowDef(); wkflw != nil {
		if found, err := wkflw.AsMap(); err == nil {
			return found
		}
	}
	return map[string]any{}
}

func (ctx *WorkflowTask) SetStatus(status swctx.StatusPhase) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.StatusPhase == nil {
		ctx.StatusPhase = []swctx.StatusPhaseLog{}
	}
	ctx.StatusPhase = append(ctx.StatusPhase, swctx.NewStatusPhaseLog(status))
}

// SetInstanceCtx safely sets the `$context` value
func (ctx *WorkflowTask) SetInstanceCtx(value any) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.Context = value
}

// GetInstanceCtx safely retrieves the `$context` value
func (ctx *WorkflowTask) GetInstanceCtx() any {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.Context == nil {
		return nil
	}
	return ctx.Context
}

// SetInput safely sets the input
func (ctx *WorkflowTask) SetInput(input any) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.Input = input
}

// GetInput safely retrieves the input
func (ctx *WorkflowTask) GetInput() any {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.Input
}

// SetOutput safely sets the output
func (ctx *WorkflowTask) SetOutput(output any) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.Output = output
}

// GetOutput safely retrieves the output
func (ctx *WorkflowTask) GetOutput() any {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.Output
}

// GetInputAsMap safely retrieves the input as a map[string]any.
// If input is not a map, it wraps it in a map with "input" as the key.
func (ctx *WorkflowTask) GetInputAsMap() map[string]any {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	if ctx.Input == nil {
		return map[string]any{}
	}

	if inputMap, ok := ctx.Input.(map[string]any); ok {
		return inputMap
	}

	return map[string]any{"input": ctx.Input}
}

func (ctx *WorkflowTask) GetContextAsMap() map[string]any {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	if ctx.Context == nil {
		return map[string]any{}
	}

	if contextMap, ok := ctx.Context.(map[string]any); ok {
		return contextMap
	}

	return map[string]any{"context": ctx.Context}
}

// GetOutputAsMap safely retrieves the output as a map[string]any.
// If output is not a map, it wraps it in a map with "output" as the key.
func (ctx *WorkflowTask) GetOutputAsMap() map[string]any {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	if ctx.Output == nil {
		return map[string]any{}
	}

	if outputMap, ok := ctx.Output.(map[string]any); ok {
		return outputMap
	}

	return map[string]any{"output": ctx.Output}
}

func (ctx *WorkflowTask) SetTaskStatus(task string, status swctx.StatusPhase) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.TasksStatusPhase == nil {
		ctx.TasksStatusPhase = map[string][]swctx.StatusPhaseLog{}
	}
	ctx.TasksStatusPhase[task] = append(ctx.TasksStatusPhase[task], swctx.NewStatusPhaseLog(status))
}

func (ctx *WorkflowTask) SetTaskRawInput(input any) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.state.Input = input
}

func (ctx *WorkflowTask) SetTaskRawOutput(output any) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.state.Output = output
}

func (ctx *WorkflowTask) SetTaskDef(def model.Task) error {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.state.Definition = def
	return nil
}

func (ctx *WorkflowTask) SetTaskStartedAt(startedAt time.Time) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.state.StartedAt = startedAt
}

func (ctx *WorkflowTask) SetTaskName(name string) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.state.Name = name
	ctx.Entrypoint = name
}

func (ctx *WorkflowTask) SetTaskReference(ref string) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.state.Reference = ref
}

func (ctx *WorkflowTask) GetTaskReference() string {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.state.Reference
}

func (ctx *WorkflowTask) ClearTaskContext() {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.state = NewWorkflowTaskState()
}
