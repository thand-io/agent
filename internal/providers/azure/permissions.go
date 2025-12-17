package azure

import (
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/data"
	"github.com/thand-io/agent/internal/models"
)

func (p *azureProvider) CanSynchronizePermissions() bool {
	return true
}

func loadPermissions() ([]models.ProviderPermission, error) {

	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Parsed Azure permissions in %s", elapsed)
	}()

	// Get pre-parsed Azure permissions from data package
	azureOperations, err := data.GetParsedAzurePermissions()
	if err != nil {
		return nil, fmt.Errorf("failed to get parsed Azure permissions: %w", err)
	}

	var permissions []models.ProviderPermission

	for _, operation := range azureOperations {
		permission := models.ProviderPermission{
			ID:          strings.ToLower(operation.Name),
			Name:        operation.Name,
			Description: operation.Description,
		}
		permissions = append(permissions, permission)
	}

	return permissions, nil
}
