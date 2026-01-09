package config

import (
	"fmt"

	"github.com/sirupsen/logrus"
	models "github.com/thand-io/agent/internal/models"
	sdkWorkflowsConfig "github.com/thand-io/agent/sdk/workflows/config"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

type thandRunner struct {
	config       sdkWorkflowsConfig.Config
	workflowTask sdkWorkflowsModel.WorkflowTaskSupport
}

func NewthandRunner(cfg sdkWorkflowsConfig.Config, workflowTask sdkWorkflowsModel.WorkflowTaskSupport) *thandRunner {
	return &thandRunner{
		config:       cfg,
		workflowTask: workflowTask,
	}
}

func (r *thandRunner) GetConfig() sdkWorkflowsConfig.Config {
	return r.config
}

func (r *thandRunner) GetWorkflowTask() sdkWorkflowsModel.WorkflowTaskSupport {
	return r.workflowTask
}

// HydrateWorkflowTask ensures that the workflow task has its workflow definition loaded
// and its state initialised.
func (c *thandRunner) HydrateWorkflowTask() error {

	workflowTask := c.GetWorkflowTask()

	if workflowTask.GetWorkflowDef() == nil {

		workflowDsl, ok := c.GetConfig().GetWorkflow(workflowTask.GetName())

		if !ok {
			return fmt.Errorf("failed to load workflow")
		}

		workflowTask.SetWorkflowDef(&workflowDsl)

	}

	// Create a new task state if it does not exist
	// This is important as we might be in the middle of a workflow and
	// the state might not have been initialised yet
	if !workflowTask.HasState() {
		workflowTask.ClearTaskContext()
	}

	return nil
}

func (r *thandRunner) PreStateTransitionHook(
	workflowTask sdkWorkflowsModel.WorkflowTaskSupport,
) error {

	if !workflowTask.HasTemporalContext() {
		return nil
	}

	elevateWorkflowTask := models.NewElevateWorkflowTask(workflowTask)

	log := workflowTask.GetLogger()

	ctx := workflowTask.GetTemporalContext()

	updates := []temporal.SearchAttributeUpdate{
		models.TypedSearchAttributeStatus.ValueSet(string(elevateWorkflowTask.GetStatus())),
	}

	isApproved := elevateWorkflowTask.IsApproved()

	if isApproved != nil {
		updates = append(updates,
			models.TypedSearchAttributeApproved.ValueSet(*isApproved),
		)
	}

	_, currentTask := elevateWorkflowTask.GetCurrentTaskItem()

	if currentTask != nil && len(currentTask.Key) > 0 {
		updates = append(updates,
			models.TypedSearchAttributeTask.ValueSet(currentTask.Key),
		)
	}

	elevationRequest, err := elevateWorkflowTask.GetContextAsElevationRequest()

	if err != nil {

		log.WithError(err).Warn("No valid elevation context found, skipping search attribute update.")

	} else {

		if elevationRequest.User != nil && len(elevationRequest.User.Email) > 0 {
			updates = append(updates,
				models.TypedSearchAttributeUser.ValueSet(elevationRequest.User.Email),
			)
		}

		if len(elevationRequest.Role.Name) > 0 {
			updates = append(updates,
				models.TypedSearchAttributeRole.ValueSet(elevationRequest.Role.Name),
			)
		}

		if len(elevationRequest.Workflow) > 0 {
			updates = append(updates,
				models.TypedSearchAttributeWorkflow.ValueSet(elevationRequest.Workflow),
			)
		}

		if len(elevationRequest.Providers) > 0 {
			updates = append(updates,
				models.TypedSearchAttributeProviders.ValueSet(elevationRequest.Providers),
			)
		}

		if len(elevationRequest.Identities) > 0 {
			updates = append(updates,
				models.TypedSearchAttributeIdentities.ValueSet(elevationRequest.Identities),
			)
		}
	}

	log.WithFields(logrus.Fields{
		"workflowID": elevateWorkflowTask.GetWorkflowID(),
	}).Info("Updating temporal search attributes")

	return workflow.UpsertTypedSearchAttributes(ctx, updates...)

}

func (r *thandRunner) PostStateTransitionHook(workflowTask sdkWorkflowsModel.WorkflowTaskSupport) error {
	// No-op by default
	return nil
}
