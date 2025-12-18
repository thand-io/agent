package aws

import (
	"fmt"
	"strings"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/data"
	"github.com/thand-io/agent/internal/models"
)

// Never synchronize permissions from AWS as they are
// statically defined by AWS and cannot be modified
func (a *awsProvider) CanSynchronizePermissions() bool {
	return false
}

func loadPermissions() ([]models.ProviderPermission, error) {

	startTime := time.Now()
	defer func() {
		elapsed := time.Since(startTime)
		logrus.Debugf("Parsed AWS permissions in %s", elapsed)
	}()

	// Get pre-parsed AWS permissions from data package
	docs, err := data.GetParsedAwsDocs()
	if err != nil {
		return nil, fmt.Errorf("failed to get parsed AWS permissions: %w", err)
	}

	var permissions []models.ProviderPermission

	// Convert to slice and create fast lookup map
	for name, description := range docs {
		perm := models.ProviderPermission{
			ID:          strings.ToLower(name),
			Name:        name,
			Description: description,
		}
		permissions = append(permissions, perm)
	}

	return permissions, nil
}
