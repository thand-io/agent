package aws

import (
	aws "github.com/thand-io/agent/internal/providers/aws"
	"github.com/thand-io/agent/sdk/models"
)

func GetRoles() ([]models.ProviderRole, error) {
	return aws.GetRoles()
}

func GetPermissions() ([]models.ProviderPermission, error) {
	return aws.GetPermissions()
}
