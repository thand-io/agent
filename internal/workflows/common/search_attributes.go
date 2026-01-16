package common

import (
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
	"go.temporal.io/sdk/temporal"
	"go.temporal.io/sdk/workflow"
)

func UpdateSearchAttributes(
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
