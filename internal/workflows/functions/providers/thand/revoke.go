package thand

import (
	"fmt"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/sdk/workflows/functions"
	workflowModel "github.com/thand-io/agent/sdk/workflows/models"
)

const ThandRevokeFunction = "thand.revoke"

type ThandRevokeRequest struct {
	Provider string `json:"provider"` // Provider to use for revocation
	models.RevokeRoleRequest
}

func (r *ThandRevokeRequest) IsValid() bool {
	return r.RoleRequest != nil && r.RoleRequest.User != nil && r.RoleRequest.Role != nil && len(r.Provider) > 0
}

// RevokeFunction implements access revocation functionality
type revokeFunction struct {
	config models.ConfigImpl
	*functions.BaseFunction
}

// NewRevokeFunction creates a new revocation Function
func NewRevokeFunction(config models.ConfigImpl) *revokeFunction {
	return &revokeFunction{
		config: config,
		BaseFunction: functions.NewBaseFunction(
			ThandRevokeFunction,
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
	workflowTask workflowModel.WorkflowTask,
	call *model.CallFunction,
	input any,
) error {
	return nil
}

// Execute performs the revocation logic
func (t *revokeFunction) Execute(
	workflowTask workflowModel.WorkflowTask,
	call *model.CallFunction,
	input any,
) (any, error) {

	revokeRequest, err := t.validateAndParseRequests(workflowTask, call, input)

	if err != nil {
		return nil, err
	}

	return t.executeRevocation(workflowTask, revokeRequest)
}

// validateAndParseRequests validates and parses the incoming requests
func (t *revokeFunction) validateAndParseRequests(
	workflowTask workflowModel.WorkflowTask,
	call *model.CallFunction,
	input any,
) (*ThandRevokeRequest, error) {

	// Cast to ThandWorkflowTask to access context methods
	thandWorkflowTask, ok := workflowTask.(*models.ThandWorkflowTask)

	if !ok {
		return nil, fmt.Errorf("workflow task is not of type ThandWorkflowTask")
	}

	elevationRequest, err := thandWorkflowTask.GetContextAsElevationRequest()

	if err != nil {
		return nil, fmt.Errorf("failed to get elevation request from context: %w", err)
	}

	if !elevationRequest.IsValid() {
		return nil, fmt.Errorf("invalid elevate request in context")
	}

	var revokeRequest ThandRevokeRequest
	if err := common.ConvertInterfaceToInterface(input, &revokeRequest); err != nil {
		return nil, fmt.Errorf("failed to convert revoke request: %w", err)
	}

	if err := common.ConvertInterfaceToInterface(call.With, &revokeRequest); err != nil {
		return nil, fmt.Errorf("failed to convert call request: %w", err)
	}

	if !revokeRequest.IsValid() {
		logrus.Infoln("No valid revoke request provided")
	}

	return &revokeRequest, nil
}

// executeRevocation performs the main revocation workflow
func (t *revokeFunction) executeRevocation(
	workflowTask workflowModel.WorkflowTask,
	revokeRequest *ThandRevokeRequest,
) (any, error) {

	providerCall, err := t.config.GetProviderByName(revokeRequest.Provider)

	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	revokeOut, err := providerCall.GetClient().RevokeRole(
		workflowTask.GetContext(), &models.RevokeRoleRequest{
			RoleRequest:           revokeRequest.RoleRequest,
			AuthorizeRoleResponse: revokeRequest.AuthorizeRoleResponse,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to revoke user: %w", err)
	}

	revokedAt := time.Now().UTC()

	logrus.WithFields(logrus.Fields{
		"revoked_at": revokedAt.Format(time.RFC3339),
		"user":       revokeRequest.RoleRequest.User.GetIdentity(),
		"role":       revokeRequest.RoleRequest.Role.GetName(),
		"provider":   revokeRequest.Provider,
	}).Info("Successfully revoked access")

	return revokeOut, nil
}

func (t *revokeFunction) GetExport() *model.Export {
	return &model.Export{
		As: model.NewObjectOrRuntimeExpr(
			model.RuntimeExpression{
				Value: "${ $context + . }",
			},
		),
	}
}
