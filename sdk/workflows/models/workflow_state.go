package models

import (
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
)

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
