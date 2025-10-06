package runner

import (
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/workflow"
)

func (r *ResumableWorkflowRunner) executeHttpFunction(
	taskName string,
	call *model.CallHTTP,
	input any,
) (any, error) {

	logrus.WithFields(logrus.Fields{
		"task": taskName,
		"call": call.Call,
	}).Info("Executing HTTP function call")

	// Execute the function call
	workflowTask := r.GetWorkflowTask()

	if workflowTask == nil {
		return nil, fmt.Errorf("workflow task is not set")
	}

	httpCall := call.With
	uri := httpCall.Endpoint

	// Get the final URL to use for the request
	finalURL := uri.String()

	if uri.URITemplate != nil && uri.URITemplate.IsURITemplate() {

		logrus.WithFields(logrus.Fields{
			"template": uri.String(),
		}).Debug("Expanding URI template")

		// Expand URI template with variables from input
		expandedURI, err := expandURITemplate(uri.String(), input)
		if err != nil {
			logrus.WithFields(logrus.Fields{
				"template": uri.String(),
				"error":    err,
			}).Warnln("Failed to expand URI template")
			// Continue with original URI if expansion fails
		} else {
			finalURL = expandedURI
		}

	} else if uri.RuntimeExpression != nil {
		// Note: Runtime expressions are not supported in URI templates

		newUrl, err := workflowTask.TraverseAndEvaluate(uri.RuntimeExpression.String(), input)

		if err != nil {

			logrus.WithFields(logrus.Fields{
				"expression": uri.RuntimeExpression.String(),
			}).WithError(err).Error("Failed to evaluate runtime expression in URI")

			return nil, err

		} else if newUrl == nil {

			logrus.WithFields(logrus.Fields{
				"expression": uri.RuntimeExpression.String(),
			}).Warn("Runtime expression in URI evaluated to nil, using empty string")

			return nil, fmt.Errorf("runtime expression in URI evaluated to nil, using empty string")

		}

		if strUrl, ok := newUrl.(string); ok {
			finalURL = strUrl
		}

	}

	if workflowTask.HasTemporalContext() {

		// Execute the HTTP request within a Temporal activity
		fut := workflow.ExecuteActivity(
			workflowTask.GetTemporalContext(),
			models.TemporalHttpActivityName,
			httpCall,
			finalURL,
		)

		var result any
		err := fut.Get(workflowTask.GetTemporalContext(), &result)

		return result, err

		// Get the result based on the output format

	} else {

		return MakeHttpRequest(httpCall, finalURL)

	}

}

func MakeHttpRequest(httpCall model.HTTPArguments, finalURL string) (any, error) {

	builder, err := common.CreateRequestBuilderFromEndpoint(&httpCall)

	if err != nil {
		return nil, fmt.Errorf("failed to execute HTTP call for %s: %w", httpCall, err)
	}

	res, err := common.MakeRequestFromBuilder(builder, httpCall.Method, finalURL)

	if err != nil {
		return nil, fmt.Errorf("failed to make HTTP request to %s: %w", finalURL, err)
	}

	// Get the result based on the output format
	result := getHttpResponseFromResult(httpCall, res)

	// Wrap the result in a map for consistency with the expected return type
	return result, nil

}

/*
	The http call's output format.
		Supported values are:
		- raw, which output's the base-64 encoded http response content, if any.
		- content, which outputs the content of http response, possibly deserialized.
		- response, which outputs the http response.
		Defaults to content.
*/

