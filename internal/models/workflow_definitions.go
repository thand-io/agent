package models

import (
	"encoding/json"
	"fmt"

	"github.com/hashicorp/go-version"
)

// WorkflowDefinitions represents the structure for workflows YAML/JSON
// Thse are are used for confiuration.
type WorkflowDefinitions struct {
	Version   *version.Version    `yaml:"version" json:"version"`
	Workflows map[string]Workflow `yaml:"workflows" json:"workflows"`
	Meta      ResponseMeta        `json:"meta"`
}

// UnmarshalJSON converts Version to string from any type and handles both
// API response format (with workflows as SearchResult array) and config file format (map)
func (h *WorkflowDefinitions) UnmarshalJSON(data []byte) error {
	// First, try to detect if this is a WorkflowsResponse (array) or WorkflowDefinitions (map)
	var detector struct {
		Workflows json.RawMessage `json:"workflows"`
	}

	if err := json.Unmarshal(data, &detector); err != nil {
		return err
	}

	// Check if workflows starts with '[' (array) or '{' (object/map)
	if len(detector.Workflows) > 0 && detector.Workflows[0] == '[' {
		// This is a WorkflowsResponse format with workflows as an array of SearchResult
		aux := &struct {
			Version   any                              `json:"version"`
			Workflows []SearchResult[WorkflowResponse] `json:"workflows"`
			Meta      ResponseMeta                     `json:"meta"`
		}{}

		if err := json.Unmarshal(data, &aux); err != nil {
			return err
		}

		parsedVersion, err := version.NewVersion(ConvertVersionToString(aux.Version))
		if err != nil {
			return err
		}
		h.Version = parsedVersion
		h.Meta = aux.Meta

		// Convert SearchResult array to map
		h.Workflows = make(map[string]Workflow)
		for _, searchResult := range aux.Workflows {
			workflowResp := searchResult.Result
			if workflowResp.Identifier != "" {
				// Create a Workflow from WorkflowResponse
				workflow := Workflow{
					Version:     workflowResp.Version,
					Identifier:  workflowResp.Identifier,
					Name:        workflowResp.Name,
					Description: workflowResp.Description,
					Enabled:     workflowResp.Enabled,
					// Note: Workflow field is not populated from response
				}
				h.Workflows[workflowResp.Identifier] = workflow
			}
		}

		return nil
	}

	// This is a WorkflowDefinitions format with workflows as a map
	aux := &struct {
		Version   any                 `json:"version"`
		Workflows map[string]Workflow `json:"workflows"`
		Meta      ResponseMeta        `json:"meta"`
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
	h.Meta = aux.Meta

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

		if err := workflow.Validate(); err != nil {
			return fmt.Errorf("validation failed for workflow '%s': %w", workflowKey, err)
		}

	}

	return nil
}
