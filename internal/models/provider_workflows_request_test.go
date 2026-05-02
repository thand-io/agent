package models_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
)

type authorizeRoleRequestConfigStub struct {
	*config.Config
	identity              *models.Identity
	identityErr           error
	lastCompositeIdentity *models.Identity
}

func (s *authorizeRoleRequestConfigStub) GetIdentity(byEmail string) (*models.Identity, error) {
	if s.identityErr != nil {
		return nil, s.identityErr
	}
	return s.identity, nil
}

func (s *authorizeRoleRequestConfigStub) GetCompositeRoleForWorkflow(
	identity *models.Identity,
	baseRole *models.Role,
	workflowID string,
	providers ...models.Provider,
) (*models.CompositeRole, error) {
	s.lastCompositeIdentity = identity
	return &models.CompositeRole{
		Role:      *baseRole,
		Composite: false,
	}, nil
}

type authorizeRoleRequestProvider struct {
	*models.BaseProvider
}

func (p *authorizeRoleRequestProvider) ValidateRole(
	ctx context.Context,
	user *models.Identity,
	role *models.Role,
) (map[string]any, error) {
	return map[string]any{}, nil
}

func newAuthorizeRoleRequestProvider() models.Provider {
	return &authorizeRoleRequestProvider{
		BaseProvider: models.NewBaseProvider(
			"test-provider",
			models.ProviderConfig{
				Name:     "Test Provider",
				Provider: "test",
			},
			models.NewProviderCapabilities(),
		),
	}
}

func newAuthorizeRoleRequestRole() *models.Role {
	return &models.Role{
		Name: "local_sudo",
	}
}

func TestCreateAuthorizeRoleRequestUsesResolvedIdentitySnapshot(t *testing.T) {
	cfg := &authorizeRoleRequestConfigStub{
		Config:      config.DefaultConfig(),
		identityErr: errors.New("identity lookup should not be used"),
	}
	provider := newAuthorizeRoleRequestProvider()
	role := newAuthorizeRoleRequestRole()
	resolvedIdentity := &models.Identity{
		ID: "identity-abc",
		User: &models.User{
			Email:    "user@example.com",
			Username: "example-user",
			Name:     "Example User",
		},
	}

	req, err := models.CreateAuthorizeRoleRequest(cfg, provider, &models.WorkflowRoleRequest{
		WorkflowID:       "wf-123",
		Identity:         "opaque-identity-id",
		ResolvedIdentity: resolvedIdentity,
		Role:             role,
	})

	require.NoError(t, err)
	require.NotNil(t, req)
	assert.Same(t, resolvedIdentity, req.Identity)
	assert.Same(t, resolvedIdentity, cfg.lastCompositeIdentity)
}

func TestCreateAuthorizeRoleRequestFallsBackToConfigLookup(t *testing.T) {
	lookedUpIdentity := &models.Identity{
		ID: "identity-def",
		User: &models.User{
			Email:    "resolved@example.com",
			Username: "resolved-user",
		},
	}
	cfg := &authorizeRoleRequestConfigStub{
		Config:   config.DefaultConfig(),
		identity: lookedUpIdentity,
	}
	provider := newAuthorizeRoleRequestProvider()
	role := newAuthorizeRoleRequestRole()

	req, err := models.CreateAuthorizeRoleRequest(cfg, provider, &models.WorkflowRoleRequest{
		WorkflowID: "wf-456",
		Identity:   "resolved@example.com",
		Role:       role,
	})

	require.NoError(t, err)
	require.NotNil(t, req)
	assert.Same(t, lookedUpIdentity, req.Identity)
	assert.Same(t, lookedUpIdentity, cfg.lastCompositeIdentity)
}

func TestCreateAuthorizeRoleRequestUsesSyntheticFallbackWhenLookupFails(t *testing.T) {
	cfg := &authorizeRoleRequestConfigStub{
		Config:      config.DefaultConfig(),
		identityErr: errors.New("identity not found"),
	}
	provider := newAuthorizeRoleRequestProvider()
	role := newAuthorizeRoleRequestRole()

	req, err := models.CreateAuthorizeRoleRequest(cfg, provider, &models.WorkflowRoleRequest{
		WorkflowID: "wf-789",
		Identity:   "opaque-identity-id",
		Role:       role,
	})

	require.NoError(t, err)
	require.NotNil(t, req)
	require.NotNil(t, req.Identity)
	require.NotNil(t, req.Identity.User)
	assert.Equal(t, "opaque-identity-id", req.Identity.ID)
	assert.Equal(t, "opaque-identity-id", req.Identity.User.Email)
	assert.Same(t, req.Identity, cfg.lastCompositeIdentity)
}

func TestCreateChildWorkflowIDIncludesDeviceID(t *testing.T) {
	childIDWithDevice := models.CreateChildWorkflowID("parent-wf", "authorizeRole", "test-provider", &models.WorkflowRoleRequest{
		Identity: "user@example.com",
		DeviceID: "device-alpha",
	})
	childIDWithoutDevice := models.CreateChildWorkflowID("parent-wf", "authorizeRole", "test-provider", &models.WorkflowRoleRequest{
		Identity: "user@example.com",
	})

	assert.NotEqual(t, childIDWithoutDevice, childIDWithDevice)
}
