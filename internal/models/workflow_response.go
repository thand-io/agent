package models

import (
	"encoding/json"

	"github.com/hashicorp/go-version"
)

// The response object is a simplied version of the workflow to be
// returned in a safe public API response.
type WorkflowsResponse struct {
	Version   *version.Version                 `json:"version"`
	Workflows []SearchResult[WorkflowResponse] `json:"workflows"`
	Meta      ResponseMeta                     `json:"meta"`
}

// UnmarshalJSON handles backwards compatibility for WorkflowsResponse
// It supports both the old format (workflows as map) and new format (workflows as array)
func (w *WorkflowsResponse) UnmarshalJSON(data []byte) error {
	// First try to unmarshal version and meta
	aux := &struct {
		Version any             `json:"version"`
		Meta    ResponseMeta    `json:"meta"`
		Raw     json.RawMessage `json:"workflows"`
	}{}

	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}

	// Parse version
	if aux.Version != nil {
		parsedVersion, err := version.NewVersion(ConvertVersionToString(aux.Version))
		if err != nil {
			return err
		}
		w.Version = parsedVersion
	}

	w.Meta = aux.Meta

	// Check if workflows is an array or object
	if len(aux.Raw) == 0 {
		w.Workflows = []SearchResult[WorkflowResponse]{}
		return nil
	}

	// Try to unmarshal as array (new format)
	var workflowArray []SearchResult[WorkflowResponse]
	if err := json.Unmarshal(aux.Raw, &workflowArray); err == nil {
		w.Workflows = workflowArray
		return nil
	}

	// Try to unmarshal as map (old format for backwards compatibility)
	var workflowMap map[string]WorkflowResponse
	if err := json.Unmarshal(aux.Raw, &workflowMap); err != nil {
		return err
	}

	// Convert map to array of SearchResults
	w.Workflows = make([]SearchResult[WorkflowResponse], 0, len(workflowMap))
	for id, workflow := range workflowMap {
		// Ensure the ID is set in case it's missing
		if workflow.ID == "" {
			workflow.ID = id
		}
		w.Workflows = append(w.Workflows, SearchResult[WorkflowResponse]{
			ID:     id,
			Score:  1.0,
			Result: workflow,
		})
	}

	return nil
}

type WorkflowResponse struct {
	Version     *version.Version `json:"version,omitempty"`
	ID          string           `json:"id"`
	Name        string           `json:"name"`
	Description string           `json:"description"`
	Enabled     bool             `json:"enabled"`
}
