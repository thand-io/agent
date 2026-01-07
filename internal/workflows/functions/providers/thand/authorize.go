package thand

import (
	"fmt"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/sdk/workflows/functions"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
)

const ThandAuthorizeFunction = "thand.authorize"

// AuthorizeFunction implements access authorization based on roles and workflows
type authorizeFunction struct {
	config models.ConfigImpl
	*functions.BaseFunction
}

// NewAuthorizeFunction creates a new authorization Function
func NewAuthorizeFunction(config models.ConfigImpl) *authorizeFunction {
	return &authorizeFunction{
		config: config,
		BaseFunction: functions.NewBaseFunction(
			"thand.authorize",
			"Authorizes access based on roles and workflows",
			"1.0.0",
		),
	}
}

// GetRequiredParameters returns the required parameters for authorization
func (t *authorizeFunction) GetRequiredParameters() []string {
	return []string{
		"revocation",
	}
}

// GetOptionalParameters returns optional parameters with defaults
func (t *authorizeFunction) GetOptionalParameters() map[string]any {
	return map[string]any{}
}

// ValidateRequest validates the input parameters
func (t *authorizeFunction) ValidateRequest(
	workflowTask sdkWorkflowsModel.WorkflowTaskSupport,
	call *model.CallFunction,
	input any,
) error {
	return nil
}

type ThandAuthorizeRequest struct {
	Provider string `json:"provider"` // Provider to use for authorization
	models.AuthorizeRoleRequest
}

func (r *ThandAuthorizeRequest) IsValid() bool {
	return r.Role != nil && r.User != nil && len(r.Provider) > 0
}

// Execute performs the authorization logic
func (t *authorizeFunction) Execute(
	workflowTask sdkWorkflowsModel.WorkflowTaskSupport,
	call *model.CallFunction,
	input any,
) (any, error) {

	elevateRequest, err := t.validateAndParseRequests(workflowTask, call, input)

	if err != nil {
		return nil, err
	}

	return t.executeAuthorization(workflowTask, elevateRequest)
}

// validateAndParseRequests validates and parses the incoming requests
func (t *authorizeFunction) validateAndParseRequests(
	workflowTask sdkWorkflowsModel.WorkflowTaskSupport,
	call *model.CallFunction,
	input any,
) (*ThandAuthorizeRequest, error) {

	elevationRequest, err := models.NewElevateWorkflowTask(workflowTask).GetContextAsElevationRequest()

	if err != nil {
		return nil, fmt.Errorf("failed to get elevation request from context: %w", err)
	}

	if !elevationRequest.IsValid() {
		return nil, fmt.Errorf("invalid elevate request in context")
	}

	var authRequest ThandAuthorizeRequest
	if err := common.ConvertInterfaceToInterface(input, &authRequest); err != nil {
		return nil, fmt.Errorf("failed to convert auth request: %w", err)
	}

	if err := common.ConvertInterfaceToInterface(call.With, &authRequest); err != nil {
		return nil, fmt.Errorf("failed to convert call request: %w", err)
	}

	// Set tenant if available
	// TODO(hugh): Only a single tenant is supported for now
	// later we can expand this to support multiple tenants
	if len(elevationRequest.Tenants) > 0 {
		authRequest.RoleRequest.Tenant = elevationRequest.Tenants[0]
	}

	if !authRequest.IsValid() {
		logrus.Infoln("No revocation state provided. Just handling via the cleanup state")
	}

	return &authRequest, nil
}

// executeAuthorization performs the main authorization workflow
func (t *authorizeFunction) executeAuthorization(
	workflowTask sdkWorkflowsModel.WorkflowTaskSupport,
	elevateRequest *ThandAuthorizeRequest,
) (any, error) {

	// ElevateRequest contains the role to be authorized
	// AuthRequest contains the revocation state and the user to be authorized

	provider, err := t.config.GetProviderByName(elevateRequest.Provider)

	if err != nil {
		return nil, fmt.Errorf("failed to get provider: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"provider":      elevateRequest.Provider,
		"provider_type": provider.GetProvider(),
		"user_email":    elevateRequest.RoleRequest.User.Email,
		"user_source":   elevateRequest.RoleRequest.User.Source,
		"user_username": elevateRequest.RoleRequest.User.Username,
	}).Info("About to call AuthorizeRole on provider")

	authOut, err := provider.AuthorizeRole(
		workflowTask.GetContext(), &models.AuthorizeRoleRequest{
			RoleRequest: elevateRequest.RoleRequest,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to authorize user: %w", err)
	}

	authorizedAt := time.Now().UTC()
	revocationDate := authorizedAt.Add(*elevateRequest.Duration)

	logrus.WithFields(logrus.Fields{
		"authorized_at": authorizedAt.Format(time.RFC3339),
		"revocation_at": revocationDate.Format(time.RFC3339),
	}).Info("Scheduled revocation")

	return authOut, nil
}

func (t *authorizeFunction) GetExport() *model.Export {
	return &model.Export{
		As: model.NewObjectOrRuntimeExpr(
			model.RuntimeExpression{
				Value: "${ $context + . }",
			},
		),
	}
}
