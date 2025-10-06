package slack

import (
	"errors"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/functions"
)

// SlackFunction implements Slack notification functionality
type slackFunction struct {
	*functions.BaseFunction
}

// NewSlackFunction creates a new Slack notification Function
func NewSlackPostMessageFunction(config *config.Config) *slackFunction {
	return &slackFunction{
		BaseFunction: functions.NewBaseFunction(
			"slack.postMessage",
			"Sends notifications and messages to Slack channels",
			"1.0.0",
		),
	}
}

// GetRequiredParameters returns the required parameters for Slack notifications
func (t *slackFunction) GetRequiredParameters() []string {
	return []string{"provider", "to"}
}

// GetOptionalParameters returns optional parameters with defaults
func (t *slackFunction) GetOptionalParameters() map[string]any {
	return map[string]any{
		"text": "Your message",
	}
}

// ValidateRequest validates the input parameters
func (t *slackFunction) ValidateRequest(
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

// Execute performs the Slack notification logic
func (t *slackFunction) Execute(
	workflowTask *models.WorkflowTask,
	call *model.CallFunction,
	req any,
) (any, error) {

	return nil, nil
}
