package manager

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	models "github.com/thand-io/agent/internal/models"
)

func CreateWorkflowFromEncodedTask(
	encryption models.EncryptionImpl,
	encodedTask string,
) (*models.ThandWorkflowTask, error) {

	// Tasks may contain sensitive information, ensure encryption is used
	decodedTask, err := models.EncodingWrapper{}.DecodeAndDecrypt(encodedTask, encryption)

	if err != nil {
		return nil, fmt.Errorf("failed to decode workflow state: %w", err)
	}

	if decodedTask.Type != models.ENCODED_WORKFLOW_TASK {
		return nil, fmt.Errorf("invalid workflow state type: %s", decodedTask.Type)
	}

	var result models.ThandWorkflowTask
	common.ConvertMapToInterface(decodedTask.Data.(map[string]any), &result)

	return &result, nil
}
