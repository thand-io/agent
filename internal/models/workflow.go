package models

import (
	"encoding/json"
	"fmt"
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

// Validate validates all workflows in the definition
// Note: Workflows use serverless workflow SDK with complex validation,
// so we only perform basic structural validation here
func (h *WorkflowDefinitions) Validate() error {
	// Basic validation without struct tags (workflows have complex SDK requirements)

	for workflowKey, workflow := range h.Workflows {
		if workflow.Workflow == nil {
			return fmt.Errorf("workflow '%s' is missing workflow definition", workflowKey)
		}
		if workflow.Name == "" {
			return fmt.Errorf("workflow '%s' is missing required field 'name'", workflowKey)
		}

		// Validate individual tasks within the workflow
		if workflow.Workflow.Do != nil {
			if err := validateTaskList(workflowKey, *workflow.Workflow.Do); err != nil {
				return err
			}
		}
	}

	return nil
}

// validateTaskList validates all tasks in a task list
func validateTaskList(workflowKey string, tasks model.TaskList) error {
	for _, taskItem := range tasks {
		if taskItem == nil || taskItem.Task == nil {
			continue
		}

		// Check if the task implements Validatable interface
		if validatable, ok := taskItem.Task.(Validatable); ok {
			if err := validatable.Validate(); err != nil {
				return fmt.Errorf("workflow '%s' task '%s': %w", workflowKey, taskItem.Key, err)
			}
		}

		// Recursively validate nested task lists (e.g., in DoTask, TryTask, etc.)
		if err := validateNestedTasks(workflowKey, taskItem); err != nil {
			return err
		}
	}
	return nil
}

// validateNestedTasks validates tasks nested within other tasks (e.g., do, try/catch blocks)
func validateNestedTasks(workflowKey string, taskItem *model.TaskItem) error {
	if taskItem == nil || taskItem.Task == nil {
		return nil
	}

	// Check for DoTask which contains a nested Do list
	if doTask, ok := taskItem.Task.(*model.DoTask); ok && doTask.Do != nil {
		if err := validateTaskList(workflowKey, *doTask.Do); err != nil {
			return err
		}
	}

	// Check for TryTask which contains Try and Catch blocks
	if tryTask, ok := taskItem.Task.(*model.TryTask); ok {
		if tryTask.Try != nil {
			if err := validateTaskList(workflowKey, *tryTask.Try); err != nil {
				return err
			}
		}
		if tryTask.Catch != nil && tryTask.Catch.Do != nil {
			if err := validateTaskList(workflowKey, *tryTask.Catch.Do); err != nil {
				return err
			}
		}
	}

	// Check for ForkTask which contains branches
	if forkTask, ok := taskItem.Task.(*model.ForkTask); ok && forkTask.Fork.Branches != nil {
		if err := validateTaskList(workflowKey, *forkTask.Fork.Branches); err != nil {
			return err
		}
	}

	return nil
}