func getHttpResponseFromResult(httpCall model.HTTPArguments, res *resty.Response) any {
	/*
		HTTP Response structure according to specification:
		request:
		  method: get
		  uri: https://petstore.swagger.io/v2/pet/1
		  headers:
		    Content-Type: application/json
		headers:
		  Content-Type: application/json
		statusCode: 200
		content:
		  id: 1
		  name: milou
		  status: pending
	*/

	// Convert headers from http.Header to map[string]string
	headers := make(map[string]string)
	for key, values := range res.Header() {
		if len(values) > 0 {
			headers[key] = values[0] // Use first value if multiple
		}
	}

	// Handle output format
	outputFormat := strings.ToLower(httpCall.Output)
	if len(outputFormat) == 0 {
		outputFormat = "content" // Default to content
	}

	switch outputFormat {
	case "raw":
		// Return base64 encoded content only
		return base64.StdEncoding.EncodeToString(res.Body())

	case "content":
		// Return just the parsed content
		content := getContent(res)

		if content != nil {
			return content
		}

		logrus.WithFields(logrus.Fields{
			"statusCode": res.StatusCode(),
			"headers":    headers,
			"url":        httpCall.Endpoint.String(),
		}).Debug("HTTP response has no content")

		return nil

	case "response":
		fallthrough
	default:
		// Return the complete HTTP response structure
		request := map[string]any{
			"method": strings.ToLower(httpCall.Method),
			"uri":    httpCall.Endpoint.String(),
		}

		// Add request headers if they exist
		if len(httpCall.Headers) > 0 {
			request["headers"] = httpCall.Headers
		}

		// Build the complete HTTP response structure
		response := map[string]any{
			"request":    request,
			"statusCode": res.StatusCode(),
			"headers":    headers,
		}

		content := getContent(res)

		if content != nil {
			response["content"] = content
		} else {
			logrus.WithFields(logrus.Fields{
				"statusCode": res.StatusCode(),
				"headers":    headers,
				"url":        httpCall.Endpoint.String(),
			}).Debug("HTTP response has no content")
		}

		return response

	}
}

func getContent(res *resty.Response) any {
	if res == nil {
		return nil
	}

	content := res.Result()

	// Only include content if it's not nil/empty
	if content != nil {

		if contentResponse, ok := content.(*any); ok && contentResponse != nil {
			return *contentResponse
		}

	}

	return nil
}

/*
The DSL has limited support for URI template syntax as defined by RFC 6570. Specifically, only the Simple String Expansion is supported, which allows authors to embed variables in a URI.

To substitute a variable within a URI, use the {} syntax. The identifier inside the curly braces will be replaced with its value during runtime evaluation. If no value is found for the identifier, an empty string will be used.

This has the following limitations compared to runtime expressions:

	Only top-level properties can be interpolated within strings, thus identifiers are treated verbatim. This means that {pet.id} will be replaced with the value of the "pet.id" property, not the value of the id property of the pet property.
	The referenced variable must be of type string, number, boolean, or null. If the variable is of a different type an error with type https://https://serverlessworkflow.io/spec/1.0.0/errors/expression and status 400 will be raised.
	Runtime expression arguments are not available for string substitution.
*/
func expandURITemplate(template string, input any) (string, error) {
	if input == nil {
		// If no input provided, replace all placeholders with empty strings
		return regexp.MustCompile(`\{[^}]*\}`).ReplaceAllString(template, ""), nil
	}

	// Convert input to map[string]any if possible
	var inputMap map[string]any
	switch v := input.(type) {
	case map[string]any:
		inputMap = v
	default:
		// If input is not a map, we cannot substitute variables
		return template, fmt.Errorf("input must be a map[string]any for URI template expansion, got %T", input)
	}

	// Regular expression to find {variable} patterns
	re := regexp.MustCompile(`\{([^}]+)\}`)

	// Replace each placeholder with the corresponding value from inputMap
	result := re.ReplaceAllStringFunc(template, func(match string) string {
		// Extract variable name (without the curly braces)
		varName := match[1 : len(match)-1]

		// Look up the value in the input map
		if value, exists := inputMap[varName]; exists {
			// Convert the value to string
			return fmt.Sprintf("%v", value)
		}

		// If variable not found, replace with empty string (RFC 6570 behavior)
		return ""
	})

	return result, nil
}
