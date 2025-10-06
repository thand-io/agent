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
STEP 3

Lastly, now that we have the provider, roles, permissions and resources
we can now generate the actual role that will be used to elevate the user.

This role will then be passed back to the caller to use in their request.
*/

var GenerateRoleSystemPrompt = `Act as a cloud security architect specializing 
in IAM role creation and access control. Your task is to create a 
secure, principle-of-least-privilege role based on the user's evaluated
request.

**Role Creation Guidelines**:

1. **Name & Description**:
   - Create a descriptive, professional role name
   - Include clear description of the role's purpose
   - Reference the time-limited nature

2. **Role Inheritance & Permissions**:
   **You can use EITHER inheritance OR explicit permissions OR BOTH:**
   
   - **Inheritance Approach**: Inherit from existing roles that provide baseline permissions
     - Use the "inherits" field to specify role names to inherit from
     - Inherited permissions automatically include all allow/deny rules from parent roles
     - Ideal when existing roles closely match the needed permissions
   
   - **Explicit Permissions Approach**: Define specific permissions directly
     - Use "permissions.allow" for specific service actions needed
     - Use "permissions.deny" for explicit restrictions
     - Grant ONLY the minimum permissions needed
     - Use specific service actions rather than wildcards when possible
   
   - **Hybrid Approach**: Combine inheritance with additional explicit permissions
     - Inherit from a base role that provides common permissions
     - Add specific additional permissions in "permissions.allow" for unique requirements
     - Add specific denials in "permissions.deny" to restrict inherited permissions
     - This approach provides flexibility while maintaining security

3. **Permissions (Allow/Deny)**:
   - Consider read vs write vs admin access levels
   - Include necessary supporting permissions (e.g., describe actions for context)
   - Add explicit deny rules for high-risk actions if needed
   - When using inheritance, review what the parent role provides before adding explicit permissions

4. **Resources (Allow/Deny)**:
   - Specify exact resource ARNs when possible. Only include account, project or organisation ids
     if provided. Otherwise, use wildcards.
   - Use path-based restrictions appropriately
   - Consider account, region, and environment boundaries
   - Deny access to sensitive resources not needed for the task

5. **Security Considerations**:
   - Follow principle of least privilege
   - Consider potential privilege escalation vectors
   - Include appropriate resource boundaries
   - Balance functionality with security
   - When inheriting roles, ensure parent roles don't grant excessive permissions

**Common Patterns**:
- **Read Access**: Get*, List*, Describe* actions
- **Write Access**: Create*, Update*, Put*, Delete* actions  
- **Admin Access**: Full service access with restrictions
- **Cross-Service Dependencies**: S3 for logs, IAM for roles, etc.

**Resource Patterns**:
- AWS: arn:aws:service:region:account:resource-type/resource-name
- Wildcards: Use judiciously and with appropriate boundaries
- Account boundaries: arn:aws:service:region:ACCOUNT_ID:*

**Rationale Requirements**:
Explain your permission and resource choices, including:
- Why specific permissions were granted
- How resource restrictions enhance security  
- Any trade-offs made between usability and security
- Potential risks and mitigations`

var GenerateRolePrompt = `Create a secure, time-appropriate role for 
this request:

- User needs access to: %s
- Specific request: %s
- Duration: %s
- Rationale: %s

`

func GenerateRole(
	ctx context.Context,
	llm models.LargeLanguageModelImpl,
	provider models.Provider,
	workflow models.Workflow,
	providers map[string]models.Provider,
	evaluationResponse *ElevationRequestResponse,
	queryResponse *ElevationQueryResponse,
) (*models.Role, error) {

	if err := validateGenerateRoleInputs(llm, evaluationResponse, queryResponse); err != nil {
		return nil, err
	}

	providerNames := extractProviderNames(providers)
	systemPrompt := fmt.Sprintf("%s\n\n%s", InitalSystemPrompt, QueryElevationInfoPrompt)

	queryResponseMap, err := buildQueryResponseMap(ctx, provider.GetClient(), queryResponse)
	if err != nil {
		return nil, err
	}

	response, err := generateLLMContent(llm, systemPrompt, providerNames, provider, workflow, evaluationResponse, queryResponseMap)
	if err != nil {
		return nil, err
	}

	return extractRoleFromResponse(provider, response)
}

func validateGenerateRoleInputs(llm models.LargeLanguageModelImpl, evaluationResponse *ElevationRequestResponse, queryResponse *ElevationQueryResponse) error {
	if llm == nil {
		return fmt.Errorf("LLM is not configured")
	}
	if evaluationResponse == nil {
		return fmt.Errorf("evaluation is required")
	}
	if queryResponse == nil {
		return fmt.Errorf("query response is required")
	}
	return nil
}

