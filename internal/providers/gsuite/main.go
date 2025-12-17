package gsuite

import (
	"context"
	"fmt"

	admin "google.golang.org/api/admin/directory/v1"
	"google.golang.org/api/option"

	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
	"github.com/thand-io/agent/internal/providers/gcp"
)

const GsuiteProviderName = "gsuite"

// gsuiteProvider implements the ProviderImpl interface for Google Workspace (GSuite)
type gsuiteProvider struct {
	*models.BaseProvider

	adminService *admin.Service
	domain       string
	adminEmail   string
}

func (p *gsuiteProvider) Initialize(identifier string, provider models.Provider) error {
	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		models.ProviderCapabilityIdentities,
	)

	// Disable unsupported capabilities
	p.DisableCapability(models.ProviderCapabilitySynchronizeIdentities)

	// Get configuration
	config := p.GetConfig()

	// Get required configuration values - service account key path will be handled by GCP config
	_, foundKeyPath := config.GetString("service_account_key_path")
	if !foundKeyPath {
		return fmt.Errorf("service_account_key_path is required for GSuite provider")
	}

	domain, foundDomain := config.GetString("domain")
	if !foundDomain {
		return fmt.Errorf("domain is required for GSuite provider")
	}

	adminEmail, foundAdminEmail := config.GetString("admin_email")
	if !foundAdminEmail {
		return fmt.Errorf("admin_email is required for GSuite provider")
	}

	p.domain = domain
	p.adminEmail = adminEmail

	// Create admin service with domain-wide delegation
	ctx := context.Background()

	// Use shared GCP configuration to handle credentials
	gcpClient, err := gcp.CreateGcpConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create GCP config: %w", err)
	}

	// Create JWT config for domain-wide delegation
	conf, err := gcpClient.CreateJWTConfig(
		admin.AdminDirectoryUserReadonlyScope,
		admin.AdminDirectoryGroupReadonlyScope,
	)
	if err != nil {
		return fmt.Errorf("failed to create JWT config: %w", err)
	}

	// Set the subject (admin email) for domain-wide delegation
	conf.Subject = adminEmail

	// Create the admin service with the JWT config
	adminService, err := admin.NewService(ctx, option.WithTokenSource(conf.TokenSource(ctx)))
	if err != nil {
		return fmt.Errorf("failed to create admin service: %w", err)
	}

	p.adminService = adminService

	return nil
}

func init() {
	providers.Register(GsuiteProviderName, &gsuiteProvider{})
}
