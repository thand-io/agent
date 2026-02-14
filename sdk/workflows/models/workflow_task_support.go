package models

import (
	"context"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"go.temporal.io/sdk/workflow"
)

type WorkflowTaskSupport interface {
	GetName() string
	GetWorkflowID() string

	SetStartedAt(startedAt time.Time)
	SetStatus(status ctx.StatusPhase)
	GetStatus() ctx.StatusPhase
	HasStatus() bool

	SetTaskDef(taskDef model.Task) error
	SetTaskName(name string)
	SetTaskRawInput(input any)
	SetTaskRawOutput(output any)
	SetTaskReference(reference string)
	SetTaskStartedAt(startedAt time.Time)
	SetTaskStatus(name string, status ctx.StatusPhase)
	SetState(state *WorkflowTaskState)
	HasState() bool

	GetTaskList() *model.TaskList
	GetTaskName() string
	GetTaskReference() string
	ClearTaskContext()
	GetVars() map[string]any
	SetLocalExprVars(vars map[string]any)
	RemoveLocalExprVars(keys ...string)
	GetWorkflowDef() *model.Workflow
	SetWorkflowDef(workflowDef *model.Workflow)
	SetTaskReferenceFromName(taskName string) error

	HasEntrypoint() bool
	GetEntrypointIndex() (int, error)

	GetInput() any
	SetInput(input any)
	SetRawInput(input any)
	GetInputAsMap() map[string]any

	GetOutput() any
	SetOutput(output any)
	SetRawOutput(output any)
	GetOutputAsMap() map[string]any

	GetInstanceCtx() any
	SetInstanceCtx(value any)
	SetWorkflowInstanceCtx(value any)

	GetContext() context.Context
	GetContextAsMap() map[string]any

	GetLogger() *LogBuilder

	SetContextKeyValue(key string, value any)
	WithTemporalContext(ctx workflow.Context) WorkflowTaskSupport

	TraverseAndEvaluate(node any, input any) (any, error)
	TraverseAndEvaluateBool(runtimeExpr string, input any) (bool, error)
	TraverseAndEvaluateObj(runtimeExpr *model.ObjectOrRuntimeExpr, input any, taskName string) (output any, err error)

	Clone() ctx.WorkflowContext
	SetInternalContext(ctx context.Context)

	HasTemporalContext() bool
	GetTemporalContext() workflow.Context

	GetTaskQueue() string
	SetTaskQueue(taskQueue string)

	AddLocalExprVars(vars map[string]any)
}
