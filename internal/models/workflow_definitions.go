package models

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-version"
	"github.com/serverlessworkflow/sdk-go/v3/model"
)

// WorkflowDefinitions represents the structure for workflows YAML/JSON
// Thse are are used for confiuration.
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
