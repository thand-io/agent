package gcp

import (
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/data"
	"github.com/thand-io/agent/internal/models"
)

// Never synchronize roles from GCP as they are
// statically defined by GCP and cannot be modified
func (p *gcpProvider) CanSynchronizeRoles() bool {
	return false
}

func loadRoles(stage string) ([]models.ProviderRole, error) {

	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Parsed GCP roles in %s", elapsed)
	}()

	// Get pre-parsed GCP roles from data package
	predefinedRoles, err := data.GetParsedGcpRoles()
	if err != nil {
		return nil, fmt.Errorf("failed to get parsed GCP roles: %w", err)
	}

	var roles = make([]models.ProviderRole, 0, len(predefinedRoles))

	if len(stage) == 0 {
		stage = DefaultStage
	}

	for _, gcpRole := range predefinedRoles {

		if !strings.EqualFold(gcpRole.Stage, stage) {
			continue
		}

		role := models.ProviderRole{
			Name:        gcpRole.Name,
			Title:       gcpRole.Title,
			Description: gcpRole.Description,
		}
		roles = append(roles, role)
	}

	return roles, nil
}
