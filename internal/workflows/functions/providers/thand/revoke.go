package thand

import (
	"errors"
	"fmt"
	"maps"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/functions"
)

// RevokeFunction implements access revocation functionality
type revokeFunction struct {
	config *config.Config
	*functions.BaseFunction
}

// NewRevokeFunction creates a new revocation Function
func NewRevokeFunction(config *config.Config) *revokeFunction {
	return &revokeFunction{
		config: config,
		BaseFunction: functions.NewBaseFunction(
			"thand.revoke",
			"Revokes access permissions and terminates sessions",
			"1.0.0",
		),
	}
}

// GetRequiredParameters returns the required parameters for revocation
func (t *revokeFunction) GetRequiredParameters() []string {
	return []string{}
}

// GetOptionalParameters returns optional parameters with defaults
func (t *revokeFunction) GetOptionalParameters() map[string]any {
	return map[string]any{
		"reason": "Manual revocation",
	}
}

// ValidateRequest validates the input parameters
func (t *revokeFunction) ValidateRequest(
	workflowTask *models.WorkflowTask,
	call *model.CallFunction,
	input any,
) error {

	req := workflowTask.GetContextAsMap()

	if req == nil {
		return errors.New("request cannot be nil")
	}

	return nil
}

// Execute performs the revocation logic
func (t *revokeFunction) Execute(
	workflowTask *models.WorkflowTask,
	call *model.CallFunction,
	input any,
) (any, error) {

	req := workflowTask.GetContextAsMap()

	if req == nil {
		return nil, errors.New("request cannot be nil")
	}

	// Right - we ned to take the role, polciy and provide and make the request to
	// the provider to elevate.

	var elevateRequest models.ElevateRequestInternal
	if err := common.ConvertMapToInterface(req, &elevateRequest); err != nil {
		return nil, fmt.Errorf("failed to convert request: %w", err)
	}

	if !elevateRequest.IsValid() {
		return nil, errors.New("invalid elevate request")
	}

	user := elevateRequest.User
	role := elevateRequest.Role
	provider := elevateRequest.Provider
	duration, err := elevateRequest.AsDuration()

	if err != nil {
		return nil, fmt.Errorf("failed to get duration: %w", err)
	}

	// TODO use the duration to revoke the request

	logrus.WithFields(logrus.Fields{
		"user":     user,
		"role":     role,
		"provider": provider,
		"duration": duration,
	}).Info("Executing authorization logic")

	// First lets call the provider to execute the role request.

	providerCall, err := t.config.GetProviderByName(provider)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	modelOutput := map[string]any{
		"revoked": true,
	}

	revokeOut, err := providerCall.GetClient().RevokeRole(
		workflowTask.GetContext(), user, role, req)

	if err != nil {
		return nil, fmt.Errorf("failed to revoke user: %w", err)
	}

	// If the revoke returned any output, merge it into modelOutput
	if len(revokeOut) > 0 {
		maps.Copy(modelOutput, revokeOut)
	}

	return &modelOutput, nil
}
