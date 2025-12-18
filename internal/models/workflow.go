package models

import (
	"encoding/json"
	"time"

	"github.com/hashicorp/go-version"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
)

type Workflow struct {
	Version     *version.Version `json:"version,omitempty"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Workflow    *model.Workflow  `json:"workflow,omitempty"`
	Enabled     bool             `json:"enabled" default:"true"` // By default enable the workflow
}

func (r *Workflow) HasPermission(user *User) bool {
	return true
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

// WorkflowsResponse represents the response for /workflows endpoint
type WorkflowsResponse struct {
	Version   string                      `json:"version"`
	Workflows map[string]WorkflowResponse `json:"workflows"`
}

type WorkflowResponse struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Enabled     bool   `json:"enabled"`
}

type WorkflowRequest struct {
	Task *WorkflowTask `json:"task"`
	Url  string        `json:"url"`
}

func (r *WorkflowRequest) GetTask() *WorkflowTask {
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
	workflowTask *WorkflowTask,
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

// WorkflowDefinitions represents the structure for workflows YAML/JSON
type WorkflowDefinitions struct {
	Version   *version.Version    `yaml:"version" json:"version"`
	Workflows map[string]Workflow `yaml:"workflows" json:"workflows"`
}

// UnmarshalJSON converts Version to string from any type
func (h *WorkflowDefinitions) UnmarshalJSON(data []byte) error {
	aux := &struct {
		Version   any                 `json:"version"`
		Workflows map[string]Workflow `json:"workflows"`
	}{
		Workflows: make(map[string]Workflow),
	}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	parsedVersion, err := version.NewVersion(ConvertVersionToString(aux.Version))
	if err != nil {
		return err
	}

	h.Version = parsedVersion
	h.Workflows = aux.Workflows

	return nil
}

// UnmarshalYAML converts Version to string from any type
func (h *WorkflowDefinitions) UnmarshalYAML(unmarshal func(any) error) error {
	aux := &struct {
		Version   any                 `yaml:"version"`
		Workflows map[string]Workflow `yaml:"workflows"`
	}{
		Workflows: make(map[string]Workflow),
	}

	if err := unmarshal(&aux); err != nil {
		return err
	}

	parsedVersion, err := version.NewVersion(ConvertVersionToString(aux.Version))
	if err != nil {
		return err
	}

	h.Version = parsedVersion
	h.Workflows = aux.Workflows

	return nil
}
