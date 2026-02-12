package gcp

import (
	gcp "github.com/thand-io/agent/internal/providers/gcp"
	"github.com/thand-io/agent/sdk/models"
)

func GetRoles(stage string) ([]models.ProviderRole, error) {
	return gcp.GetRoles(stage)
}

func GetPermissions(stage string) ([]models.ProviderPermission, error) {
	return gcp.GetPermissions(stage)
}
