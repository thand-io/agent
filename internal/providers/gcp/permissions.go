package gcp

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

// Never synchronize permissions from GCP as they are
// statically defined by GCP and cannot be modified
func (p *gcpProvider) CanSynchronizePermissions() bool {
	return false
}

func loadPermissions(stage string) ([]models.ProviderPermission, error) {
	var permissionMap gcpPermissionMap

	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Parsed GCP permissions in %s", elapsed)
	}()

	// Load GCP Permissions
	if err := json.Unmarshal(GetGcpPermissions(), &permissionMap); err != nil {
		return nil, fmt.Errorf("failed to unmarshal GCP permissions: %w", err)
	}

	var permissions = make([]models.ProviderPermission, 0, len(permissionMap))

	if len(stage) == 0 {
		stage = DefaultStage
	}

	for _, perm := range permissionMap {

		if perm.OnlyInPredefinedRoles {
			continue
		}

		if !strings.EqualFold(perm.Stage, stage) {
			continue
		}

		permission := models.ProviderPermission{
			ID:          strings.ToLower(fmt.Sprintf("%s-%s", perm.Stage, perm.Name)),
			Name:        perm.Name,
			Title:       perm.Title,
			Description: perm.Description,
		}

		permissions = append(permissions, permission)
	}

	return permissions, nil
}
