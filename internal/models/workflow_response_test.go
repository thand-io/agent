package models

import (
	"encoding/json"
	"testing"
)

func TestWorkflowsResponse_UnmarshalJSON_OldFormat(t *testing.T) {
	// Old map-based format
	oldFormatJSON := `{
		"version": "1.0",
		"workflows": {
			"approval-workflow": {
				"id": "approval-workflow",
				"name": "Approval Workflow",
				"description": "Standard approval process",
				"enabled": true
			},
			"auto-approve": {
				"id": "auto-approve",
				"name": "Auto Approve",
				"description": "Automatic approval",
				"enabled": true
			}
		}
	}`

	var response WorkflowsResponse
	err := json.Unmarshal([]byte(oldFormatJSON), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal old format: %v", err)
	}

	// Verify we have 2 workflows
	if len(response.Workflows) != 2 {
		t.Errorf("Expected 2 workflows, got %d", len(response.Workflows))
	}

	// Verify the workflows were correctly converted to SearchResult array
	workflowMap := make(map[string]WorkflowResponse)
	for _, sr := range response.Workflows {
		workflowMap[sr.Result.ID] = sr.Result
	}

	// Check approval-workflow
	approval, exists := workflowMap["approval-workflow"]
	if !exists {
		t.Error("Expected approval-workflow to exist")
	}
	if approval.Name != "Approval Workflow" {
		t.Errorf("Expected name 'Approval Workflow', got '%s'", approval.Name)
	}
	if !approval.Enabled {
		t.Error("Expected workflow to be enabled")
	}

	// Check auto-approve
	autoApprove, exists := workflowMap["auto-approve"]
	if !exists {
		t.Error("Expected auto-approve workflow to exist")
	}
	if autoApprove.Name != "Auto Approve" {
		t.Errorf("Expected name 'Auto Approve', got '%s'", autoApprove.Name)
	}
}

func TestWorkflowsResponse_UnmarshalJSON_NewFormat(t *testing.T) {
	// New SearchResult array format
	newFormatJSON := `{
		"version": "1.0",
		"workflows": [
			{
				"_id": "approval-workflow",
				"_score": 1.0,
				"_source": {
					"id": "approval-workflow",
					"name": "Approval Workflow",
					"description": "Standard approval process",
					"enabled": true
				}
			},
			{
				"_id": "auto-approve",
				"_score": 1.0,
				"_source": {
					"id": "auto-approve",
					"name": "Auto Approve",
					"description": "Automatic approval",
					"enabled": true
				}
			}
		],
		"meta": {}
	}`

	var response WorkflowsResponse
	err := json.Unmarshal([]byte(newFormatJSON), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal new format: %v", err)
	}

	// Verify we have 2 workflows
	if len(response.Workflows) != 2 {
		t.Errorf("Expected 2 workflows, got %d", len(response.Workflows))
	}

	// Check first workflow
	if response.Workflows[0].Result.ID != "approval-workflow" {
		t.Errorf("Expected first workflow ID 'approval-workflow', got '%s'", response.Workflows[0].Result.ID)
	}
	if response.Workflows[0].Result.Name != "Approval Workflow" {
		t.Errorf("Expected name 'Approval Workflow', got '%s'", response.Workflows[0].Result.Name)
	}

	// Check second workflow
	if response.Workflows[1].Result.ID != "auto-approve" {
		t.Errorf("Expected second workflow ID 'auto-approve', got '%s'", response.Workflows[1].Result.ID)
	}
}

