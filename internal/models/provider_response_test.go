package models

import (
	"encoding/json"
	"testing"
)

func TestProvidersResponse_UnmarshalJSON_NewFormat(t *testing.T) {
	// New SearchResult array format
	newFormatJSON := `{
		"version": "1.0.0",
		"providers": [
			{
				"_id": "aws-prod",
				"_score": 1.0,
				"_source": {
					"id": "aws-prod",
					"name": "AWS Production",
					"description": "Production AWS environment",
					"provider": "aws",
					"capabilities": {
						"provisioning": {
							"can_provision_roles": true
						}
					},
					"enabled": true
				}
			},
			{
				"_id": "gcp-dev",
				"_score": 1.0,
				"_source": {
					"id": "gcp-dev",
					"name": "GCP Development",
					"description": "Development GCP environment",
					"provider": "gcp",
					"enabled": true
				}
			}
		],
		"meta": {}
	}`

	var response ProvidersResponse
	err := json.Unmarshal([]byte(newFormatJSON), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal new format: %v", err)
	}

	// Verify we have 2 providers
	if len(response.Providers) != 2 {
		t.Errorf("Expected 2 providers, got %d", len(response.Providers))
	}

	// Check first provider
	if response.Providers[0].Result.ID != "aws-prod" {
		t.Errorf("Expected first provider ID 'aws-prod', got '%s'", response.Providers[0].Result.ID)
	}
	if response.Providers[0].Result.Name != "AWS Production" {
		t.Errorf("Expected name 'AWS Production', got '%s'", response.Providers[0].Result.Name)
	}

	// Check second provider
	if response.Providers[1].Result.ID != "gcp-dev" {
		t.Errorf("Expected second provider ID 'gcp-dev', got '%s'", response.Providers[1].Result.ID)
	}
}

func TestProviderDefinitions_UnmarshalJSON_OldFormat(t *testing.T) {
	// Old map-based format (config file format)
	oldFormatJSON := `{
		"version": "1.0.0",
		"providers": {
			"aws-prod": {
				"name": "AWS Production",
				"description": "Production AWS environment",
				"provider": "aws",
				"enabled": true
			}
		}
	}`

	var defs ProviderDefinitions
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

	// Verify providers
	if len(defs.Providers) != 1 {
		t.Errorf("Expected 1 provider, got %d", len(defs.Providers))
	}

	awsProd, exists := defs.Providers["aws-prod"]
	if !exists {
		t.Fatal("Expected aws-prod provider to exist")
	}
	if awsProd.Name != "AWS Production" {
		t.Errorf("Expected name 'AWS Production', got '%s'", awsProd.Name)
	}
}

func TestProviderDefinitions_UnmarshalJSON_NewFormat(t *testing.T) {
	// New SearchResult array format (API response format)
	newFormatJSON := `{
		"version": "1.0.0",
		"providers": [
			{
				"_id": "aws-prod",
				"_source": {
					"id": "aws-prod",
					"name": "AWS Production",
					"description": "Production AWS environment",
					"provider": "aws",
					"enabled": true
				}
			}
		]
	}`

	var defs ProviderDefinitions
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

	// Verify providers were converted to map
	if len(defs.Providers) != 1 {
		t.Errorf("Expected 1 provider, got %d", len(defs.Providers))
	}

	awsProd, exists := defs.Providers["aws-prod"]
	if !exists {
		t.Fatal("Expected aws-prod provider to exist")
	}
	if awsProd.Name != "AWS Production" {
		t.Errorf("Expected name 'AWS Production', got '%s'", awsProd.Name)
	}
}

func TestProvidersResponse_UnmarshalJSON_EmptyProviders(t *testing.T) {
	tests := []struct {
		name string
		json string
	}{
		{
			name: "Empty array",
			json: `{"version": "1.0.0", "providers": [], "meta": {}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var response ProvidersResponse
			err := json.Unmarshal([]byte(tt.json), &response)
			if err != nil {
				t.Fatalf("Failed to unmarshal: %v", err)
			}

			if len(response.Providers) != 0 {
				t.Errorf("Expected 0 providers, got %d", len(response.Providers))
			}
		})
	}
}

func TestProvidersResponse_UnmarshalJSON_RealAPIResponse(t *testing.T) {
	// This is the exact structure from the actual API error log
	realAPIJSON := `{
		"version": null,
		"providers": [{
			"_source": {
				"id": "aws-dev",
				"name": "AWS Development",
				"description": "Development AWS environment",
				"provider": "aws",
				"capabilities": {
					"identities": {"synchronizable": true, "interval": 360, "enabled": true},
					"users": {"synchronizable": true, "interval": 360, "enabled": true},
					"groups": {"synchronizable": true, "interval": 360, "enabled": true},
					"provisioning": {"enabled": true},
					"roles": {"enabled": true},
					"permissions": {"enabled": true},
					"tenants": {"synchronizable": true, "interval": 360, "enabled": true}
				},
				"enabled": true
			}
		}],
		"meta": {"page": 0, "page_size": 0, "total": 0, "total_pages": 0}
	}`

	var response ProvidersResponse
	err := json.Unmarshal([]byte(realAPIJSON), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal real API response: %v", err)
	}

	if len(response.Providers) != 1 {
		t.Errorf("Expected 1 provider, got %d", len(response.Providers))
	}

	provider := response.Providers[0].Result
	if provider.ID != "aws-dev" {
		t.Errorf("Expected ID 'aws-dev', got '%s'", provider.ID)
	}
	if provider.Name != "AWS Development" {
		t.Errorf("Expected name 'AWS Development', got '%s'", provider.Name)
	}
	if !provider.Enabled {
		t.Error("Expected provider to be enabled")
	}
}
