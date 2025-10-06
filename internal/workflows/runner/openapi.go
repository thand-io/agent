package runner

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
	"sync"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-resty/resty/v2"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/workflow"
)

// OpenAPIExecutor handles OpenAPI document loading and request execution
type OpenAPIExecutor struct {
	documents map[string]*openapi3.T
	mutex     sync.RWMutex
}

// NewOpenAPIExecutor creates a new OpenAPI executor
func NewOpenAPIExecutor() *OpenAPIExecutor {
	return &OpenAPIExecutor{
		documents: make(map[string]*openapi3.T),
	}
}

func (r *ResumableWorkflowRunner) executeOpenAPIFunction(
	taskName string,
	call *model.CallOpenAPI,
	input any,
) (map[string]any, error) {

	logrus.WithFields(logrus.Fields{
		"task": taskName,
		"call": call.Call,
	}).Info("Executing OpenAPI function call")

	// Extract fields from OpenAPIArguments struct
	args := call.With

	// Output format (defaults to "content")
	outputFormat := args.Output
	if len(outputFormat) == 0 {
		outputFormat = "content"
	}

	// Get the workflow task for potential Temporal execution
	workflowTask := r.GetWorkflowTask()

	if workflowTask.HasTemporalContext() {
		// Execute within Temporal activity
		fut := workflow.ExecuteActivity(
			workflowTask.GetTemporalContext(),
			models.TemporalOpenAPIActivityName,
			args,
			input,
		)

		var result map[string]any
		err := fut.Get(workflowTask.GetTemporalContext(), &result)
		return result, err
	} else {
		// Execute directly
		return MakeOpenAPIRequest(args, input)
	}
}

// MakeOpenAPIRequest executes an OpenAPI call directly
func MakeOpenAPIRequest(args model.OpenAPIArguments, input any) (map[string]any, error) {
	executor := NewOpenAPIExecutor()

	// Extract fields from args
	document := args.Document
	operationId := args.OperationID
	parameters := args.Parameters
	if parameters == nil {
		parameters = make(map[string]any)
	}
	outputFormat := args.Output
	if len(outputFormat) == 0 {
		outputFormat = "content"
	}
	authentication := args.Authentication

	// Load OpenAPI document
	documentURL, err := resolveDocumentURL(document)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve document URL: %w", err)
	}

	doc, err := executor.loadOpenAPIDocument(documentURL)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI document: %w", err)
	}

	// Find operation by operationId
	operation, path, method, err := executor.findOperationById(doc, operationId)
	if err != nil {
		return nil, fmt.Errorf("failed to find operation '%s': %w", operationId, err)
	}

	// Build HTTP arguments for the request
	httpArgs, err := buildHTTPArgumentsFromOpenAPI(
		doc, path, method, operation, parameters, input, authentication,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to build HTTP arguments: %w", err)
	}

	// Execute HTTP request using existing infrastructure
	res, err := common.InvokeHttpRequest(httpArgs)
	if err != nil {
		return nil, fmt.Errorf("failed to execute HTTP request: %w", err)
	}

	// Format output according to spec
	result := formatOpenAPIOutput(res, outputFormat)

	logrus.WithFields(logrus.Fields{
		"operation": operationId,
		"status":    res.StatusCode(),
		"output":    outputFormat,
	}).Info("OpenAPI call completed")

	return result, nil
}

func resolveDocumentURL(document *model.ExternalResource) (string, error) {
	if document == nil {
		return "", fmt.Errorf("document is required")
	}

	// Get the URI from the external resource
	if document.Endpoint != nil {
		return document.Endpoint.String(), nil
	}

	return "", fmt.Errorf("document must have an endpoint")
}

func (e *OpenAPIExecutor) loadOpenAPIDocument(documentURL string) (*openapi3.T, error) {
	e.mutex.RLock()
	if doc, exists := e.documents[documentURL]; exists {
		e.mutex.RUnlock()
		return doc, nil
	}
	e.mutex.RUnlock()

	e.mutex.Lock()
	defer e.mutex.Unlock()

	// Double-check after acquiring write lock
	if doc, exists := e.documents[documentURL]; exists {
		return doc, nil
	}

	// Load from URL
	loader := openapi3.NewLoader()
	parsedURL, err := url.Parse(documentURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse document URL %s: %w", documentURL, err)
	}
	doc, err := loader.LoadFromURI(parsedURL)
	if err != nil {
		return nil, fmt.Errorf("failed to load OpenAPI document from %s: %w", documentURL, err)
	}

	// Validate the document
	if err := doc.Validate(context.Background()); err != nil {
		return nil, fmt.Errorf("invalid OpenAPI document: %w", err)
	}

	// Cache the document
	e.documents[documentURL] = doc
	return doc, nil
}

