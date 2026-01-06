package runner

import (
	"fmt"
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"go.temporal.io/sdk/workflow"
)

func (r *ResumableWorkflowRunner) executeWaitTask(
	taskName string,
	call *model.WaitTask,
	input any,
) (map[string]any, error) {

	log := r.GetLogger()

	log.WithFields(logrus.Fields{
		"task": taskName,
		"wait": call.Wait,
	}).Info("Executing Wait task")

	workflowTask := r.GetWorkflowTask()

	// This task waits for a specified duration or until a specific time
	// For now, simplified implementation

	if call.Wait == nil {
		return nil, fmt.Errorf("wait task with duration or until not implemented yet")
	}

	interpolatedWith, err := workflowTask.TraverseAndEvaluate(call.Wait, input)

	if err != nil {
		return nil, fmt.Errorf("failed to interpolate call.with: %w", err)
	}

	var duration time.Duration

	// Check type of interface
	switch v := interpolatedWith.(type) {
	case string:
		// Assume it's a duration string like "PT5M" (ISO 8601 duration)
		du, err := common.ValidateDuration(v)

		if err != nil {
			return nil, fmt.Errorf("failed to parse wait duration: %w", err)
		}

		duration = du

	case model.DurationExpression:

		du, err := common.ValidateDuration(v.Expression)

		if err != nil {
			return nil, fmt.Errorf("failed to parse wait duration expression: %w", err)
		}

		duration = du

	case model.Duration:

		du, err := common.ValidateDuration(v.AsExpression())

		if err != nil {
			return nil, fmt.Errorf("failed to parse wait duration: %w", err)
		}

		duration = du

	default:
		return nil, fmt.Errorf("unsupported wait value type: %T", v)
	}

	if workflowTask.HasTemporalContext() {

		log.WithFields(logrus.Fields{
			"task":     taskName,
			"duration": duration,
		}).Info("Registering temporal wait")

		err := workflow.Sleep(workflowTask.GetTemporalContext(), duration)
		if err != nil {
			return nil, fmt.Errorf("failed to sleep workflow: %w", err)
		}

	} else {

		log.WithFields(logrus.Fields{
			"task":     taskName,
			"duration": duration,
		}).Info("Executing standard wait")

		// TODO: Enforce limits on wait duration in non-temporal mode
		//if r.config.GetEnvironment().Ephemeral && duration > 1*time.Minute {
		//	return nil, fmt.Errorf("cannot wait for more than 1 minute in ephemeral mode")
		//}

		time.Sleep(duration)
	}

	log.WithFields(logrus.Fields{
		"task": taskName,
	}).Info("Wait task completed")

	return nil, nil
}
