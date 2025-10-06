package llm

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"google.golang.org/genai"
)

/*
STEP 2

Now that we have the provider and we know the platform type, GCP, AWS
Kubernetes, Azure, etc. We can now generate the roles, permissions
and resources we think we need to complete the request.

These details will then filter down all the roles, permissions and
resources down into step 3.

*/

var QueryElevationInfoPrompt = `
**Task**: Based on the user's evaluated request, identify and return:
1. **Roles**: Existing roles that closely match the requested access
2. **Permissions**: Specific permissions needed (allow/deny)
3. **Resources**: Target resources involved

**Context**:
- Use the provider information from the evaluation to focus your search
- Aim for least privilege while ensuring functionality

**Analysis Framework**:
1. **Role Matching**:
   - Search for roles within the specified provider that align with the requested services and actions
   - Consider roles that provide a baseline of needed permissions

2. **Permission Extraction**:
   - Identify specific permissions required for the requested actions
   - Differentiate between read, write, and admin level access
   - Include any necessary supporting permissions (e.g., describe actions for context)

3. **Resource Identification**:
   - Determine which resources (projects, buckets, instances, etc.) are relevant to the request
   - Consider resource hierarchies and dependencies

**Guidelines**:
- Be specific about roles and permissions
- Prefer existing roles over creating new ones
- Ensure permissions align with the principle of least privilege
- If uncertain, provide a rationale for your choices
`

func QueryElevationInfo(
	ctx context.Context,
	llm models.LargeLanguageModelImpl,
	provider models.Provider,
	workflow models.Workflow,
	providers map[string]models.Provider,
	evaluationResponse *ElevationRequestResponse,
) (*ElevationQueryResponse, error) {

	if llm == nil {
		return nil, fmt.Errorf("LLM is not configured")
	}

	if evaluationResponse == nil {
		return nil, fmt.Errorf("evaluation is required")
	}

	// Convert provider impl to a string[]
	var providerNames []string
	for name := range providers {
		providerNames = append(providerNames, name)
	}

	systemPrompt := fmt.Sprintf("%s\n\n%s",
		InitalSystemPrompt, QueryElevationInfoPrompt)

	// First using the provided provider query to find all the similar roles
	// and permissions that closely match the request

	response, err := llm.GenerateContent(
		context.Background(),
		llm.GetModelName(),
		[]*genai.Content{
			{
				Parts: []*genai.Part{
					{
						Text: "Based on the evaluation provided, please analyze and identify the roles, permissions, and resources needed for this elevation request.",
					},
					{
						FunctionResponse: &genai.FunctionResponse{
							Name: EvaluateRequestToolName,
							Response: map[string]any{
								"provider": provider,
								"workflow": workflow,
								"request":  evaluationResponse.Request,
								"duration": evaluationResponse.Duration.String(),
							},
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
					Mode:                 genai.FunctionCallingConfigModeAny,
					AllowedFunctionNames: []string{QueryElevationInfoToolName},
				},
			},
			Temperature: &LLM_TEMPERATURE,
			Seed:        &LLM_SEED,
		},
	)

	if err != nil {
		logrus.WithError(err).Error("failed to generate query elevation info content")
		return nil, err
	}

	candidates := []*ElevationQueryResponse{}

	for _, candidate := range response.Candidates {
		// Process each candidate and extract the relevant information
		// to create a role

		for _, part := range candidate.Content.Parts {
			// Process each part and extract the relevant information
			if part.FunctionCall != nil {
				// If the part has a function call, we can use it
				if part.FunctionCall.Name == QueryElevationInfoToolName {
					// If the function call is "createEvaluation", we can use it
					// Extract the parameters from the function call

					parameters := part.FunctionCall.Args
					evaluation, err := createElevationQueryResponseFromParams(parameters)
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

	return nil, fmt.Errorf("no valid role found")

}

func createElevationQueryResponseFromParams(params map[string]any) (*ElevationQueryResponse, error) {

	rationale, rationaleOk := params["rationale"].(string)

	if !rationaleOk {
		logrus.Warn("missing rationale field")
		return nil, fmt.Errorf("No valid rationale provided")
	}

	res := ElevationQueryResponse{
		BaseElevationResponse: BaseElevationResponse{
			Rationale: rationale,
			Success:   false,
		},
	}

	rolesAny, rolesOk := params["roles"].([]any)
	permissionsAny, permisisonsOk := params["permissions"].([]any)
	resourcesAny, resourcesOk := params["resources"].([]any)
	success, successOk := params["success"].(bool)

	if rolesOk {
		// Convert []any to []string
		for _, r := range rolesAny {
			if roleStr, ok := r.(string); ok {
				res.Roles = append(res.Roles, roleStr)
			}
		}
	}

	if permisisonsOk {
		for _, p := range permissionsAny {
			if permStr, ok := p.(string); ok {
				res.Permissions = append(res.Permissions, permStr)
			}
		}
	}

	if resourcesOk {
		for _, r := range resourcesAny {
			if resStr, ok := r.(string); ok {
				res.Resources = append(res.Resources, resStr)
			}
		}
	}

	if successOk {
		res.Success = success
	} else {
		logrus.Warn("missing success field, defaulting to false")
		success = false
	}

	return &res, nil
}
