package runner

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/internal/models"
)

// getTestOpenAPIDoc returns the test OpenAPI document template
func getTestOpenAPIDoc() string {
	return `{
		"openapi": "3.0.0",
		"info": {
			"title": "Test API",
			"version": "1.0.0"
		},
		"servers": [
			{
				"url": "%s"
			}
		],
		"paths": {
			"/pets/{petId}": {
				"get": {
					"operationId": "getPetById",
					"parameters": [
						{
							"name": "petId",
							"in": "path",
							"required": true,
							"schema": {
								"type": "string"
							}
						}
					],
					"responses": {
						"200": {
							"description": "Pet details",
							"content": {
								"application/json": {
									"schema": {
										"type": "object",
										"properties": {
											"id": {"type": "string"},
											"name": {"type": "string"}
										}
									}
								}
							}
						}
					}
				}
			},
			"/pets": {
				"post": {
					"operationId": "createPet",
					"requestBody": {
						"content": {
							"application/json": {
								"schema": {
									"type": "object",
									"properties": {
										"name": {"type": "string"}
									}
								}
							}
						}
					},
					"responses": {
						"201": {
							"description": "Pet created"
						}
					}
				}
			}
		}
	}`
}

// setupOpenAPIDocServer creates a test server that serves the OpenAPI document
func setupOpenAPIDocServer(openAPIDoc string) func(*httptest.Server) {
	return func(server *httptest.Server) {
		mux := server.Config.Handler.(*http.ServeMux)
		mux.HandleFunc("/openapi.json", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, openAPIDoc, "http://"+r.Host)
		})
	}
}

// setupGetPetEndpoint sets up a GET /pets/{petId} endpoint for testing
func setupGetPetEndpoint() func(*httptest.Server) {
	return func(server *httptest.Server) {
		mux := server.Config.Handler.(*http.ServeMux)
		mux.HandleFunc("/pets/123", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "GET" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"id":   "123",
				"name": "Fluffy",
			})
		})
	}
}

// setupPostPetEndpoint sets up a POST /pets endpoint for testing
func setupPostPetEndpoint() func(*httptest.Server) {
	return func(server *httptest.Server) {
		mux := server.Config.Handler.(*http.ServeMux)
		mux.HandleFunc("/pets", func(w http.ResponseWriter, r *http.Request) {
			if r.Method != "POST" {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}

			var body map[string]any
			json.NewDecoder(r.Body).Decode(&body)

			w.WriteHeader(http.StatusCreated)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{
				"id":   "new-id",
				"name": body["name"],
			})
		})
	}
}

