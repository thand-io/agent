package llm

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

var LLM_TEMPERATURE = float32(0.2)
var LLM_SEED = int32(1337)

func GenerateElevateRequestFromReason(
	ctx context.Context,
	llm models.LargeLanguageModelImpl,
	providers map[string]models.Provider,
	workflows map[string]models.Workflow,
	reason string,
) (*models.ElevateRequest, error) {

	if err := validateInputs(llm, providers, reason); err != nil {
		return nil, err
	}

	evaluationResponse, err := CreateEvaluateRequest(
		ctx, llm, providers, workflows, reason)
	if err != nil {
		return nil, err
	}

	if err := validateEvaluationResponse(evaluationResponse, reason); err != nil {
		return nil, err
	}

	provider, workflow, err := getProviderAndWorkflow(
		evaluationResponse, providers, workflows)
	if err != nil {
		return nil, err
	}

	queryableInfo, err := getQueryableInfo(
		ctx, llm, provider, workflow, providers, evaluationResponse, reason)
	if err != nil {
		return nil, err
	}

	role, err := GenerateRole(
		ctx, llm, provider, workflow, providers, evaluationResponse, queryableInfo)
	if err != nil {
		logrus.WithError(err).Error("failed to generate role")
		return nil, err
	}

	return buildElevateRequest(role, evaluationResponse, workflows, reason), nil
}

// validateInputs validates the basic inputs to the function
func validateInputs(llm models.LargeLanguageModelImpl, providers map[string]models.Provider, reason string) error {
	if llm == nil {
		return fmt.Errorf("LLM is not configured")
	}

	if len(providers) == 0 {
		return fmt.Errorf("no providers configured")
	}

	if len(reason) == 0 {
		return fmt.Errorf("reason is required")
	}

	return nil
}

// validateEvaluationResponse validates the evaluation response from the LLM
func validateEvaluationResponse(evaluationResponse *ElevationRequestResponse, reason string) error {
	if !evaluationResponse.Success {
		logrus.WithFields(logrus.Fields{
			"request":   reason,
			"rationale": evaluationResponse.Rationale,
		}).Warn("failed to elevate")
		return fmt.Errorf("failed to elevate: %s", evaluationResponse.Rationale)
	}

	return nil
}

// getProviderAndWorkflow retrieves and validates the provider and workflow
func getProviderAndWorkflow(evaluationResponse *ElevationRequestResponse, providers map[string]models.Provider, workflows map[string]models.Workflow) (models.Provider, models.Workflow, error) {
	provider, ok := providers[evaluationResponse.Provider]
	if !ok {
		return models.Provider{}, models.Workflow{}, fmt.Errorf("provider not found: %s", evaluationResponse.Provider)
	}

	if len(evaluationResponse.Workflow) == 0 {
		return models.Provider{}, models.Workflow{}, fmt.Errorf("workflow is required from LLM")
	}

	workflow, ok := workflows[evaluationResponse.Workflow]
	if !ok {
		return models.Provider{}, models.Workflow{}, fmt.Errorf("workflow not found: %s", evaluationResponse.Workflow)
	}

	return provider, workflow, nil
}

// getQueryableInfo queries and validates the elevation information
func getQueryableInfo(ctx context.Context, llm models.LargeLanguageModelImpl, provider models.Provider, workflow models.Workflow, providers map[string]models.Provider, evaluationResponse *ElevationRequestResponse, reason string) (*ElevationQueryResponse, error) {
	queryableInfo, err := QueryElevationInfo(
		ctx, llm, provider, workflow, providers, evaluationResponse)
	if err != nil {
		logrus.WithError(err).Error("failed to query elevation info")
		return nil, err
	}

	if !queryableInfo.Success {
		logrus.WithFields(logrus.Fields{
			"request":   reason,
			"duration":  evaluationResponse.Duration.String(),
			"rationale": queryableInfo.Rationale,
		}).Warn("failed to query elevation info")
		return nil, fmt.Errorf("failed to query elevation info: %s", queryableInfo.Rationale)
	}

	if len(queryableInfo.Permissions) == 0 && len(queryableInfo.Roles) == 0 {
		return nil, fmt.Errorf("no roles or permissions found from LLM")
	}

	return queryableInfo, nil
}

// buildElevateRequest constructs the final ElevateRequest
func buildElevateRequest(role *models.Role, evaluationResponse *ElevationRequestResponse, workflows map[string]models.Workflow, reason string) *models.ElevateRequest {
	if len(evaluationResponse.Workflow) == 0 {
		// get the first key name from the workflows map
		for name := range workflows {
			evaluationResponse.Workflow = name
			break
		}
	}

	if len(evaluationResponse.Workflow) == 0 {
		return nil
	}

	role.Providers = []string{evaluationResponse.Provider}
	role.Workflow = evaluationResponse.Workflow

	return &models.ElevateRequest{
		Role:     role,
		Provider: evaluationResponse.Provider,
		Reason:   reason,
		Duration: evaluationResponse.Duration.String(),
	}
}
