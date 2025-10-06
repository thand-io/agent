package thand

import (
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/functions"
)

// approvalsFunction implements access approval based on roles and workflows
type approvalsFunction struct {
	config *config.Config
	*functions.BaseFunction
}

// NewApprovalsFunction creates a new approvals Function
func NewApprovalsFunction(config *config.Config) *approvalsFunction {
	return &approvalsFunction{
		config: config,
		BaseFunction: functions.NewBaseFunction(
			"thand.approvals",
			"Approves access based on roles and workflows",
			"1.0.0",
		),
	}
}

// GetRequiredParameters returns the required parameters for validation
func (t *approvalsFunction) GetRequiredParameters() []string {
	return []string{
		"provider",
	} // No strictly required parameters
}

// GetOptionalParameters returns optional parameters with defaults
func (t *approvalsFunction) GetOptionalParameters() map[string]any {
	return map[string]any{
		"provider": "email",
	}
}

// ValidateRequest validates the input parameters
func (t *approvalsFunction) ValidateRequest(
	workflowTask *models.WorkflowTask,
	call *model.CallFunction,
	input any,
) error {
	return nil
}

// Execute performs the validation logic
func (t *approvalsFunction) Execute(
	workflowTask *models.WorkflowTask,
	call *model.CallFunction,
	req any,
) (any, error) {
	return nil, nil
}
