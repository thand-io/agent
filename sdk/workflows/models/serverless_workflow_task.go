package models

// Copyright 2025 The Serverless Workflow Specification Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/impl"
	"github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"go.temporal.io/sdk/workflow"
)

const (
	VarsContextUser      = "user"
	VarsContextRequest   = "request"
	VarsContextProviders = "providers"
	VarsContextWorkflow  = "workflow"
	VarsContextRole      = "role"
	VarsContextApproved  = "approved"

	varsContext  = "$context"
	varsInput    = "$input"
	varsOutput   = "$output"
	varsWorkflow = "$workflow"
	varsRuntime  = "$runtime"
	varsTask     = "$task"

	// TODO: script during the release to update this value programmatically
	runtimeVersion = "v3.1.0"
	runtimeName    = "Thand (CNCF Serverless Workflow Specification Go SDK)"
)

// ServerlessWorkflowTask represents a task within a workflow and implements TaskSupport
type ServerlessWorkflowTask struct {
	mu              sync.Mutex
	internalContext context.Context    `json:"-"`
	state           *WorkflowTaskState `json:"-"`
	localExprVars   map[string]any     `json:"-"` // local variables for expressions

	// Core workfork/task fields
	WorkflowID   string `json:"id"`
	WorkflowName string `json:"name"`
	Signature    string `json:"signature"` // The signature of the task, used for validation

	// TaskSupport implementation fields
	// Entrypoint is the current task name to start from
	Entrypoint string          `json:"entrypoint,omitempty"` // The entrypoint of the workflow - allows for resumption
	Status     ctx.StatusPhase `json:"status,omitempty"`
	StartedAt  time.Time       `json:"started_at,omitempty"`

	// Never store the actual workflow workflow. We can just load it from the
	// workflow engine
	Workflow *model.Workflow `json:"-"` //  The workflow definition - no need to store this we can get it from the engine

	// Store the global context input/output state
	Context any `json:"context,omitempty"` // Use a map to allow serialization
	Input   any `json:"input,omitempty"`
	Output  any `json:"output,omitempty"`

	// Important?
	StatusPhase      []ctx.StatusPhaseLog            `json:"-"`
	TasksStatusPhase map[string][]ctx.StatusPhaseLog `json:"tasks,omitempty"`
}

func (r *ServerlessWorkflowTask) GetWorkflowID() string {
	return r.WorkflowID
}

func (r *ServerlessWorkflowTask) GetName() string {
	return r.WorkflowName
}

func (r *ServerlessWorkflowTask) HasStatus() bool {
	return len(r.Status) > 0
}

func (r *ServerlessWorkflowTask) GetWorkflowDef() *model.Workflow {
	return r.Workflow
}

func (r *ServerlessWorkflowTask) SetWorkflowInstanceCtx(value any) {
	r.SetContext(value)
}

func (r *ServerlessWorkflowTask) GetContext() context.Context {
	if r.internalContext != nil {
		return r.internalContext
	}
	return context.Background()
}

// SetInternalContext sets the internal context used by this workflow task.
// This keeps the task's context coherent when cloning runners.
func (r *ServerlessWorkflowTask) SetInternalContext(ctx context.Context) {
	r.internalContext = ctx
}

func (r *ServerlessWorkflowTask) GetTemporalContext() workflow.Context {
	if r.internalContext != nil {
		if wc, ok := r.internalContext.Value(WorkflowTaskTemporalCtxKey).(workflow.Context); ok {
			return wc
		}
	}
	return nil
}

func (r *ServerlessWorkflowTask) WithTemporalContext(ctx workflow.Context) *ServerlessWorkflowTask {

	intlCtx := r.internalContext

	if intlCtx == nil {
		intlCtx = context.Background()
	}

	r.internalContext = context.WithValue(intlCtx, WorkflowTaskTemporalCtxKey, ctx)
	return r
}

