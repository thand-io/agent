package example

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
)

const ExampleProviderName = "example"

var ExampleSession = models.Session{
	UUID:         uuid.New(),
	User:         &models.User{},
	AccessToken:  "",
	RefreshToken: "",
	Expiry:       time.Now(),
}

var ExamplePermission = models.ProviderPermission{
	ID:          "1",
	Name:        "example",
	Title:       "Example",
	Description: "Example Permission",
}

var ExampleRole = models.ProviderRole{
	ID:          "1",
	Name:        "example",
	Title:       "Example",
	Description: "Example Role",
}

// exampleProvider implements the ProviderImpl interface for Example
type exampleProvider struct {
	*models.BaseProvider
}

func (p *exampleProvider) Initialize(identifier string, provider models.ProviderConfig) error {
	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		ExampleCapabilities,
	)
	return nil
}

// ValidateConfig validates the Example configuration without initialization
func (p *exampleProvider) ValidateConfig(config *models.BasicConfig) error {
	schema := &ConfigSchema{}
	if err := schema.Unmarshal(config); err != nil {
		return fmt.Errorf("failed to unmarshal Example config: %w", err)
	}
	return schema.Validate()
}

// GetConfigSchema returns the Example configuration schema
func (p *exampleProvider) GetConfigSchema() any {
	return &ConfigSchema{}
}

// Validate validates the Example provider configuration
func (p *exampleProvider) Validate() error {
	return p.ValidateConfig(p.GetConfig())
}

func (p *exampleProvider) AuthorizeSession(ctx context.Context, authRequest *models.AuthorizeUser) (*models.AuthorizeSessionResponse, error) {
	return &models.AuthorizeSessionResponse{
		Url: "",
	}, nil
}

func (p *exampleProvider) CreateSession(ctx context.Context, authRequest *models.AuthorizeUser) (*models.Session, error) {
	return &ExampleSession, nil
}

func (p *exampleProvider) ValidateSession(ctx context.Context, session *models.Session) error {
	return nil
}

func (p *exampleProvider) RenewSession(ctx context.Context, session *models.Session) (*models.Session, error) {
	return &ExampleSession, nil
}

// Authorize grants access for a user to a role
func (p *exampleProvider) AuthorizeRole(
	ctx context.Context,
	req *models.AuthorizeRoleRequest,
) (*models.AuthorizeRoleResponse, error) {
	return &models.AuthorizeRoleResponse{}, nil
}

// Revoke removes access for a user from a role
func (p *exampleProvider) RevokeRole(
	ctx context.Context,
	req *models.RevokeRoleRequest,
) (*models.RevokeRoleResponse, error) {
	return &models.RevokeRoleResponse{}, nil
}

func (p *exampleProvider) GetPermission(ctx context.Context, permission string) (*models.ProviderPermission, error) {
	return &ExamplePermission, nil
}

func (p *exampleProvider) ListPermissions(ctx context.Context, searchRequest *models.SearchRequest) ([]models.SearchResult[models.ProviderPermission], error) {
	return []models.SearchResult[models.ProviderPermission]{
		{
			Result: ExamplePermission,
		},
	}, nil
}

func (p *exampleProvider) GetRole(ctx context.Context, role string) (*models.ProviderRole, error) {
	return &ExampleRole, nil
}

func (p *exampleProvider) ListRoles(ctx context.Context, searchRequest *models.SearchRequest) ([]models.SearchResult[models.ProviderRole], error) {
	return []models.SearchResult[models.ProviderRole]{
		{
			Result: ExampleRole,
		},
	}, nil
}

func init() {
	providers.Register(ExampleProviderName, &exampleProvider{}, ExampleCapabilities, &ConfigSchema{})
}
