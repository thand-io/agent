package manager

import (
	"fmt"

	"github.com/thand-io/agent/internal/common"
	models "github.com/thand-io/agent/internal/models"
	sdkConstants "github.com/thand-io/agent/sdk/constants"
)

func CreateWorkflowFromEncodedTask(
	encryption models.EncryptionImpl,
	encodedTask string,
) (*models.ElevateWorkflowTask, error) {

	// Tasks may contain sensitive information, ensure encryption is used
	decodedTask, err := models.EncodingWrapper{}.DecodeAndDecrypt(encodedTask, encryption)

	if err != nil {
		return nil, fmt.Errorf("failed to decode workflow state: %w", err)
	}

	if decodedTask.Type != sdkConstants.ENCODED_WORKFLOW_TASK {
		return nil, fmt.Errorf("invalid workflow state type: %s", decodedTask.Type)
	}

	var result models.ElevateWorkflowTask
	common.ConvertMapToInterface(decodedTask.Data.(map[string]any), &result)

	return &result, nil
}
