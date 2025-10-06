package manager

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	models "github.com/thand-io/agent/internal/models"
)

func CreateWorkflowFromEncodedTask(encryption models.EncryptionImpl, encodedTask string) (*models.WorkflowTask, error) {
	// Tasks may contain sensitive information, ensure encryption is used
	decodedTask, err := models.EncodingWrapper{}.DecodeAndDecrypt(encodedTask, encryption)

	if err != nil {
		return nil, fmt.Errorf("failed to decode workflow state: %w", err)
	}

	if decodedTask.Type != models.ENCODED_WORKFLOW_TASK {
		return nil, fmt.Errorf("invalid workflow state type: %s", decodedTask.Type)
	}

	var result models.WorkflowTask
	common.ConvertMapToInterface(decodedTask.Data.(map[string]any), &result)

	return &result, nil
}

// Hydrate populates the workflow task with necessary data
func (m *WorkflowManager) Hydrate(workflowTask *models.WorkflowTask) error {

	if workflowTask.GetWorkflowDef() == nil {

		elevationRequest, err := workflowTask.GetContextAsElevationRequest()

		if err != nil {
			return fmt.Errorf("failed to get context as ElevateRequestInternal: %w", err)
		}

		if !elevationRequest.IsValid() {
			return fmt.Errorf("invalid elevation request")
		}

		workflowDsl, err := m.config.GetWorkflowByName(elevationRequest.Role.Workflow)

		if err != nil {
			return fmt.Errorf("failed to load workflow: %w", err)
		}

		workflowTask.SetWorkflowDsl(workflowDsl.GetWorkflow())

	}

	// Create a new task state if it does not exist
	// This is important as we might be in the middle of a workflow and
	// the state might not have been initialised yet
	if !workflowTask.HasState() {
		workflowTask.ClearTaskContext()
	}

	return nil
}
