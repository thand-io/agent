package thand

import (
	"fmt"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	taskModel "github.com/thand-io/agent/internal/workflows/tasks/model"
	runner "github.com/thand-io/agent/sdk/workflows/runner"
)

const ThandMonitorTask = "monitor"

type MonitorRequest struct {
	Mode      string `json:"mode,omitempty"`      // e.g., "alert", "log", etc.
	Threshold int    `json:"threshold,omitempty"` // e.g., number of alerts to trigger action
}

// ThandMonitorTask represents a custom task for Thand monitoring
func (t *thandTask) executeMonitorTask(
	workflowTask *models.ElevateWorkflowTask,
	taskName string,
	call *taskModel.ThandTask,
	input any) (any, error) {

	if !workflowTask.HasTemporalContext() {
		// Only supported within Temporal workflows
		return nil, fmt.Errorf("monitoring is only supported with temporal for task: %s", taskName)
	}

	var monitorReq MonitorRequest
	err := common.ConvertInterfaceToInterface(call.With, &monitorReq)

	if err != nil {
		return nil, fmt.Errorf("failed to parse monitor request: %w", err)
	}

	log := workflowTask.GetLogger()

	log.WithFields(logrus.Fields{
		"task_name": taskName,
		"mode":      monitorReq.Mode,
		"threshold": monitorReq.Threshold,
	}).Info("Executing Thand monitor task")

	thandAlert, err := runner.ListenTaskHandler(workflowTask, taskName, &model.ListenTask{
		Listen: model.ListenTaskConfiguration{
			To: &model.EventConsumptionStrategy{
				Any: []*model.EventFilter{
					{
						With: &model.EventProperties{
							Type: "com.thand.alert",
						},
					},
				},
			},
		},
	}, input)

	if err != nil {
		log.WithError(err).WithFields(logrus.Fields{
			"taskName": taskName,
		}).Error("Failed to listen for Thand alert")

		return nil, err
	}

	log.WithField("taskName", taskName).Info("Received Thand alert in monitor task")

	var alertData map[string]any

	if alertEvent, ok := thandAlert.(*cloudevents.Event); ok {

		alertEvent.DataAs(&alertData)

		if level, exists := alertData["level"]; exists {

			if levelStr, ok := level.(string); ok && levelStr == "critical" {
				log.WithField("taskName", taskName).Warn("Critical alert received in Thand monitor task")
				// Handle critical alert (e.g., escalate, notify, etc.)
				return alertEvent, nil
			}
		}
	}

	// Keep listening for more alerts
	return &model.FlowDirective{
		Value: taskName, // loop back to await more alerts
	}, nil
}
