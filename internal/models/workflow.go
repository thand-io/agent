package models

import (
	"encoding/json"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
)

// Validatable is an interface for tasks that support custom validation.
// Tasks implementing this interface will have their Validate method called
// during workflow validation.
type Validatable interface {
	Validate() error
}

type Workflow struct {
	Version     *version.Version `json:"version,omitempty"`
	Identifier  string           `json:"-"`
	Name        string           `json:"name" validate:"required,min=1,max=100"`
	Description string           `json:"description" validate:"max=500"`
	Workflow    *model.Workflow  `json:"workflow,omitempty" validate:"required"`
	Enabled     bool             `json:"enabled" default:"true"` // By default enable the workflow
}

func NewWorkflow(version *version.Version, identifier string, name string, description string, workflow *model.Workflow) *Workflow {
	return &Workflow{
		Version:     version,
		Identifier:  identifier,
		Name:        name,
		Description: description,
		Workflow:    workflow,
		Enabled:     true,
	}
}

func (w *Workflow) GetVersion() *version.Version {
	return w.Version
}

func (r *Workflow) HasPermission(user *User) bool {
	return true
}

func (w *Workflow) GetIdentifier() string {
	return w.Identifier
}

func (w *Workflow) GetName() string {
	return w.Name
}

func (w *Workflow) GetDescription() string {
	return w.Description
}

func (w *Workflow) GetWorkflow() *model.Workflow {
	return w.Workflow
}

// Create a clone of the workflow to avoid mutations
func (w *Workflow) GetWorkflowClone() *model.Workflow {
	if w.Workflow == nil {
		logrus.Errorln("Failed to clone workflow. Base workflow not provided")
		return nil
	}

	// Deep copy via JSON marshaling
	data, err := json.Marshal(w.Workflow)
	if err != nil {
		logrus.WithError(err).Errorln("Failed to marshal workflow for cloning")
		return nil
	}

	clone := &model.Workflow{}
	if err := json.Unmarshal(data, clone); err != nil {
		logrus.WithError(err).Errorln("Failed to unmarshal workflow for cloning")
		return nil
	}
	return clone
}

func (w *Workflow) GetEnabled() bool {
	return w.Enabled
}

type WorkflowRequest struct {
	Task *ElevateWorkflowTask `json:"task"`
	Url  string               `json:"url"`
}

func (r *WorkflowRequest) GetTask() *ElevateWorkflowTask {
	return r.Task
}

func (r *WorkflowRequest) GetRedirectURL() string {
	return r.Url
}

type WorkflowExecutionInfo struct {
	WorkflowID string `json:"id"`
	RunID      string `json:"run"`

	StartTime time.Time  `json:"started_at"`
	CloseTime *time.Time `json:"finished_at"`

	Status string `json:"status"`
	Task   string `json:"task,omitempty"`

	History []string `json:"history,omitempty"` // History of state transitions

	// SearchAttributes are the custom search attributes associated with the workflow
	Workflow string `json:"name"` // workflowName
	Role     string `json:"role"`
	User     string `json:"user"`
	Reason   string `json:"reason,omitempty"`
	Duration int64  `json:"duration,omitempty"` // Duration in seconds
	Approved *bool  `json:"approved"`           // nil = pending approval, true = approved, false = denied

	Providers  []string    `json:"providers,omitempty"`
	Identities []*Identity `json:"identities,omitempty"`

	// Context
	Input   any `json:"input,omitempty"`
	Output  any `json:"output,omitempty"`
	Context any `json:"context,omitempty"`
}

// TaskHandler defines the signature for task execution functions
type TaskHandler func(
	workflowTask *ElevateWorkflowTask,
	task *model.TaskItem,
	input any,
) (any, error)

func (w *WorkflowExecutionInfo) GetAuthorizationTime() *time.Time {

	if w.Approved == nil {
		return nil
	}

	if !*w.Approved {
		return nil
	}

	approvalTime := time.Now()

	// Find the authorization time in the context
	if w.Context == nil {
		return &approvalTime
	}

	contextMap, ok := w.Context.(map[string]any)
	if !ok {
		return &approvalTime
	}

	if authTimeRaw, exists := contextMap["authorized_at"]; exists {
		if authTimeStr, ok := authTimeRaw.(string); ok {
			parsedTime, err := time.Parse(time.RFC3339, authTimeStr)
			if err == nil {
				return &parsedTime
			}
		}
	}

	return &approvalTime
}