// setupRawPetEndpoint sets up a GET /pets/{petId} endpoint that returns raw JSON
func setupRawPetEndpoint() func(*httptest.Server) {
	return func(server *httptest.Server) {
		mux := server.Config.Handler.(*http.ServeMux)
		mux.HandleFunc("/pets/123", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"id":"123","name":"Raw Pet"}`))
		})
	}
}

// createTestServer creates a test server with the given setup functions
func createTestServer(setupFuncs ...func(*httptest.Server)) *httptest.Server {
	mux := http.NewServeMux()
	server := httptest.NewServer(mux)

	for _, setup := range setupFuncs {
		setup(server)
	}

	return server
}

// validateRawOutput validates the raw output format
func validateRawOutput(t *testing.T, result map[string]any) {
	content, ok := result["content"]
	if !ok || len(content.(string)) == 0 {
		t.Errorf("Expected base64 encoded content in raw output")
	}
}

// validateResponseOutput validates the response output format
func validateResponseOutput(t *testing.T, result map[string]any, expected map[string]any) {
	statusCode, ok := result["statusCode"]
	if !ok {
		t.Errorf("Expected statusCode in response output")
		return
	}

	if statusCode != expected["statusCode"] {
		t.Errorf("Expected statusCode %v, got %v", expected["statusCode"], statusCode)
	}

	body, ok := result["body"]
	if !ok {
		return
	}

	bodyMap, ok := body.(map[string]any)
	if !ok {
		return
	}

	expectedBody := expected["body"].(map[string]any)
	for key, expectedValue := range expectedBody {
		actualValue, exists := bodyMap[key]
		if !exists || actualValue != expectedValue {
			t.Errorf("Expected body[%s] = %v, got %v", key, expectedValue, actualValue)
		}
	}
}

// validateContentOutput validates the content output format
func validateContentOutput(t *testing.T, result map[string]any, expected map[string]any) {
	for key, expectedValue := range expected {
		actualValue, ok := result[key]
		if !ok {
			t.Errorf("Expected key %s not found in result", key)
		} else if actualValue != expectedValue {
			t.Errorf("Expected %s = %v, got %v", key, expectedValue, actualValue)
		}
	}
}

// runOpenAPITest runs a single OpenAPI test case
func runOpenAPITest(t *testing.T, name string, args model.OpenAPIArguments, input any,
	serverURL string, expectedResult map[string]any, expectError bool) {

	t.Run(name, func(t *testing.T) {
		args.Document = &model.ExternalResource{
			Endpoint: model.NewEndpoint(serverURL + "/openapi.json"),
		}

		result, err := MakeOpenAPIRequest(args, input)

		if expectError {
			if err == nil {
				t.Errorf("Expected error but got none")
			}
			return
		}

		if err != nil {
			t.Errorf("Unexpected error: %v", err)
			return
		}

		switch args.Output {
		case "raw":
			validateRawOutput(t, result)
		case "response":
			validateResponseOutput(t, result, expectedResult)
		default:
			validateContentOutput(t, result, expectedResult)
		}
	})
}

func TestMakeOpenAPIRequest(t *testing.T) {
	openAPIDoc := getTestOpenAPIDoc()

	// Test GET request with path parameter
	server1 := createTestServer(
		setupOpenAPIDocServer(openAPIDoc),
		setupGetPetEndpoint(),
	)
	defer server1.Close()

	runOpenAPITest(t, "GET request with path parameter",
		model.OpenAPIArguments{
			OperationID: "getPetById",
			Parameters: map[string]any{
				"petId": "123",
			},
			Output: "content",
		},
		nil,
		server1.URL,
		map[string]any{
			"id":   "123",
			"name": "Fluffy",
		},
		false,
	)

	// Test POST request with request body
	server2 := createTestServer(
		setupOpenAPIDocServer(openAPIDoc),
		setupPostPetEndpoint(),
	)
	defer server2.Close()

	runOpenAPITest(t, "POST request with request body",
		model.OpenAPIArguments{
			OperationID: "createPet",
			Output:      "response",
		},
		map[string]any{
			"name": "New Pet",
		},
		server2.URL,
		map[string]any{
			"statusCode": 201,
			"body": map[string]any{
				"id":   "new-id",
				"name": "New Pet",
			},
		},
		false,
	)

	// Test raw output format
	server3 := createTestServer(
		setupOpenAPIDocServer(openAPIDoc),
		setupRawPetEndpoint(),
	)
	defer server3.Close()

	runOpenAPITest(t, "Raw output format",
		model.OpenAPIArguments{
			OperationID: "getPetById",
			Parameters: map[string]any{
				"petId": "123",
			},
			Output: "raw",
		},
		nil,
		server3.URL,
		map[string]any{
			"content": "eyJpZCI6IjEyMyIsIm5hbWUiOiJSYXcgUGV0In0=",
		},
		false,
	)

	// Test operation not found
	server4 := createTestServer(
		setupOpenAPIDocServer(openAPIDoc),
	)
	defer server4.Close()

	runOpenAPITest(t, "Operation not found",
		model.OpenAPIArguments{
			OperationID: "nonExistentOperation",
		},
		nil,
		server4.URL,
		nil,
		true,
	)
}

func TestResolveDocumentURL(t *testing.T) {
	tests := []struct {
		name        string
		document    *model.ExternalResource
		expected    string
		expectError bool
	}{
		{
			name: "External resource with endpoint",
			document: &model.ExternalResource{
				Endpoint: model.NewEndpoint("https://api.example.com/openapi.json"),
			},
			expected:    "https://api.example.com/openapi.json",
			expectError: false,
		},
		{
			name:        "Nil document",
			document:    nil,
			expectError: true,
		},
		{
			name:     "External resource without endpoint",
			document: &model.ExternalResource{
				// No endpoint set
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := resolveDocumentURL(tt.document)

			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestBuildRequestURL(t *testing.T) {
	tests := []struct {
		name       string
		baseURL    string
		path       string
		parameters map[string]any
		expected   string
	}{
		{
			name:     "Simple path without parameters",
			baseURL:  "https://api.example.com",
			path:     "/pets",
			expected: "https://api.example.com/pets",
		},
		{
			name:    "Path with single parameter",
			baseURL: "https://api.example.com",
			path:    "/pets/{petId}",
			parameters: map[string]any{
				"petId": "123",
			},
			expected: "https://api.example.com/pets/123",
		},
		{
			name:    "Path with multiple parameters",
			baseURL: "https://api.example.com",
			path:    "/users/{userId}/pets/{petId}",
			parameters: map[string]any{
				"userId": "456",
				"petId":  "123",
			},
			expected: "https://api.example.com/users/456/pets/123",
		},
		{
			name:    "Path with extra parameters (should be ignored for URL building)",
			baseURL: "https://api.example.com",
			path:    "/pets/{petId}",
			parameters: map[string]any{
				"petId": "123",
				"extra": "value",
			},
			expected: "https://api.example.com/pets/123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := buildRequestURL(tt.baseURL, tt.path, tt.parameters)

			if result != tt.expected {
				t.Errorf("Expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestExecuteOpenAPIFunction(t *testing.T) {
	// Create a minimal workflow runner for testing
	runner := &ResumableWorkflowRunner{}

	// Initialize the workflow task to avoid nil pointer
	runner.workflowTask = &models.WorkflowTask{}

	// Test missing operationId
	call := &model.CallOpenAPI{
		Call: "openapi",
		With: model.OpenAPIArguments{
			Document: &model.ExternalResource{
				Endpoint: model.NewEndpoint("https://example.com/openapi.json"),
			},
			// Missing OperationID
		},
	}

	_, err := runner.executeOpenAPIFunction("test-task", call, nil)
	if err == nil {
		t.Errorf("Expected error for missing operationId")
	}
}
