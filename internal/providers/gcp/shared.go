package gcp

import (
	"sync"

	_ "embed"

	"github.com/thand-io/agent/internal/models"
)

//go:embed permissions.json
var gcpPermissions []byte

func GetGcpPermissions() []byte {
	return gcpPermissions
}

type gcpData struct {
	permissions []models.ProviderPermission
	roles       []models.ProviderRole
}

type gcpSingleton struct {
	data *gcpData
	err  error
	once sync.Once
}

var (
	sharedDataMap = make(map[string]*gcpSingleton)
	sharedDataMu  sync.Mutex
)

func getSharedData(stage string) (*gcpData, error) {
	sharedDataMu.Lock()
	singleton, ok := sharedDataMap[stage]
	if !ok {
		singleton = &gcpSingleton{}
		sharedDataMap[stage] = singleton
	}
	sharedDataMu.Unlock()

	singleton.once.Do(func() {
		data := &gcpData{}
		var err error

		data.permissions, err = loadPermissions(stage)
		if err != nil {
			singleton.err = err
			return
		}

		data.roles, err = loadRoles(stage)
		if err != nil {
			singleton.err = err
			return
		}

		singleton.data = data
	})

	return singleton.data, singleton.err
}

type gcpPermissionMap []struct {
	ApiDisabled           bool   `json:"apiDisabled,omitempty"`
	Description           string `json:"description,omitempty"`
	Name                  string `json:"name,omitempty"`
	Stage                 string `json:"stage,omitempty"`
	Title                 string `json:"title,omitempty"`
	OnlyInPredefinedRoles bool   `json:"onlyInPredefinedRoles,omitempty"`
}
