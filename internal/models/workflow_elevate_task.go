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
	sdkConstants "github.com/thand-io/agent/sdk/constants"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
)

// ElevateWorkflowTask represents a task within a workflow and implements TaskSupport
type ElevateWorkflowTask struct {
	*sdkWorkflowsModel.WorkflowTask
}

func NewElevateWorkflowTask(serverlessWorkflow sdkWorkflowsModel.WorkflowTaskSupport) *ElevateWorkflowTask {
	if _, ok := serverlessWorkflow.(*sdkWorkflowsModel.WorkflowTask); ok {
		return &ElevateWorkflowTask{
			WorkflowTask: serverlessWorkflow.(*sdkWorkflowsModel.WorkflowTask),
		}
	} else if elevateTask, ok := serverlessWorkflow.(*ElevateWorkflowTask); ok {
		return elevateTask
	} else {
		panic(fmt.Sprintf("unsupported workflow task type: %T", serverlessWorkflow))
	}
}

func (r *ElevateWorkflowTask) GetWorkflowTask() *sdkWorkflowsModel.WorkflowTask {
	return r.WorkflowTask
}

func (r *ElevateWorkflowTask) GetEncodedTask(encryptor EncryptionImpl) string {

	// Tasks may contain sensitive data so always encrypt
	return EncodingWrapper{
		Type: sdkConstants.ENCODED_WORKFLOW_TASK,
		Data: r,
	}.EncodeAndEncrypt(encryptor)
}

func (r *ElevateWorkflowTask) SetUser(user *User) {
	r.SetContextKeyValue(sdkConstants.VarsContextUser, user.AsMap())
}

func (r *ElevateWorkflowTask) SetRole(role *Role) {
	r.SetContextKeyValue(sdkConstants.VarsContextRole, role.AsMap())
}

// Helper methods for TaskSupport
func (r *ElevateWorkflowTask) SetWorkflowDef(workflow *model.Workflow) {
	r.Workflow = workflow
}

func (r *ElevateWorkflowTask) SetContext(ctx any) {
	r.Context = ctx
}

func (r *ElevateWorkflowTask) SetContextKeyValue(key string, value any) {
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

func (r *ElevateWorkflowTask) GetAuthenticationProvider() string {

	elevationRequest, err := r.GetContextAsElevationRequest()

	if err != nil {
		logrus.Warnf("failed to get elevation request from context: %v", err)
		return ""
	}

	return elevationRequest.Authenticator

}

func (r *ElevateWorkflowTask) GetTaskList() *model.TaskList {
	workflow := r.GetWorkflowDef()

	if workflow == nil {
		logrus.Warnf("workflow definition is nil")
		return nil
	}

	return workflow.Do
}

func (r *ElevateWorkflowTask) GetCurrentTaskItem() (int, *model.TaskItem) {
	taskList := r.GetTaskList()
	currentState := r.GetTaskName()
	return taskList.KeyAndIndex(currentState)

}

func (r *ElevateWorkflowTask) GetNextTask() (int, *model.TaskItem) {
	taskList := r.GetTaskList()
	currentIndex, _ := r.GetCurrentTaskItem()
	nextIndex, nextState := taskList.Next(currentIndex)
	return nextIndex, nextState
}

func (r *ElevateWorkflowTask) GetContextAsElevationRequest() (*ElevateRequestInternal, error) {
	var req ElevateRequestInternal
	if err := common.ConvertInterfaceToInterface(r.GetInstanceCtx(), &req); err != nil {
		return nil, fmt.Errorf("failed to decode context as ElevateRequestInternal: %w", err)
	}
	return &req, nil
}

func (r *ElevateWorkflowTask) GetUser() *User {

	req, err := r.GetContextAsElevationRequest()

	if req == nil || err != nil {
		return nil
	}

	return req.User

}

func (r *ElevateWorkflowTask) GetRole() *Role {
	req, err := r.GetContextAsElevationRequest()

	if req == nil || err != nil {
		return nil
	}

	return req.Role
}

func (ctx *ElevateWorkflowTask) IsApproved() *bool {

	if context := ctx.GetContextAsMap(); len(context) > 0 {
		if approved, ok := context[sdkConstants.VarsContextApproved].(bool); ok {
			return &approved
		}
	}

	return nil
}
