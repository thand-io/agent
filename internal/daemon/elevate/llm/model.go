package llm

import "time"

var InitalSystemPrompt = `
Act as a cloud security architect specializing in IAM role creation 
and access control. Your task is to create a secure, 
principle-of-least-privilege role based on the user's evaluated
request.
`

type ElevationRequestResponse struct {
	Provider string        `json:"provider,omitempty"`
	Workflow string        `json:"workflow,omitempty"`
	Duration time.Duration `json:"duration,omitempty"`
	Request  string        `json:"request,omitempty"`
	BaseElevationResponse
}

type ElevationQueryResponse struct {
	Roles       []string `json:"roles,omitempty"`
	Permissions []string `json:"permissions,omitempty"`
	Resources   []string `json:"resources,omitempty"`
	BaseElevationResponse
}

type BaseElevationResponse struct {
	Rationale string `json:"rationale,omitempty"`
	Success   bool   `json:"success,omitempty"`
}
