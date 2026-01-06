package models

import (
	"github.com/sirupsen/logrus"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"go.temporal.io/sdk/activity"
	"go.temporal.io/sdk/log"
	"go.temporal.io/sdk/workflow"
)

func (r *ThandWorkflowTask) GetLogger() *sdkWorkflowsModel.LogBuilder {
	var logger log.Logger
	if r.HasTemporalContext() {
		logger = workflow.GetLogger(r.GetTemporalContext())
	} else if activity.IsActivity(r.GetContext()) {
		logger = activity.GetLogger(r.GetContext())
	} else {
		// Use the existing global logger
		logger = sdkWorkflowsModel.NewLogrusAdapter(
			logrus.StandardLogger(),
		)
	}
	return sdkWorkflowsModel.NewLogBuilder(
		logger,
	)
}
