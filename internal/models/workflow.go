package models

import (
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
)

type Workflow struct {
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Authentication string          `json:"authentication" default:"default"`
	Workflow       *model.Workflow `json:"workflow,omitempty"`
	Enabled        bool            `json:"enabled" default:"true"` // By default enable the workflow
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

func (w *Workflow) GetAuthentication() string {
	return w.Authentication
}

func (w *Workflow) GetWorkflow() *model.Workflow {
	return w.Workflow
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
	Name           string `json:"name"`
	Description    string `json:"description"`
	Authentication string `json:"authentication"`
	Enabled        bool   `json:"enabled"`
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
	WorkflowID   string     `json:"id"`
	RunID        string     `json:"run"`
	WorkflowType string     `json:"type"`
	StartTime    time.Time  `json:"started_at"`
	CloseTime    *time.Time `json:"finished_at"`
	Status       string     `json:"status"`

	// SearchAttributes are the custom search attributes associated with the workflow
	Role     string `json:"role"`
	User     string `json:"user"`
	Approved bool   `json:"approved" default:"false"`
}