func (r *ServerlessWorkflowTask) HasTemporalContext() bool {

	if r.internalContext == nil {
		return false
	}

	return r.internalContext.Value(WorkflowTaskTemporalCtxKey) != nil
}

func (wr *ServerlessWorkflowTask) SetTaskReferenceFromName(taskName string) error {
	ref, err := impl.GenerateJSONPointer(wr.Workflow, taskName)
	if err != nil {
		return err
	}
	wr.SetTaskReference(ref)
	return nil
}

func (r *ServerlessWorkflowTask) GetTaskName() string {

	if r.state != nil && len(r.state.Name) > 0 {
		return r.state.Name
	} else if len(r.GetEntrypoint()) > 0 {
		return r.GetEntrypoint()
	} else {
		return "unknown"
	}

}

// Helper methods for TaskSupport
func (r *ServerlessWorkflowTask) SetWorkflowDef(workflow *model.Workflow) {
	r.Workflow = workflow
}

func (r *ServerlessWorkflowTask) SetContext(ctx any) {
	r.Context = ctx
}

func (r *ServerlessWorkflowTask) SetContextKeyValue(key string, value any) {
	if r.Context == nil {
		r.Context = map[string]any{}
	}
	if ctxMap, ok := r.Context.(map[string]any); ok {
		ctxMap[key] = value
	} else {
		// Not a map[string]any so can't set user
		logrus.Warnf("workflow task context is not a map, cannot set user")
		return
	}

}

func (r *ServerlessWorkflowTask) GetTaskList() *model.TaskList {
	workflow := r.GetWorkflowDef()

	if workflow == nil {
		logrus.Warnf("workflow definition is nil")
		return nil
	}

	return workflow.Do
}

func (r *ServerlessWorkflowTask) GetCurrentTaskItem() (int, *model.TaskItem) {
	taskList := r.GetTaskList()
	currentState := r.GetTaskName()
	return taskList.KeyAndIndex(currentState)

}

func (r *ServerlessWorkflowTask) GetNextTask() (int, *model.TaskItem) {
	taskList := r.GetTaskList()
	currentIndex, _ := r.GetCurrentTaskItem()
	nextIndex, nextState := taskList.Next(currentIndex)
	return nextIndex, nextState
}

func (ctx *ServerlessWorkflowTask) GetState() *WorkflowTaskState {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.state
}

func (ctx *ServerlessWorkflowTask) GetStateAsMap() map[string]any {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	if ctx.state == nil {
		return map[string]any{}
	}
	var stateMap map[string]any
	err := common.ConvertInterfaceToInterface(ctx.state, &stateMap)
	if err != nil {
		return map[string]any{}
	}
	return stateMap
}

func (ctx *ServerlessWorkflowTask) HasState() bool {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.state != nil
}

func (ctx *ServerlessWorkflowTask) SetState(state *WorkflowTaskState) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.state = state
}

func (ctx *ServerlessWorkflowTask) GetStatus() ctx.StatusPhase {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.Status
}

// Set where to resume the workflow from
func (ctx *ServerlessWorkflowTask) SetEntrypoint(entrypoint string) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.Entrypoint = entrypoint
}

// Get where to resume the workflow from
func (ctx *ServerlessWorkflowTask) GetEntrypoint() string {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.Entrypoint
}

func (ctx *ServerlessWorkflowTask) HasEntrypoint() bool {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return len(ctx.Entrypoint) > 0
}

func (ctx *ServerlessWorkflowTask) GetEntrypointIndex() (int, error) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()

	if len(ctx.Entrypoint) == 0 {
		return 0, nil
	}

	if ctx.Workflow == nil || ctx.Workflow.Do == nil {
		return 0, fmt.Errorf("workflow or task list is nil")
	}

	idx, task := ctx.GetTaskList().KeyAndIndex(ctx.Entrypoint)

	if task == nil {
		return 0, fmt.Errorf("invalid entrypoint: %s", ctx.Entrypoint)
	}

	return idx, nil

}
