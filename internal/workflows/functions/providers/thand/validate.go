package thand

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/functions"
)

// ValidateFunction implements access request validation using LLM or user input
type validateFunction struct {
	config *config.Config
	*functions.BaseFunction
}

var VALIDATOR_STATIC = "static"
var VALIDATOR_LLM = "llm"

// NewValidateFunction creates a new validation Function
func NewValidateFunction(config *config.Config) *validateFunction {
	return &validateFunction{
		config: config,
		BaseFunction: functions.NewBaseFunction(
			"thand.validate",
			"This validates the incoming access requests. To ensure the provided roles and providers are authorized.",
			"1.0.0",
		),
	}
}

// GetRequiredParameters returns the required parameters for validation
func (t *validateFunction) GetRequiredParameters() []string {
	return []string{} // No parameters are strictly required
}

// GetOptionalParameters returns optional parameters with defaults
func (t *validateFunction) GetOptionalParameters() map[string]any {
	return map[string]any{
		"validator": "strict",
	}
}

// ValidateRequest validates the input parameters
func (t *validateFunction) ValidateRequest(
	workflowTask *models.WorkflowTask,
	call *model.CallFunction,
	input any,
) error {

	req := workflowTask.GetContextAsMap()

	if req == nil {
		return errors.New("request cannot be nil")
	}

	// The request should always map to an Elevate Request object

	var elevateRequest models.ElevateRequest
	if err := common.ConvertMapToInterface(req, &elevateRequest); err != nil {
		return fmt.Errorf("failed to convert request: %w", err)
	}

	duration := strings.ToLower(elevateRequest.Duration)
	role := elevateRequest.Role
	reason := elevateRequest.Reason

	if role == nil {
		return errors.New("role must be provided")
	}

	if len(reason) == 0 {
		return errors.New("reason must be provided")
	}

	if len(duration) == 0 {
		duration = "t1h" // Default to 1 hour if not provided
	}

	// Try and fix durations
	if !strings.HasPrefix(duration, "t") &&
		!strings.HasPrefix(duration, "p") &&
		!strings.HasPrefix(duration, "pt") {
		// Can't break up durations so assume time
		duration = "t" + duration
	}

	logrus.WithFields(logrus.Fields{
		"duration": duration,
		"role":     role,
		"reason":   reason,
	}).Info("Validating elevate request")

	// Convert duration to ISO 8601 format from string
	if _, err := elevateRequest.AsDuration(); err != nil {
		return fmt.Errorf("invalid duration format: %s got: %w", duration, err)
	}

	return nil
}

// Execute performs the validation logic
func (t *validateFunction) Execute(
	workflowTask *models.WorkflowTask,
	call *model.CallFunction,
	input any,
) (any, error) {

	ctx := workflowTask.GetContext()
	with := call.With
	validator, exists := with["validator"].(string)

	// Validate validator type
	if !exists {
		validator = VALIDATOR_STATIC
	}

	logrus.WithFields(logrus.Fields{
		"validator": validator,
	}).Info("Executing validation")

	req := workflowTask.GetContextAsMap()

	// Convert req to ElevateRequest
	var elevateRequest models.ElevateRequestInternal
	if err := common.ConvertMapToInterface(req, &elevateRequest); err != nil {
		return nil, fmt.Errorf("failed to convert request: %w", err)
	}

	providerCall, err := t.config.GetProviderByName(elevateRequest.Provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	responseOut := map[string]any{}

	// Validate role
	validateOut, err := models.ValidateRole(providerCall.GetClient(), elevateRequest)

	if err != nil {
		return nil, err
	}

	// If the validation returned any output, merge it into responseOut
	if len(validateOut) > 0 {
		maps.Copy(responseOut, validateOut)
	}

	logrus.WithFields(logrus.Fields{
		"role":     elevateRequest.Role,
		"provider": elevateRequest.Provider,
		"output":   validateOut,
	}).Info("Role validated successfully")

	// TODO: Do something with the output for static validation

	switch validator {
	case VALIDATOR_STATIC:
		// Perform static validation
		if response, err := t.executeStaticValidation(ctx, call, elevateRequest); err != nil {

			if len(response) > 0 {
				maps.Copy(responseOut, response)
			}

			return responseOut, err
		}
	case VALIDATOR_LLM:
		// Perform LLM validation - this checks background information
		// like github issues, jira tickets etc to confirm the reason
		// makes sense
		if response, err := t.executeLLMValidation(ctx, call, elevateRequest); err != nil {

			if len(response) > 0 {
				maps.Copy(responseOut, response)
			}

			return responseOut, err
		}
	default:
		return nil, fmt.Errorf("unknown validator: %s", validator)
	}

	return nil, nil
}

// executeLLMValidation performs AI/LLM-based validation
func (t *validateFunction) executeLLMValidation(
	ctx context.Context,
	call *model.CallFunction,
	elevateRequest models.ElevateRequestInternal,
) (map[string]any, error) {

	reason := elevateRequest.Reason

	if len(reason) == 0 {
		return nil, errors.New("reason must be provided")
	}

	withOptions := call.With

	modelName, foundModelName := withOptions["model"].(string)

	if !foundModelName {
		modelName = "gemini-2.5-pro"
	}

	// TODO validate reason to make sure its valid.

	fmt.Println("Using model: ", modelName)
	return nil, nil
}

func (t *validateFunction) executeStaticValidation(
	_ context.Context,
	_ *model.CallFunction,
	elevateRequest models.ElevateRequestInternal,
) (map[string]any, error) {

	if elevateRequest.User == nil {
		return nil, errors.New("user must be provided for static validation")
	}

	responseOut := map[string]any{}

	err := common.ConvertInterfaceToInterface(elevateRequest, &responseOut)

	if err != nil {
		return nil, fmt.Errorf("failed to convert elevate request to map: %w", err)
	}

	return responseOut, nil
}
