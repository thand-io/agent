package llm

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"google.golang.org/genai"
)

/*
STEP 1

Evaluate the request to extract provider, request details, and duration
and expand on the request to ensure all necessary details are captured.
These details will then be used to try and find the provider. So we can
then lookup the roles, permissions and resources in step 2.
*/

var EvaluationRequestPrompt = `
**Task**: Parse and evaluate the user's access request to determine:
1. **Provider**: Which cloud provider they need access to
2. **Request Details**: What specific actions/resources they need
3. **Duration**: How long they need access

**Context**: 
- This evaluation will be used to create a temporary elevated access role
- Focus on the principle of least privilege

**Analysis Framework**:
1. **Provider Identification**: Match the request to available providers based on:
   - Explicit provider mentions (AWS, GCP, Azure, etc.)
   - Service names (EC2, S3, BigQuery, etc.)
   - Account references or environments (prod, dev, staging)

2. **Request Elaboration**: Expand the user's request by:
   - Identifying specific services needed
   - Determining required access levels (read, write, admin)
   - Inferring related resources they might need
   - Considering typical workflows for their use case

3. **Duration Assessment**: Determine appropriate access duration considering:
   - Explicit time requests
   - Nature of the task (quick fix vs development work)
   - Security best practices (shorter is better)
   - Typical task completion times

**Guidelines**:
- Be specific about resource requirements
- Consider dependencies between services
- Prefer shorter durations for security
- Provide clear rationale for decisions
- If ambiguous, ask for clarification rather than assume
`

func CreateEvaluateRequest(
	ctx context.Context,
	llm models.LargeLanguageModelImpl,
	providers map[string]models.Provider,
	workflows map[string]models.Workflow,
	reason string,
) (*ElevationRequestResponse, error) {

	if llm == nil {
		return nil, fmt.Errorf("LLM is not configured")
	}

	if len(providers) == 0 {
		return nil, fmt.Errorf("no providers configured")
	}

	if len(reason) == 0 {
		return nil, fmt.Errorf("reason is required")
	}

	// Convert provider impl to a string[]
	var providerNames []string
	var providersMap = map[string]any{}
	for name, provider := range providers {
		providerNames = append(providerNames, name)
		providersMap[name] = provider
	}

	var workflowNames []string
	var workflowsMap = map[string]any{}
	for name, workflow := range workflows {
		workflowNames = append(workflowNames, name)
		workflowsMap[name] = workflow
	}

	systemPrompt := fmt.Sprintf("%s\n\n%s",
		InitalSystemPrompt, EvaluationRequestPrompt)

	fullPrompt := fmt.Sprintf(`
Given the user access requsest provided in the 'reason' field in the %s
function call. Extrapolate the request to ensure all necessary details are captured.
`, GetUserRequest)

	logrus.WithFields(logrus.Fields{
		"providers": providerNames,
		"reason":    reason,
		"prompt":    fullPrompt,
	}).Info("Evaluating access request")

	response, err := llm.GenerateContent(
		context.Background(),
		llm.GetModelName(),
		[]*genai.Content{
			{
				Parts: []*genai.Part{
					{
						Text: fullPrompt,
					},
					{
						FunctionResponse: &genai.FunctionResponse{
							Name: GetUserRequest,
							Response: map[string]any{
								"reason": reason,
							},
						},
					},
					{
						FunctionResponse: &genai.FunctionResponse{
							Name:     GetProviderListToolName,
							Response: providersMap,
						},
					},
					{
						FunctionResponse: &genai.FunctionResponse{
							Name:     GetWorkflowListToolName,
							Response: workflowsMap,
						},
					},
				},
			},
		},
		&genai.GenerateContentConfig{
			Tools: getToolchain(providerNames),
			// Labels: getLabels(),
			SystemInstruction: &genai.Content{
				Parts: []*genai.Part{
					{
						Text: systemPrompt,
					},
				},
				Role: genai.RoleModel,
			},
			ToolConfig: &genai.ToolConfig{
				FunctionCallingConfig: &genai.FunctionCallingConfig{
					// Force the function call to be used
					Mode: genai.FunctionCallingConfigModeAny,
					AllowedFunctionNames: []string{
						EvaluateRequestToolName,
					},
				},
			},
			Temperature: &LLM_TEMPERATURE,
			Seed:        &LLM_SEED,
		},
	)

	if err != nil {
		logrus.WithError(err).Error("failed to generate evaluation content")
		return nil, err
	}

	candidates := []*ElevationRequestResponse{}

	for _, candidate := range response.Candidates {
		// Process each candidate and extract the relevant information
		// to create a role

		for _, part := range candidate.Content.Parts {
			// Process each part and extract the relevant information
			if part.FunctionCall != nil {
				// If the part has a function call, we can use it
				if part.FunctionCall.Name == EvaluateRequestToolName {
					// If the function call is "createEvaluation", we can use it
					// Extract the parameters from the function call
					parameters := part.FunctionCall.Args
					// Do something with the parameters
					evaluation, err := createElevationRequestResponseFromParams(parameters)
					if err != nil {
						logrus.WithError(err).Warn("failed to create evaluation from parameters")
						return nil, err
					} else if evaluation != nil {
						candidates = append(candidates, evaluation)
					}
				}
			}
		}
	}

	if len(candidates) > 0 {
		return candidates[0], nil
	}

	return nil, fmt.Errorf("no valid evaluation found")
}

func createElevationRequestResponseFromParams(params map[string]any) (*ElevationRequestResponse, error) {

	rationale, rationaleOk := params["rationale"].(string)

	if !rationaleOk {
		logrus.Warn("missing rationale field")
		return nil, fmt.Errorf("No valid rationale provided")
	}

	res := ElevationRequestResponse{
		BaseElevationResponse: BaseElevationResponse{
			Rationale: rationale,
			Success:   false,
		},
	}
	request, requestOk := params["request"].(string)
	provider, providerOk := params["provider"].(string)
	workflow, workflowOk := params["workflow"].(string)
	duration, durationOk := params["duration"].(string)
	success, successOk := params["success"].(bool)

	if providerOk {
		res.Provider = provider
	}

	if workflowOk {
		res.Workflow = workflow
	}

	if durationOk {
		// Lets ensure the duration is valid
		validDuration, err := common.ValidateDuration(duration)

		if err != nil {
			logrus.Warnf("invalid duration format: %s", duration)
			success = false
		} else {
			res.Duration = validDuration
		}
	}

	if requestOk {
		res.Request = request
	}

	if successOk {
		res.Success = success
	} else {
		logrus.Warn("missing success field, defaulting to false")
		success = false
	}

	return &res, nil
}