func TestWorkflowResponse_UnmarshalJSON_OldFormat(t *testing.T) {
	// Old map-based format (config file format)
	oldFormatJSON := `{
		"version": "1.0.0",
		"workflows": {
			"approval-workflow": {
				"name": "Approval Workflow",
				"description": "Standard approval process",
				"enabled": true
			}
		}
	}`

	var defs WorkflowsResponse
	err := json.Unmarshal([]byte(oldFormatJSON), &defs)
	if err != nil {
		t.Fatalf("Failed to unmarshal old format: %v", err)
	}

	// Verify version
	if defs.Version == nil {
		t.Error("Expected version to be set")
	} else if defs.Version.String() != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", defs.Version.String())
	}

	// Verify workflows
	if len(defs.Workflows) != 1 {
		t.Errorf("Expected 1 workflow, got %d", len(defs.Workflows))
	}

	var approval *WorkflowResponse
	for _, sr := range defs.Workflows {
		if sr.Result.ID == "approval-workflow" {
			approval = &sr.Result
			break
		}
	}
	if approval == nil {
		t.Fatal("Expected approval-workflow to exist")
	}
	if approval.Name != "Approval Workflow" {
		t.Errorf("Expected name 'Approval Workflow', got '%s'", approval.Name)
	}
}

func TestWorkflowDefinitions_UnmarshalJSON_NewFormat(t *testing.T) {
	// New SearchResult array format (API response format)
	newFormatJSON := `{
		"version": "1.0.0",
		"workflows": [
			{
				"_id": "approval-workflow",
				"_source": {
					"id": "approval-workflow",
					"name": "Approval Workflow",
					"description": "Standard approval process",
					"enabled": true
				}
			}
		]
	}`

	var defs WorkflowsResponse
	err := json.Unmarshal([]byte(newFormatJSON), &defs)
	if err != nil {
		t.Fatalf("Failed to unmarshal new format: %v", err)
	}

	// Verify version
	if defs.Version == nil {
		t.Error("Expected version to be set")
	} else if defs.Version.String() != "1.0.0" {
		t.Errorf("Expected version '1.0.0', got '%s'", defs.Version.String())
	}

	// Verify workflows were converted to map
	if len(defs.Workflows) != 1 {
		t.Errorf("Expected 1 workflow, got %d", len(defs.Workflows))
	}

	var approval *WorkflowResponse
	for _, sr := range defs.Workflows {
		if sr.Result.ID == "approval-workflow" {
			approval = &sr.Result
			break
		}
	}
	if approval == nil {
		t.Fatal("Expected approval-workflow to exist")
	}
	if approval.Name != "Approval Workflow" {
		t.Errorf("Expected name 'Approval Workflow', got '%s'", approval.Name)
	}
}

func TestWorkflowsResponse_UnmarshalJSON_EmptyWorkflows(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{
			name: "Empty array",
			json: `{"version": "1.0", "workflows": [], "meta": {}}`,
		},
		{
			name: "Empty object",
			json: `{"version": "1.0", "workflows": {}, "meta": {}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response WorkflowsResponse
			err := json.Unmarshal([]byte(tt.json), &response)
			if err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if len(response.Workflows) != 0 {
				t.Errorf("Expected 0 workflows, got %d", len(response.Workflows))
			}
		})
	}
}

func TestWorkflowsResponse_UnmarshalJSON_RealAPIResponse(t *testing.T) {
	// This is the exact structure from the actual API error log
	realAPIJSON := `{
		"version": "1.0",
		"workflows": [{
			"_source": {
				"id": "account_management",
				"name": "account_management",
				"description": "Account management workflow with multiple form options",
				"enabled": true
			}
		}, {
			"_source": {
				"id": "aws_email_approval",
				"name": "aws_email_approval",
				"description": "AWS access elevation with email approval via SES",
				"enabled": true
			}
		}],
		"meta": {"page": 0, "page_size": 0, "total": 0, "total_pages": 0}
	}`

	var response WorkflowsResponse
	err := json.Unmarshal([]byte(realAPIJSON), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal real API response: %v", err)
	}

	if len(response.Workflows) != 2 {
		t.Errorf("Expected 2 workflows, got %d", len(response.Workflows))
	}

	workflow1 := response.Workflows[0].Result
	if workflow1.ID != "account_management" {
		t.Errorf("Expected ID 'account_management', got '%s'", workflow1.ID)
	}
	if workflow1.Name != "account_management" {
		t.Errorf("Expected name 'account_management', got '%s'", workflow1.Name)
	}
	if !workflow1.Enabled {
		t.Error("Expected workflow to be enabled")
	}
}
