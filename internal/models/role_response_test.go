package models

import (
	"encoding/json"
	"testing"

	"github.com/hashicorp/go-version"
)

func TestRoleResponse_MarshalJSON(t *testing.T) {
	// Test that RoleResponse only marshals identifier, name, and description
	roleResp := RoleResponse{
		Identifier:  "admin",
		Name:        "Administrator",
		Description: "Full administrative access",
		Providers:   []string{"aws", "gcp"},
		Enabled:     true,
		Permissions: RolePermissions{
			Allow: []Statement{},
		},
	}

	jsonData, err := json.Marshal(roleResp)
	if err != nil {
		t.Fatalf("Failed to marshal RoleResponse: %v", err)
	}

	// Parse back to verify only expected fields are present
	var result map[string]any
	err = json.Unmarshal(jsonData, &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result["identifier"] != "admin" {
		t.Errorf("Expected identifier 'admin', got '%v'", result["identifier"])
	}
	if result["name"] != "Administrator" {
		t.Errorf("Expected name 'Administrator', got '%v'", result["name"])
	}
	if result["description"] != "Full administrative access" {
		t.Errorf("Expected description 'Full administrative access', got '%v'", result["description"])
	}

}

func TestRolesResponse_Marshal(t *testing.T) {
	// Test that a full RolesResponse with RoleResponse objects
	// only marshals the simplified fields
	response := RolesResponse{
		Version: version.Must(version.NewVersion("1.0.0")),
		Roles: []SearchResult[RoleResponse]{
			{
				ID:    "admin",
				Score: 1.0,
				Result: RoleResponse{
					Identifier:  "admin",
					Name:        "Administrator",
					Description: "Full administrative access",
					Providers:   []string{"aws", "gcp"},
					Enabled:     true,
				},
			},
			{
				ID:    "developer",
				Score: 1.0,
				Result: RoleResponse{
					Identifier:  "developer",
					Name:        "Developer",
					Description: "Development access",
					Providers:   []string{"aws"},
					Enabled:     true,
				},
			},
		},
		Meta: ResponseMeta{},
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal RolesResponse: %v", err)
	}

	// Parse back and verify structure
	var result map[string]any
	err = json.Unmarshal(jsonData, &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	// Verify roles array exists
	roles, ok := result["roles"].([]any)
	if !ok {
		t.Fatal("roles should be an array")
	}
	if len(roles) != 2 {
		t.Errorf("Expected 2 roles, got %d", len(roles))
	}

	// Check first role in _source
	firstRole := roles[0].(map[string]any)
	source1 := firstRole["_source"].(map[string]any)
	if source1["identifier"] != "admin" {
		t.Errorf("Expected first role identifier 'admin', got '%v'", source1["identifier"])
	}
	if source1["name"] != "Administrator" {
		t.Errorf("Expected name 'Administrator', got '%v'", source1["name"])
	}
	if source1["description"] != "Full administrative access" {
		t.Errorf("Expected description 'Full administrative access', got '%v'", source1["description"])
	}

	// Check second role
	secondRole := roles[1].(map[string]any)
	source2 := secondRole["_source"].(map[string]any)
	if source2["identifier"] != "developer" {
		t.Errorf("Expected second role identifier 'developer', got '%v'", source2["identifier"])
	}
}

func TestRoleResponse_EmptyFields(t *testing.T) {
	// Test RoleResponse with empty description
	roleResp := RoleResponse{
		Identifier:  "basic",
		Name:        "Basic Role",
		Description: "",
	}

	jsonData, err := json.Marshal(roleResp)
	if err != nil {
		t.Fatalf("Failed to marshal RoleResponse: %v", err)
	}

	var result map[string]any
	err = json.Unmarshal(jsonData, &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if result["identifier"] != "basic" {
		t.Errorf("Expected identifier 'basic', got '%v'", result["identifier"])
	}
	if result["name"] != "Basic Role" {
		t.Errorf("Expected name 'Basic Role', got '%v'", result["name"])
	}
	if result["description"] != "" {
		t.Errorf("Expected empty description, got '%v'", result["description"])
	}
}

func TestRoleResponse_MultipleRoles(t *testing.T) {
	// Test multiple RoleResponse objects in a list
	roles := []RoleResponse{
		{
			Identifier:  "admin",
			Name:        "Administrator",
			Description: "Full admin access",
		},
		{
			Identifier:  "viewer",
			Name:        "Viewer",
			Description: "Read-only access",
		},
	}

	jsonData, err := json.Marshal(roles)
	if err != nil {
		t.Fatalf("Failed to marshal role list: %v", err)
	}

	var result []map[string]any
	err = json.Unmarshal(jsonData, &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	if len(result) != 2 {
		t.Errorf("Expected 2 roles, got %d", len(result))
	}

	// Check first role
	if result[0]["identifier"] != "admin" {
		t.Errorf("Expected identifier 'admin', got '%v'", result[0]["identifier"])
	}

	// Check second role
	if result[1]["identifier"] != "viewer" {
		t.Errorf("Expected identifier 'viewer', got '%v'", result[1]["identifier"])
	}
}

func TestRolesResponse_EmptyRoles(t *testing.T) {
	// Test that empty roles response works correctly
	response := RolesResponse{
		Version: version.Must(version.NewVersion("1.0.0")),
		Roles:   []SearchResult[RoleResponse]{},
		Meta:    ResponseMeta{},
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal empty response: %v", err)
	}

	var result map[string]any
	err = json.Unmarshal(jsonData, &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	roles, ok := result["roles"].([]any)
	if !ok {
		t.Fatal("roles should be an array")
	}
	if len(roles) != 0 {
		t.Errorf("Expected 0 roles, got %d", len(roles))
	}
}

func TestRolesResponse_WithSearchResult(t *testing.T) {
	// Test that RolesResponse properly wraps RoleResponse in SearchResult
	response := RolesResponse{
		Version: version.Must(version.NewVersion("1.0.0")),
		Roles: []SearchResult[RoleResponse]{
			{
				ID:    "aws_user",
				Score: 0.95,
				Result: RoleResponse{
					Identifier:  "aws_user",
					Name:        "User",
					Description: "Basic access to user resources.",
					Providers:   []string{"aws-thand-dev", "aws"},
					Enabled:     true,
				},
			},
		},
		Meta: ResponseMeta{Page: 0, PageSize: 10, Total: 1, TotalPages: 1},
	}

	jsonData, err := json.Marshal(response)
	if err != nil {
		t.Fatalf("Failed to marshal response: %v", err)
	}

	var result map[string]any
	err = json.Unmarshal(jsonData, &result)
	if err != nil {
		t.Fatalf("Failed to unmarshal result: %v", err)
	}

	roles := result["roles"].([]any)
	if len(roles) != 1 {
		t.Errorf("Expected 1 role, got %d", len(roles))
	}

	roleData := roles[0].(map[string]any)
	if roleData["_id"] != "aws_user" {
		t.Errorf("Expected _id 'aws_user', got '%v'", roleData["_id"])
	}
	if roleData["_score"] != 0.95 {
		t.Errorf("Expected _score 0.95, got '%v'", roleData["_score"])
	}

	source := roleData["_source"].(map[string]any)
	if source["identifier"] != "aws_user" {
		t.Errorf("Expected identifier 'aws_user', got '%v'", source["identifier"])
	}
	if source["name"] != "User" {
		t.Errorf("Expected name 'User', got '%v'", source["name"])
	}
	if source["description"] != "Basic access to user resources." {
		t.Errorf("Expected correct description, got '%v'", source["description"])
	}
}
