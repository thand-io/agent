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

package models

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

type ctxKey string

const (
	VarsContextUser     = "user"
	VarsContextRequest  = "request"
	VarsContextProvider = "provider"
	VarsContextWorkflow = "workflow"
	VarsContextRole     = "role"
	VarsContextApproved = "approved"

	runnerCtxKey   ctxKey = "wfRunnerContext"
	temporalCtxKey ctxKey = "wfTemporalContext"

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

// WorkflowTask represents a task within a workflow and implements TaskSupport
type WorkflowTask struct {
	mu              sync.Mutex
	internalContext context.Context    `json:"-"`
	state           *WorkflowTaskState `json:"-"`
	localExprVars   map[string]any     `json:"-"` // local variables for expressions

	// Core workfork/task fields
	WorkflowID             string `json:"id"`
	WorkflowName           string `json:"name"`
	AuthenticationProvider string `json:"authentication"` // The authorisor used for this task
	Signature              string `json:"signature"`      // The signature of the task, used for validation

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

type WorkflowTaskState struct {
	Definition model.Task `json:"definition"`
	StartedAt  time.Time  `json:"started_at,omitempty"`
	Name       string     `json:"name"`
	Reference  string     `json:"reference"`
	Input      any        `json:"input"`
	Output     any        `json:"output"`
}

func NewWorkflowTaskState() *WorkflowTaskState {
	return &WorkflowTaskState{
		Input:  map[string]any{},
		Output: map[string]any{},
	}
}

func (r *WorkflowTask) GetEncodedTask(encryptor EncryptionImpl) string {

	// Tasks may contain sensitive data so always encrypt
	return EncodingWrapper{
		Type: ENCODED_WORKFLOW_TASK,
		Data: r,
	}.EncodeAndEncrypt(encryptor)
}

func (r *WorkflowTask) HasStatus() bool {
	return len(r.Status) > 0
}

func (r *WorkflowTask) SetAuthentication(auth string) {
	r.AuthenticationProvider = auth
}

func (r *WorkflowTask) GetWorkflowDef() *model.Workflow {
	return r.Workflow
}

func (r *WorkflowTask) SetWorkflowInstanceCtx(value any) {
	r.SetContext(value)
}

func (r *WorkflowTask) GetContext() context.Context {
	if r.internalContext != nil {
		return r.internalContext
	}
	return context.Background()
}

// WithInternalContext sets the internal context used by this workflow task.
// This keeps the task's context coherent when cloning runners.
func (r *WorkflowTask) WithInternalContext(ctx context.Context) {
	r.internalContext = ctx
}

func (r *WorkflowTask) GetTemporalContext() workflow.Context {
	if r.internalContext != nil {
		if wc, ok := r.internalContext.Value(temporalCtxKey).(workflow.Context); ok {
			return wc
		}
	}
	return nil
}

func (r *WorkflowTask) WithTemporalContext(ctx workflow.Context) *WorkflowTask {

	intlCtx := r.internalContext

	if intlCtx == nil {
		intlCtx = context.Background()
	}

	r.internalContext = context.WithValue(intlCtx, temporalCtxKey, ctx)
	return r
}

func (r *WorkflowTask) HasTemporalContext() bool {

	if r.internalContext == nil {
		return false
	}

	return r.internalContext.Value(temporalCtxKey) != nil
}

func (wr *WorkflowTask) SetTaskReferenceFromName(taskName string) error {
	ref, err := impl.GenerateJSONPointer(wr.Workflow, taskName)
	if err != nil {
		return err
	}
	wr.SetTaskReference(ref)
	return nil
}

func (r *WorkflowTask) GetTaskName() string {

	if r.state == nil {
		if len(r.GetEntrypoint()) > 0 {
			return r.GetEntrypoint()
		} else {
			return "unknown"
		}
	}

	return r.state.Name
}

func (r *WorkflowTask) SetUser(user *User) {
	r.SetContextKeyValue(VarsContextUser, user.AsMap())
}

// Helper methods for TaskSupport
func (r *WorkflowTask) SetWorkflowDsl(workflow *model.Workflow) {
	r.Workflow = workflow
}

func (r *WorkflowTask) SetContext(ctx any) {
	r.Context = ctx
}

func (r *WorkflowTask) SetContextKeyValue(key string, value any) {
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

func (r *WorkflowTask) GetAuthenticationProvider() string {
	return r.AuthenticationProvider
}

func (r *WorkflowTask) GetTaskList() *model.TaskList {
	workflow := r.GetWorkflowDef()

	if workflow == nil {
		logrus.Warnf("workflow definition is nil")
		return nil
	}

	return workflow.Do
}

func (r *WorkflowTask) GetCurrentTaskItem() (int, *model.TaskItem) {
	taskList := r.GetTaskList()
	currentState := r.GetTaskName()
	return taskList.KeyAndIndex(currentState)

}

func (r *WorkflowTask) GetNextTask() (int, *model.TaskItem) {
	taskList := r.GetTaskList()
	currentIndex, _ := r.GetCurrentTaskItem()
	nextIndex, nextState := taskList.Next(currentIndex)
	return nextIndex, nextState
}

func (r *WorkflowTask) GetContextAsElevationRequest() (*ElevateRequestInternal, error) {
	var req ElevateRequestInternal
	if err := common.ConvertInterfaceToInterface(r.GetInstanceCtx(), &req); err != nil {
		return nil, fmt.Errorf("failed to decode context as ElevateRequestInternal: %w", err)
	}
	return &req, nil
}

func (ctx *WorkflowTask) GetState() *WorkflowTaskState {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.state
}

func (ctx *WorkflowTask) GetStateAsMap() map[string]any {
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

func (ctx *WorkflowTask) HasState() bool {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.state != nil
}

func (ctx *WorkflowTask) SetState(state *WorkflowTaskState) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.state = state
}

func (ctx *WorkflowTask) GetStatus() ctx.StatusPhase {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.Status
}

// Set where to resume the workflow from
func (ctx *WorkflowTask) SetEntrypoint(entrypoint string) {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	ctx.Entrypoint = entrypoint
}

// Get where to resume the workflow from
func (ctx *WorkflowTask) GetEntrypoint() string {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return ctx.Entrypoint
}

func (ctx *WorkflowTask) HasEntrypoint() bool {
	ctx.mu.Lock()
	defer ctx.mu.Unlock()
	return len(ctx.Entrypoint) > 0
}

func (ctx *WorkflowTask) GetEntrypointIndex() (int, error) {
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

func (ctx *WorkflowTask) IsApproved() bool {

	if context := ctx.GetContextAsMap(); len(context) > 0 {
		if approved, ok := context[VarsContextApproved].(bool); ok {
			return approved
		}
	}

	if input := ctx.GetInputAsMap(); len(input) > 0 {
		if approved, ok := input[VarsContextApproved].(bool); ok {
			return approved
		}
	}

	if output := ctx.GetOutputAsMap(); len(output) > 0 {
		if approved, ok := output[VarsContextApproved].(bool); ok {
			return approved
		}
	}

	return false
}
