package thand

import (
	"errors"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/functions"
)

// monitorFunction implements security monitoring setup for access sessions
type monitorFunction struct {
	config *config.Config
	*functions.BaseFunction
}

// NewMonitorFunction creates a new monitoring Function
func NewMonitorFunction(config *config.Config) *monitorFunction {
	return &monitorFunction{
		config: config,
		BaseFunction: functions.NewBaseFunction(
			"thand.monitor",
			"Sets up monitoring for access sessions with AI-powered security analysis",
			"1.0.0",
		),
	}
}

// GetRequiredParameters returns the required parameters for monitoring
func (t *monitorFunction) GetRequiredParameters() []string {
	return []string{}
}

// GetOptionalParameters returns optional parameters with defaults
func (t *monitorFunction) GetOptionalParameters() map[string]any {
	return map[string]any{
		"llm_enabled":         true,
		"threshold":           10.0,
		"anomaly_detection":   true,
		"behavioral_analysis": true,
		"real_time_alerts":    true,
		"monitoring_level":    "standard",
		"webhook_url":         "",
		"alert_channels":      []string{"slack", "email"},
		"sampling_rate":       1.0,
	}
}

// ValidateRequest validates the input parameters
func (t *monitorFunction) ValidateRequest(
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

// Execute performs the monitoring setup logic
func (t *monitorFunction) Execute(
	workflowTask *models.WorkflowTask,
	call *model.CallFunction,
	input any,
) (any, error) {

	// Figure out what provider is being used
	// and then make API calls to set up monitoring

	_, err := workflowTask.GetContextAsElevationRequest()

	if err != nil {
		return nil, err
	}

	logrus.Infof("Monitoring is not available for request, skipping")

	return nil, nil
}
