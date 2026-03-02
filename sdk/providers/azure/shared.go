package azure

import (
	azure "github.com/thand-io/agent/internal/providers/azure"
	"github.com/thand-io/agent/sdk/models"
)

func GetRoles() ([]models.ProviderRole, error) {
	return azure.GetRoles()
}

func GetPermissions() ([]models.ProviderPermission, error) {
	return azure.GetPermissions()
}