func extractProviderNames(providers map[string]models.Provider) []string {
	var providerNames []string
	for name := range providers {
		providerNames = append(providerNames, name)
	}
	return providerNames
}

func buildQueryResponseMap(ctx context.Context, providerClient models.ProviderImpl, queryResponse *ElevationQueryResponse) (map[string]any, error) {
	queryResponseMap := map[string]any{
		"roles":       []string{},
		"permissions": []string{},
		"resources":   []string{},
	}

	if len(queryResponse.Roles) > 0 {
		foundRoles, err := providerClient.ListRoles(ctx, queryResponse.Roles...)
		if err != nil {
			logrus.WithError(err).Warn("failed to list roles, continuing without role validation")
		}
		queryResponseMap["roles"] = foundRoles
	}

	if len(queryResponse.Permissions) > 0 {
		foundPermissions, err := providerClient.ListPermissions(ctx, queryResponse.Permissions...)
		if err != nil {
			logrus.WithError(err).Warn("failed to list permissions, continuing without permission validation")
		}
		queryResponseMap["permissions"] = foundPermissions
	}

	if len(queryResponse.Resources) > 0 {
		foundResources, err := providerClient.ListResources(ctx, queryResponse.Resources...)
		if err != nil {
			logrus.WithError(err).Warn("failed to list resources, continuing without resource validation")
		}
		queryResponseMap["resources"] = foundResources
	}

	return queryResponseMap, nil
}

func generateLLMContent(
	llm models.LargeLanguageModelImpl,
	systemPrompt string,
	providerNames []string,
	provider models.Provider,
	workflow models.Workflow,
	evaluationResponse *ElevationRequestResponse,
	queryResponseMap map[string]any,
) (*genai.GenerateContentResponse, error) {

	response, err := llm.GenerateContent(
		context.Background(),
		llm.GetModelName(),
		[]*genai.Content{
			{
				Parts: []*genai.Part{
					{
						Text: fmt.Sprintf(GenerateRolePrompt,
							evaluationResponse.Provider,
							evaluationResponse.Request,
							evaluationResponse.Duration.String(),
							evaluationResponse.Rationale,
						),
					},
					{
						// Return the results in the function call
						FunctionResponse: &genai.FunctionResponse{
							Name: EvaluateRequestToolName,
							Response: map[string]any{
								"provider": provider,
								"workflow": workflow,
								"duration": evaluationResponse.Duration.String(),
							},
						},
					},
					{
						FunctionResponse: &genai.FunctionResponse{
							Name:     GenerateRoleToolName,
							Response: queryResponseMap,
						},
					},
				},
			},
		},
		&genai.GenerateContentConfig{
			Tools: getToolchain(providerNames),
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
					AllowedFunctionNames: []string{GenerateRoleToolName},
				},
			},
			Temperature: &LLM_TEMPERATURE,
			Seed:        &LLM_SEED,
		},
	)

	if err != nil {
		logrus.WithError(err).Error("failed to generate role content")
		return nil, err
	}

	return response, nil
}

func extractRoleFromResponse(provider models.Provider, response *genai.GenerateContentResponse) (*models.Role, error) {
	candidates := []*models.Role{}

	for _, candidate := range response.Candidates {
		for _, part := range candidate.Content.Parts {
			if part.FunctionCall != nil && part.FunctionCall.Name == GenerateRoleToolName {
				role, err := createRoleFromParams(provider, part.FunctionCall.Args)
				if err != nil {
					return nil, err
				}
				candidates = append(candidates, role)
			}
		}
	}

	if len(candidates) > 0 {
		return candidates[0], nil
	}

	return nil, fmt.Errorf("no valid role found")
}

func createRoleFromParams(provider models.Provider, params map[string]any) (*models.Role, error) {
	var role models.Role
	err := common.ConvertMapToInterface(params, &role)

	if err != nil {
		return nil, err
	}

	validateOut, err := models.ValidateRole(provider.GetClient(), models.ElevateRequestInternal{
		ElevateRequest: models.ElevateRequest{
			Role: &role,
		},
	})

	if err != nil {

		logrus.WithFields(logrus.Fields{
			"error": err,
			"role":  role,
		}).Error("Failed to validate generated role")

		return nil, err
	}

	logrus.WithFields(logrus.Fields{
		"role":   role,
		"output": validateOut,
	}).Info("Generated role validated successfully")

	return &role, nil
}
