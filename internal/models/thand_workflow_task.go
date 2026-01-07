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
	"fmt"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
)

const (
	VarsContextUser      = "user"
	VarsContextRequest   = "request"
	VarsContextProviders = "providers"
	VarsContextWorkflow  = "workflow"
	VarsContextRole      = "role"
	VarsContextApproved  = "approved"
)

// ThandWorkflowTask represents a task within a workflow and implements TaskSupport
type ThandWorkflowTask struct {
	*sdkWorkflowsModel.ServerlessWorkflowTask
}

func NewThandWorkflowTask(serverlessWorkflow *sdkWorkflowsModel.ServerlessWorkflowTask) *ThandWorkflowTask {
	return &ThandWorkflowTask{
		ServerlessWorkflowTask: serverlessWorkflow,
	}
}

func (r *ThandWorkflowTask) GetEncodedTask(encryptor EncryptionImpl) string {

	// Tasks may contain sensitive data so always encrypt
	return EncodingWrapper{
		Type: ENCODED_WORKFLOW_TASK,
		Data: r,
	}.EncodeAndEncrypt(encryptor)
}

func (r *ThandWorkflowTask) SetUser(user *User) {
	r.SetContextKeyValue(VarsContextUser, user.AsMap())
}

func (r *ThandWorkflowTask) SetRole(role *Role) {
	r.SetContextKeyValue(VarsContextRole, role.AsMap())
}

// Helper methods for TaskSupport
func (r *ThandWorkflowTask) SetWorkflowDef(workflow *model.Workflow) {
	r.Workflow = workflow
}

func (r *ThandWorkflowTask) SetContext(ctx any) {
	r.Context = ctx
}

func (r *ThandWorkflowTask) SetContextKeyValue(key string, value any) {
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

func (r *ThandWorkflowTask) GetAuthenticationProvider() string {

	elevationRequest, err := r.GetContextAsElevationRequest()

	if err != nil {
		logrus.Warnf("failed to get elevation request from context: %v", err)
		return ""
	}

	return elevationRequest.Authenticator

}

func (r *ThandWorkflowTask) GetTaskList() *model.TaskList {
	workflow := r.GetWorkflowDef()

	if workflow == nil {
		logrus.Warnf("workflow definition is nil")
		return nil
	}

	return workflow.Do
}

func (r *ThandWorkflowTask) GetCurrentTaskItem() (int, *model.TaskItem) {
	taskList := r.GetTaskList()
	currentState := r.GetTaskName()
	return taskList.KeyAndIndex(currentState)

}

func (r *ThandWorkflowTask) GetNextTask() (int, *model.TaskItem) {
	taskList := r.GetTaskList()
	currentIndex, _ := r.GetCurrentTaskItem()
	nextIndex, nextState := taskList.Next(currentIndex)
	return nextIndex, nextState
}

func (r *ThandWorkflowTask) GetContextAsElevationRequest() (*ElevateRequestInternal, error) {
	var req ElevateRequestInternal
	if err := common.ConvertInterfaceToInterface(r.GetInstanceCtx(), &req); err != nil {
		return nil, fmt.Errorf("failed to decode context as ElevateRequestInternal: %w", err)
	}
	return &req, nil
}

func (r *ThandWorkflowTask) GetUser() *User {

	req, err := r.GetContextAsElevationRequest()

	if req == nil || err != nil {
		return nil
	}

	return req.User

}

func (r *ThandWorkflowTask) GetRole() *Role {
	req, err := r.GetContextAsElevationRequest()

	if req == nil || err != nil {
		return nil
	}

	return req.Role
}

func (ctx *ThandWorkflowTask) IsApproved() *bool {

	if context := ctx.GetContextAsMap(); len(context) > 0 {
		if approved, ok := context[VarsContextApproved].(bool); ok {
			return &approved
		}
	}

	return nil
}
