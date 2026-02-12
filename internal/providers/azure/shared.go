package azure

import (
	"sync"

	"github.com/thand-io/agent/internal/models"
)

type azureData struct {
	permissions []models.ProviderPermission
	roles       []models.ProviderRole

	indexReady chan struct{}
}

var (
	sharedData     *azureData
	sharedDataOnce sync.Once
	sharedDataErr  error
)

func getSharedData() (*azureData, error) {
	sharedDataOnce.Do(func() {
		sharedData = &azureData{
			indexReady: make(chan struct{}),
		}
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

func GetRoles() ([]models.ProviderRole, error) {
	data, err := getSharedData()
	if err != nil {
		return nil, err
	}
	return data.roles, nil
}

func GetPermissions() ([]models.ProviderPermission, error) {
	data, err := getSharedData()
	if err != nil {
		return nil, err
	}
	return data.permissions, nil
}
