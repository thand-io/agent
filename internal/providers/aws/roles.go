package aws

import (
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/data"
	"github.com/thand-io/agent/internal/models"
)

// Never synchronize roles from AWS as they are
// statically defined by AWS and cannot be modified
func (a *awsProvider) CanSynchronizeRoles() bool {
	return false
}

func loadRoles() ([]models.ProviderRole, error) {

	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Parsed AWS roles in %s", elapsed)
	}()

	// Get pre-parsed AWS roles from data package
	docs, err := data.GetParsedAwsRoles()
	if err != nil {
		return nil, fmt.Errorf("failed to get parsed AWS roles: %w", err)
	}

	var roles []models.ProviderRole

	// Convert to slice and create fast lookup map
	for _, policy := range docs.Policies {
		role := models.ProviderRole{
			Name: policy.Name,
		}
		roles = append(roles, role)
	}

	return roles, nil
}
