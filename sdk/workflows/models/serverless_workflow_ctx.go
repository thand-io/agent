package models

import (
	"context"
	"fmt"
	"maps"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	swctx "github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"github.com/serverlessworkflow/sdk-go/v3/impl/utils"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
)

func NewThandWorkflowContext(name string, workflow *model.Workflow) (*ServerlessWorkflowTask, error) {

	workflowID := fmt.Sprintf("wf_%d", time.Now().UTC().UnixNano())

	workflowCtx := ServerlessWorkflowTask{
		state:           NewWorkflowTaskState(),
		Status:          swctx.PendingStatus,
		WorkflowID:      workflowID,
		WorkflowName:    name,
		Workflow:        workflow,
		Context:         map[string]any{},
		internalContext: context.Background(),
	}

	return &workflowCtx, nil
}

// Clone creates a deep copy of the WorkflowTask for safe concurrent use.
// Each clone gets its own mutex and state to prevent data races in forked workflows.
func (ctx *ServerlessWorkflowTask) Clone() swctx.WorkflowContext {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	return &ServerlessWorkflowTask{
		// Deep clone mutable fields
		Input:            utils.DeepCloneValue(ctx.Input),
		Output:           utils.DeepCloneValue(ctx.Output),
		Context:          utils.DeepCloneValue(ctx.Context),
		state:            ctx.cloneState(),
		localExprVars:    utils.DeepClone(ctx.localExprVars),
		StatusPhase:      append([]swctx.StatusPhaseLog(nil), ctx.StatusPhase...),
		TasksStatusPhase: ctx.cloneTasksStatusPhase(),

		// Copy read-only/shared fields
		WorkflowID:      ctx.WorkflowID,
		Workflow:        ctx.Workflow,
		internalContext: ctx.internalContext,
	}
}

// cloneState creates a deep copy of the workflow task state.
func (ctx *ServerlessWorkflowTask) cloneState() *WorkflowTaskState {
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
func (ctx *ServerlessWorkflowTask) cloneTasksStatusPhase() map[string][]swctx.StatusPhaseLog {
	result := make(map[string][]swctx.StatusPhaseLog, len(ctx.TasksStatusPhase))
	for taskName, logs := range ctx.TasksStatusPhase {
		result[taskName] = append([]swctx.StatusPhaseLog(nil), logs...)
	}
	return result
}

func (ctx *ServerlessWorkflowTask) SetStartedAt(t time.Time) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.StartedAt = t
}

func (ctx *ServerlessWorkflowTask) SetRawInput(input any) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.state.Input = input
}

func (ctx *ServerlessWorkflowTask) SetRawOutput(output any) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.state.Output = output
}

func (ctx *ServerlessWorkflowTask) AddLocalExprVars(vars map[string]any) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.localExprVars == nil {
		ctx.localExprVars = map[string]any{}
	}
	maps.Copy(ctx.localExprVars, vars)
}

func (ctx *ServerlessWorkflowTask) RemoveLocalExprVars(keys ...string) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	if ctx.localExprVars == nil {
		return
	}

	for _, k := range keys {
		delete(ctx.localExprVars, k)
	}
}

func (ctx *ServerlessWorkflowTask) SetLocalExprVars(vars map[string]any) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.localExprVars = vars
}

// GetVars returns all available variables for expression evaluation
func (ctx *ServerlessWorkflowTask) GetVars() map[string]any {
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
func (ctx *ServerlessWorkflowTask) getWorkflowDefAsMap() map[string]any {
	if wkflw := ctx.GetWorkflowDef(); wkflw != nil {
		if found, err := wkflw.AsMap(); err == nil {
			return found
		}
	}
	return map[string]any{}
}

func (ctx *ServerlessWorkflowTask) SetStatus(status swctx.StatusPhase) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.StatusPhase == nil {
		ctx.StatusPhase = []swctx.StatusPhaseLog{}
	}
	ctx.StatusPhase = append(ctx.StatusPhase, swctx.NewStatusPhaseLog(status))
}

// SetInstanceCtx safely sets the `$context` value
func (ctx *ServerlessWorkflowTask) SetInstanceCtx(value any) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.Context = value
}

// GetInstanceCtx safely retrieves the `$context` value
func (ctx *ServerlessWorkflowTask) GetInstanceCtx() any {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.Context == nil {
		return nil
	}
	return ctx.Context
}

// SetInput safely sets the input
func (ctx *ServerlessWorkflowTask) SetInput(input any) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.Input = input
}

// GetInput safely retrieves the input
func (ctx *ServerlessWorkflowTask) GetInput() any {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.Input
}

// SetOutput safely sets the output
func (ctx *ServerlessWorkflowTask) SetOutput(output any) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.Output = output
}

// GetOutput safely retrieves the output
func (ctx *ServerlessWorkflowTask) GetOutput() any {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.Output
}

// GetInputAsMap safely retrieves the input as a map[string]any.
// If input is not a map, it wraps it in a map with "input" as the key.
func (ctx *ServerlessWorkflowTask) GetInputAsMap() map[string]any {
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

func (ctx *ServerlessWorkflowTask) GetInputAsCloudEvent() *cloudevents.Event {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	var event cloudevents.Event
	if err := common.ConvertInterfaceToInterface(ctx.Input, &event); err != nil {
		logrus.WithError(err).Error("failed to unmarshal cloudevent from workflow input")
		return nil
	}

	if len(event.ID()) == 0 {
		logrus.Error("cloudevent validation failed: missing ID")
		return nil
	}
	if event.Time().IsZero() {
		logrus.Error("cloudevent validation failed: missing Time")
		return nil
	}
	if len(event.Source()) == 0 {
		logrus.Error("cloudevent validation failed: missing Source")
		return nil
	}
	if len(event.Type()) == 0 {
		logrus.Error("cloudevent validation failed: missing Type")
		return nil
	}

	return &event
}

func (ctx *ServerlessWorkflowTask) GetContextAsMap() map[string]any {
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
func (ctx *ServerlessWorkflowTask) GetOutputAsMap() map[string]any {
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

func (ctx *ServerlessWorkflowTask) SetTaskStatus(task string, status swctx.StatusPhase) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	if ctx.TasksStatusPhase == nil {
		ctx.TasksStatusPhase = map[string][]swctx.StatusPhaseLog{}
	}

	logrus.WithFields(logrus.Fields{
		"workflowID": ctx.WorkflowID,
		"task":       task,
		"status":     status,
	}).Info("Setting task status")

	ctx.TasksStatusPhase[task] = append(ctx.TasksStatusPhase[task], swctx.NewStatusPhaseLog(status))
}

func (ctx *ServerlessWorkflowTask) SetTaskRawInput(input any) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.state.Input = input
}

func (ctx *ServerlessWorkflowTask) SetTaskRawOutput(output any) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	ctx.state.Output = output
}

func (ctx *ServerlessWorkflowTask) SetTaskDef(def model.Task) error {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.state.Definition = def
	return nil
}

func (ctx *ServerlessWorkflowTask) SetTaskStartedAt(startedAt time.Time) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.state.StartedAt = startedAt
}

func (ctx *ServerlessWorkflowTask) SetTaskName(name string) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.state.Name = name
	ctx.Entrypoint = name
}

func (ctx *ServerlessWorkflowTask) SetTaskReference(ref string) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.state.Reference = ref
}

func (ctx *ServerlessWorkflowTask) GetTaskReference() string {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.state.Reference
}

func (ctx *ServerlessWorkflowTask) ClearTaskContext() {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.state = NewWorkflowTaskState()
}
