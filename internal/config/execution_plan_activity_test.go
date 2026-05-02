package config

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/models"
)

type executionPlanActivityTestProvider struct {
	*models.BaseProvider
}

func (p *executionPlanActivityTestProvider) ValidateRole(
	ctx context.Context,
	user *models.Identity,
	role *models.Role,
) (map[string]any, error) {
	return map[string]any{}, nil
}

func newExecutionPlanActivityTestProvider(identifier string) *executionPlanActivityTestProvider {
	caps := models.NewProviderCapabilities().WithDefaultProvisioningConfiguration()
	providerCfg := models.ProviderConfig{
		Name:         identifier,
		Provider:     identifier,
		Enabled:      true,
		Capabilities: caps,
		Config:       &models.BasicConfig{},
	}

	provider := &executionPlanActivityTestProvider{
		BaseProvider: models.NewBaseProvider(identifier, providerCfg, caps),
	}
	provider.SetReady()
	return provider
}

func newExecutionPlanActivityRole(identifier, name string) *models.Role {
	return &models.Role{
		Identifier: identifier,
		Name:       name,
		Enabled:    true,
		Permissions: models.RolePermissions{
			Allow: models.RoleStatements{{Operations: []string{"local:test"}}},
		},
	}
}

func TestBuildExecutionPlanActivityUsesSharedDeviceDefinitionsForLocalSudo(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.AddProvider("local-elevation", newExecutionPlanActivityTestProvider("local"))

	activities := &thandActivities{
		config: cfg,
		lookupDeviceDefinition: func(ctx context.Context, deviceID string) (*models.Device, error) {
			return &models.Device{
				ID:      deviceID,
				Name:    "Device Alpha",
				Enabled: true,
				LocalElevation: &models.DeviceLocalElevationPolicy{
					Enabled:      true,
					AllowedModes: []string{string(models.LocalSudoModeTimed)},
					Accounts: []models.DeviceLocalElevationAccount{
						{Email: "user@example.com", LocalUsername: "workstation-user"},
					},
					DeniedUsernames:  []string{"root"},
					AllowedUIDRanges: []string{"1000-60000"},
				},
			}, nil
		},
	}

	plan, err := activities.BuildExecutionPlan(context.Background(), models.ExecutionPlanRequest{
		WorkflowID: "wf-local-sudo",
		ElevateRequest: &models.ElevateRequestInternal{
			ElevateRequest: models.ElevateRequest{
				Role:       newExecutionPlanActivityRole(models.LocalSudoRoleIdentifier, "Local Sudo"),
				Providers:  []string{"local-elevation"},
				Workflow:   models.LocalSudoTimedWorkflowName,
				Device:     "device-alpha",
				Reason:     "maintenance",
				Duration:   "30m",
				Identities: []string{"user@example.com"},
				Metadata: models.LocalSudoRequestMetadata{
					Mode: models.LocalSudoModeTimed,
				}.AsMap(),
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, plan.Entries, 1)

	meta, err := models.DecodeLocalSudoRequestMetadata(plan.Entries[0].AuthorizeRequest.Metadata)
	require.NoError(t, err)
	assert.Equal(t, "device-alpha", plan.Entries[0].DeviceID)
	assert.Equal(t, "workstation-user", meta.LocalUsername)
	assert.Equal(t, []string{"root"}, meta.DeniedUsernames)
}

func TestBuildExecutionPlanActivityFailsWhenSharedDeviceDefinitionIsMissing(t *testing.T) {
	t.Parallel()

	cfg := DefaultConfig()
	cfg.AddProvider("local-elevation", newExecutionPlanActivityTestProvider("local"))

	activities := &thandActivities{
		config: cfg,
		lookupDeviceDefinition: func(ctx context.Context, deviceID string) (*models.Device, error) {
			return nil, assert.AnError
		},
	}

	_, err := activities.BuildExecutionPlan(context.Background(), models.ExecutionPlanRequest{
		WorkflowID: "wf-local-sudo",
		ElevateRequest: &models.ElevateRequestInternal{
			ElevateRequest: models.ElevateRequest{
				Role:       newExecutionPlanActivityRole(models.LocalSudoRoleIdentifier, "Local Sudo"),
				Providers:  []string{"local-elevation"},
				Workflow:   models.LocalSudoTimedWorkflowName,
				Device:     "device-alpha",
				Reason:     "maintenance",
				Duration:   "30m",
				Identities: []string{"user@example.com"},
				Metadata: models.LocalSudoRequestMetadata{
					Mode: models.LocalSudoModeTimed,
				}.AsMap(),
			},
		},
	})
	require.Error(t, err)
	assert.ErrorContains(t, err, "assert.AnError general error for testing")
}