func (e *OpenAPIExecutor) findOperationById(doc *openapi3.T, operationId string) (*openapi3.Operation, string, string, error) {
	for path, pathItem := range doc.Paths.Map() {
		for method, operation := range pathItem.Operations() {
			if operation.OperationID == operationId {
				return operation, path, strings.ToUpper(method), nil
			}
		}
	}
	return nil, "", "", fmt.Errorf("operation with id '%s' not found", operationId)
}

func buildHTTPArgumentsFromOpenAPI(
	doc *openapi3.T,
	path, method string,
	operation *openapi3.Operation,
	parameters map[string]any,
	requestBody any,
	authentication *model.ReferenceableAuthenticationPolicy,
) (*model.HTTPArguments, error) {

	// Build base URL
	baseURL := ""
	if len(doc.Servers) > 0 {
		baseURL = doc.Servers[0].URL
	}

	// Build request URL with path parameters
	requestURL := buildRequestURL(baseURL, path, parameters)

	// Parse URL for endpoint
	parsedURL, err := url.Parse(requestURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse URL: %w", err)
	}

	// Create HTTP arguments
	httpArgs := &model.HTTPArguments{
		Method:   method,
		Endpoint: model.NewEndpoint(requestURL),
		Headers:  make(map[string]string),
		Query:    make(map[string]any),
	}

	// Add query parameters
	for key, value := range parsedURL.Query() {
		if len(value) > 0 {
			httpArgs.Query[key] = value[0]
		}
	}

	// Add additional query parameters from parameters that aren't path params
	for paramName, paramValue := range parameters {
		if !strings.Contains(path, "{"+paramName+"}") {
			httpArgs.Query[paramName] = paramValue
		}
	}

	// Set request body for POST, PUT, PATCH
	if requestBody != nil && (method == "POST" || method == "PUT" || method == "PATCH") {
		bodyBytes, err := json.Marshal(requestBody)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		httpArgs.Body = json.RawMessage(bodyBytes)
		httpArgs.Headers["Content-Type"] = "application/json"
	}

	// Set default Accept header
	httpArgs.Headers["Accept"] = "application/json"

	// Handle authentication
	if authentication != nil {
		authHeaders, err := resolveAuthentication(authentication)
		if err != nil {
			return nil, fmt.Errorf("failed to resolve authentication: %w", err)
		}
		for key, value := range authHeaders {
			httpArgs.Headers[key] = value
		}
	}

	return httpArgs, nil
}

func buildRequestURL(baseURL, path string, parameters map[string]any) string {
	// Replace path parameters
	finalPath := path
	for paramName, paramValue := range parameters {
		paramStr := fmt.Sprintf("%v", paramValue)
		placeholder := "{" + paramName + "}"
		if strings.Contains(path, placeholder) {
			finalPath = strings.ReplaceAll(finalPath, placeholder, paramStr)
		}
	}

	return baseURL + finalPath
}

func resolveAuthentication(auth *model.ReferenceableAuthenticationPolicy) (map[string]string, error) {
	headers := make(map[string]string)

	if auth == nil {
		return headers, nil
	}

	// This is a simplified implementation
	// In a real implementation, you would need to handle the various authentication types
	// defined in the ReferenceableAuthenticationPolicy struct
	logrus.Warn("Authentication not fully implemented for OpenAPI calls")

	return headers, nil
}

func formatOpenAPIOutput(resp *resty.Response, outputFormat string) map[string]any {
	bodyBytes := resp.Body()

	switch outputFormat {
	case "raw":
		// Return base64 encoded response
		return map[string]any{
			"content": base64.StdEncoding.EncodeToString(bodyBytes),
		}

	case "response":
		// Return full HTTP response structure
		result := map[string]any{
			"statusCode": resp.StatusCode(),
			"headers":    resp.Header(),
		}

		if len(bodyBytes) > 0 {
			// Try to parse as JSON, fallback to string
			var bodyData any
			if err := json.Unmarshal(bodyBytes, &bodyData); err == nil {
				result["body"] = bodyData
			} else {
				result["body"] = string(bodyBytes)
			}
		}

		return result

	case "content":
		fallthrough
	default:
		// Return just the content, deserialized if possible
		if len(bodyBytes) == 0 {
			return map[string]any{}
		}

		var result any
		if err := json.Unmarshal(bodyBytes, &result); err == nil {
			// Successfully parsed as JSON
			if resultMap, ok := result.(map[string]any); ok {
				return resultMap
			}
			return map[string]any{"result": result}
		}

		// Return as string if not valid JSON
		return map[string]any{"result": string(bodyBytes)}
	}
}
