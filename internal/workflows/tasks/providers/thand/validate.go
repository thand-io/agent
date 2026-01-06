package thand

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	taskModel "github.com/thand-io/agent/internal/workflows/tasks/model"
)

const ThandValidateTask = "validate"

var VALIDATOR_STATIC = "static"
var VALIDATOR_LLM = "llm"

// ThandValidateTask represents a custom task for Thand validation
func (t *thandTask) executeValidateTask(
	workflowTask *models.ThandWorkflowTask,
	call *taskModel.ThandTask,
	input any) (any, error) {

	req := workflowTask.GetContextAsMap()

	if req == nil {
		return nil, errors.New("request cannot be nil")
	}

	log := workflowTask.GetLogger()

	// The request should always map to an Elevate Request object
	var elevateRequest models.ElevateRequestInternal
	if err := common.ConvertMapToInterface(req, &elevateRequest); err != nil {
		return nil, fmt.Errorf("failed to convert request: %w", err)
	}

	duration := strings.ToLower(elevateRequest.Duration)
	role := elevateRequest.Role
	reason := elevateRequest.Reason

	if role == nil {
		return nil, errors.New("role must be provided")
	}

	if len(reason) == 0 {
		return nil, errors.New("reason must be provided")
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

	log.WithFields(logrus.Fields{
		"duration": duration,
		"role":     role,
		"reason":   reason,
	}).Info("Validating elevate request")

	// Convert duration to ISO 8601 format from string
	if _, err := elevateRequest.AsDuration(); err != nil {
		return nil, fmt.Errorf("invalid duration format: %s got: %w", duration, err)
	}

	ctx := workflowTask.GetContext()

	with := call.With

	validator, exists := with.GetString("validator")

	// Validate validator type
	if !exists {
		validator = VALIDATOR_STATIC
	}

	log.WithFields(logrus.Fields{
		"validator": validator,
	}).Info("Executing validation")

	if len(elevateRequest.Providers) == 0 {
		return nil, errors.New("no providers specified in elevate request")
	}

	primaryProvider := elevateRequest.Providers[0]

	providerCall, err := t.config.GetProviderByName(primaryProvider)
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

	log.WithFields(logrus.Fields{
		"role":      elevateRequest.Role,
		"providers": elevateRequest.Providers,
		"output":    validateOut,
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
func (t *thandTask) executeLLMValidation(
	ctx context.Context,
	call *taskModel.ThandTask,
	elevateRequest models.ElevateRequestInternal,
) (map[string]any, error) {

	reason := elevateRequest.Reason

	if len(reason) == 0 {
		return nil, errors.New("reason must be provided")
	}

	withOptions := call.With

	modelName := withOptions.GetStringWithDefault("model", "gemini-2.5-pro")

	// TODO validate reason to make sure its valid.

	fmt.Println("Using model: ", modelName)
	return nil, nil
}

func (t *thandTask) executeStaticValidation(
	_ context.Context,
	_ *taskModel.ThandTask,
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
