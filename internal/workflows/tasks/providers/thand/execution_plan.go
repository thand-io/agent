package thand

import (
	agentConfig "github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"go.temporal.io/sdk/workflow"
)

const executionPlanActivityTimeout = 1 * models.DeviceRouteFreshnessTTL

func (t *thandTask) ensureExecutionPlan(
	workflowTask *models.ElevateWorkflowTask,
	elevateRequest *models.ElevateRequestInternal,
) (*models.ExecutionPlan, error) {
	if plan, err := workflowTask.GetContextAsExecutionPlan(); err == nil {
		return plan, nil
	}

	workflowID := workflowTask.GetWorkflowID()
	if !workflowTask.HasTemporalContext() {
		plan, err := agentConfig.BuildExecutionPlan(t.config, workflowID, elevateRequest)
		if err != nil {
			return nil, err
		}
		workflowTask.SetExecutionPlan(plan)
		return plan, nil
	}

	ctx := workflowTask.GetTemporalContext()
	actx := workflow.WithActivityOptions(ctx, workflow.ActivityOptions{
		StartToCloseTimeout: executionPlanActivityTimeout,
	})

	var plan models.ExecutionPlan
	err := workflow.ExecuteActivity(
		actx,
		models.TemporalBuildExecutionPlanActivityName,
		models.ExecutionPlanRequest{
			WorkflowID:     workflowID,
			ElevateRequest: elevateRequest,
		},
	).Get(ctx, &plan)
	if err != nil {
		return nil, err
	}

	workflowTask.SetExecutionPlan(&plan)
	return &plan, nil
}

func (t *thandTask) requireExecutionPlan(
	workflowTask *models.ElevateWorkflowTask,
) (*models.ExecutionPlan, error) {
	plan, err := workflowTask.GetContextAsExecutionPlan()
	if err != nil {
		return nil, err
	}

	return plan, nil
}
