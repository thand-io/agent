
Create a new default provider

```golang

package example

import (
	"context"
	"fmt"

	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
)

// exampleProvider implements the ProviderImpl interface for Example
type exampleProvider struct {
	*models.BaseProvider
}

func (p *exampleProvider) Initialize(provider models.Provider) error {
	p.BaseProvider = models.NewBaseProvider(
		provider,
		models.ProviderCapabilityAuthorizor,
        models.ProviderCapabilityRBAC,
	)
	// TODO: Implement Example initialization logic
	return nil
}

func (p *exampleProvider) AuthorizeSession(ctx context.Context, authRequest *models.AuthorizeUser) (string, error) {
	// TODO: Implement Example user authorization logic
	return "", nil
}

func (p *exampleProvider) CreateSession(ctx context.Context, authRequest *models.AuthorizeUser) (*models.Session, error) {
	// TODO: Implement Example session creation logic
	return nil, fmt.Errorf("CreateSession not implemented for Example provider")
}

func (p *exampleProvider) ValidateSession(ctx context.Context, session *models.Session) error {
	// TODO: Implement Example session validation logic
	return fmt.Errorf("ValidateSession not implemented for Example provider")
}

func (p *exampleProvider) RenewSession(ctx context.Context, session *models.Session) (*models.Session, error) {
	// TODO: Implement Example session renewal logic
	return nil, fmt.Errorf("RenewSession not implemented for Example provider")
}

// Authorize grants access for a user to a role
func (p *exampleProvider) AuthorizeRole(ctx context.Context, user *models.User, role *models.Role) (map[string]any, error) {
	// TODO: Implement Example authorization logic
	return nil
}

// Revoke removes access for a user from a role
func (p *exampleProvider) RevokeRole(ctx context.Context, user *models.User, role *models.Role, metadata map[string]any) (map[string]any, error) {
	// TODO: Implement Example revocation logic
	return nil, nil
}

func (p *exampleProvider) GetPermission(ctx context.Context, permission string) (*models.ProviderPermission, error) {
	// TODO: Implement Example GetPermission logic
	return nil, fmt.Errorf("GetPermission not implemented for Example provider")
}

func (p *exampleProvider) ListPermissions(ctx context.Context, filters ...string) ([]models.ProviderPermission, error) {
	// TODO: Implement Example ListPermissions logic
	return nil, fmt.Errorf("ListPermissions not implemented for Example provider")
}

func (p *exampleProvider) GetRole(ctx context.Context, role string) (*models.ProviderRole, error) {
	// TODO: Implement Example GetRole logic
	return nil, fmt.Errorf("GetRole not implemented for Example provider")
}

func (p *exampleProvider) ListRoles(ctx context.Context, filters ...string) ([]models.ProviderRole, error) {
	// TODO: Implement Example ListRoles logic
	return nil, fmt.Errorf("ListRoles not implemented for Example provider")
}

func init() {
	providers.Register("example", &exampleProvider{})
}


```