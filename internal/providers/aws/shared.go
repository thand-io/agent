package aws

import (
	"sync"

	"github.com/thand-io/agent/internal/models"
)

type awsData struct {
	permissions []models.ProviderPermission
	roles       []models.ProviderRole
}

var (
	sharedData     *awsData
	sharedDataOnce sync.Once
	sharedDataErr  error
)

func getSharedData() (*awsData, error) {
	sharedDataOnce.Do(func() {
		sharedData = &awsData{}
		var err error

		sharedData.permissions, err = loadPermissions()
		if err != nil {
			sharedDataErr = err
			return
		}

		sharedData.roles, err = loadRoles()
		if err != nil {
			sharedDataErr = err
			return
		}
	})
	return sharedData, sharedDataErr
}
