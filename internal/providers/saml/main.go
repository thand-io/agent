package saml

import (
	"context"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/crewjam/saml"
	"github.com/crewjam/saml/samlsp"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
)

// samlProvider implements the ProviderImpl interface for SAML
type samlProvider struct {
	*models.BaseProvider
	middleware   *samlsp.Middleware
	idpMetadata  *saml.EntityDescriptor
	certificates []tls.Certificate
}

// SAMLConfig represents the SAML provider configuration
type SAMLConfig struct {
	IDPMetadataURL string `yaml:"idp_metadata_url" json:"idp_metadata_url"`
	EntityID       string `yaml:"entity_id" json:"entity_id"`
	RootURL        string `yaml:"root_url" json:"root_url"`
	CertFile       string `yaml:"cert_file" json:"cert_file"`
	KeyFile        string `yaml:"key_file" json:"key_file"`
	SignRequests   bool   `yaml:"sign_requests" json:"sign_requests"`
}

func (p *samlProvider) Initialize(provider models.Provider) error {
	p.BaseProvider = models.NewBaseProvider(
		provider,
		models.ProviderCapabilityAuthorizor,
	)

	// Parse SAML configuration from provider config
	config, err := p.parseSAMLConfig(provider.Config)
	if err != nil {
		return fmt.Errorf("failed to parse SAML config: %w", err)
	}

	// Load certificate and key for SAML signing
	keyPair, err := tls.LoadX509KeyPair(config.CertFile, config.KeyFile)
	if err != nil {
		return fmt.Errorf("failed to load SAML certificate: %w", err)
	}

	// Parse the certificate
	keyPair.Leaf, err = x509.ParseCertificate(keyPair.Certificate[0])
	if err != nil {
		return fmt.Errorf("failed to parse SAML certificate: %w", err)
	}

	// Fetch IdP metadata
	idpMetadataURL, err := url.Parse(config.IDPMetadataURL)
	if err != nil {
		return fmt.Errorf("invalid IdP metadata URL: %w", err)
	}

	idpMetadata, err := samlsp.FetchMetadata(context.Background(), http.DefaultClient, *idpMetadataURL)
	if err != nil {
		return fmt.Errorf("failed to fetch IdP metadata: %w", err)
	}

	// Parse root URL
	rootURL, err := url.Parse(config.RootURL)
	if err != nil {
		return fmt.Errorf("invalid root URL: %w", err)
	}

	// Create SAML service provider
	samlSP, err := samlsp.New(samlsp.Options{
		URL:         *rootURL,
		Key:         keyPair.PrivateKey.(*rsa.PrivateKey),
		Certificate: keyPair.Leaf,
		IDPMetadata: idpMetadata,
		EntityID:    config.EntityID,
		SignRequest: config.SignRequests,
	})
	if err != nil {
		return fmt.Errorf("failed to create SAML service provider: %w", err)
	}

	p.middleware = samlSP
	p.idpMetadata = idpMetadata
	p.certificates = []tls.Certificate{keyPair}

	logrus.Infof("SAML provider %s initialized successfully", provider.Name)
	return nil
}

func (p *samlProvider) AuthorizeSession(ctx context.Context, authRequest *models.AuthorizeUser) (*models.AuthorizeSessionResponse, error) {
	if p.middleware == nil {
		return nil, fmt.Errorf("SAML provider not initialized")
	}

	// Generate a SAML authentication request URL using the correct API
	// Use MakeRedirectAuthenticationRequest for redirect binding
	authURL, err := p.middleware.ServiceProvider.MakeRedirectAuthenticationRequest(authRequest.State)
	if err != nil {
		return nil, fmt.Errorf("failed to create SAML authentication request: %w", err)
	}

	return &models.AuthorizeSessionResponse{
		Url: authURL.String(),
	}, nil
}

func (p *samlProvider) CreateSession(ctx context.Context, authRequest *models.AuthorizeUser) (*models.Session, error) {
	if p.middleware == nil {
		return nil, fmt.Errorf("SAML provider not initialized")
	}

	// In a real implementation, this would parse the SAML response from the authorization code
	// For now, we'll create a basic session structure
	// The authRequest.Code should contain the SAML response or a reference to it

	if len(authRequest.Code) == 0 {
		return nil, fmt.Errorf("no SAML response code provided")
	}

	// Extract user information from SAML assertion
	// This is a simplified implementation - in practice you'd parse the actual SAML response
	user := &models.User{
		Username: "saml_user",        // Extract from SAML assertion
		Email:    "user@example.com", // Extract from SAML assertion
		Provider: "saml",
		Groups:   []string{}, // Extract groups from SAML assertion
	}

	// Create session
	session := &models.Session{
		UUID:         uuid.New(),
		User:         user,
		AccessToken:  uuid.New().String(), // Generate or extract from SAML
		RefreshToken: uuid.New().String(),
		Expiry:       time.Now().Add(24 * time.Hour), // Configurable session duration
	}

	logrus.Infof("Created SAML session for user: %s", user.Username)
	return session, nil
}

func (p *samlProvider) ValidateSession(ctx context.Context, session *models.Session) error {
	if session == nil {
		return fmt.Errorf("session is nil")
	}

	// Check if session has expired
	if time.Now().After(session.Expiry) {
		return fmt.Errorf("session has expired")
	}

	// Validate access token (in a real implementation, you might validate against IdP)
	if len(session.AccessToken) == 0 {
		return fmt.Errorf("invalid access token")
	}

	// Validate user information
	if session.User == nil {
		return fmt.Errorf("session user is nil")
	}

	logrus.Debugf("SAML session validated for user: %s", session.User.Username)
	return nil
}

func (p *samlProvider) RenewSession(ctx context.Context, session *models.Session) (*models.Session, error) {
	if session == nil {
		return nil, fmt.Errorf("session is nil")
	}

	// Validate current session first
	if err := p.ValidateSession(ctx, session); err != nil {
		// If session is expired, we need a new authentication
		return nil, fmt.Errorf("cannot renew expired session: %w", err)
	}

	// Create a new session with extended expiry
	newSession := &models.Session{
		UUID:         uuid.New(),
		User:         session.User,
		AccessToken:  uuid.New().String(),
		RefreshToken: uuid.New().String(),
		Expiry:       time.Now().Add(24 * time.Hour),
	}

	logrus.Infof("Renewed SAML session for user: %s", session.User.Username)
	return newSession, nil
}

// Authorize grants access for a user to a role
func (p *samlProvider) AuthorizeRole(ctx context.Context, user *models.User, role *models.Role) (map[string]any, error) {
	if user == nil {
		return nil, fmt.Errorf("user is nil")
	}
	if role == nil {
		return nil, fmt.Errorf("role is nil")
	}

	// Check if user has permission for the role
	if !role.HasPermission(user) {
		return nil, fmt.Errorf("user %s does not have permission for role %s", user.Username, role.Name)
	}

	logrus.Infof("SAML authorization granted for user %s to role %s", user.Username, role.Name)
	return nil, nil
}

// Revoke removes access for a user from a role
func (p *samlProvider) RevokeRole(ctx context.Context, user *models.User, role *models.Role) (map[string]any, error) {
	if user == nil {
		return nil, fmt.Errorf("user is nil")
	}
	if role == nil {
		return nil, fmt.Errorf("role is nil")
	}

	// In SAML, revocation typically happens at the IdP level
	// This is a placeholder for any local cleanup
	logrus.Infof("SAML access revoked for user %s from role %s", user.Username, role.Name)
	return nil, nil
}

func (p *samlProvider) GetPermission(ctx context.Context, permission string) (*models.ProviderPermission, error) {
	// SAML permissions are typically defined at the IdP level
	// This would require integration with the IdP's permission system
	return nil, fmt.Errorf("GetPermission not implemented for SAML provider - permissions managed at IdP level")
}

func (p *samlProvider) ListPermissions(ctx context.Context, filters ...string) ([]models.ProviderPermission, error) {
	// SAML permissions are typically defined at the IdP level
	return []models.ProviderPermission{}, nil
}

func (p *samlProvider) GetRole(ctx context.Context, role string) (*models.ProviderRole, error) {
	// SAML roles are typically defined at the IdP level
	// This would require integration with the IdP's role system
	return nil, fmt.Errorf("GetRole not implemented for SAML provider - roles managed at IdP level")
}

func (p *samlProvider) ListRoles(ctx context.Context, filters ...string) ([]models.ProviderRole, error) {
	// SAML roles are typically defined at the IdP level
	return []models.ProviderRole{}, nil
}

// GetResource is required by ProviderRoleBasedAccessControl interface
func (p *samlProvider) GetResource(ctx context.Context, resource string) (*models.ProviderResource, error) {
	return nil, fmt.Errorf("GetResource not implemented for SAML provider")
}

// ListResources is required by ProviderRoleBasedAccessControl interface
func (p *samlProvider) ListResources(ctx context.Context, filters ...string) ([]models.ProviderResource, error) {
	return []models.ProviderResource{}, nil
}

// ValidateRole is required by ProviderRoleBasedAccessControl interface
func (p *samlProvider) ValidateRole(
	ctx context.Context, user *models.User, role *models.Role) (map[string]any, error) {
	if user == nil || role == nil {
		return nil, fmt.Errorf("user or role is nil")
	}
	return nil, nil
}

// SendNotification is required by ProviderNotifier interface
func (p *samlProvider) SendNotification(ctx context.Context, notification models.NotificationRequest) error {
	return fmt.Errorf("SendNotification not implemented for SAML provider")
}

// parseSAMLConfig parses the SAML configuration from the provider config
func (p *samlProvider) parseSAMLConfig(config *models.BasicConfig) (*SAMLConfig, error) {
	if config == nil {
		return nil, fmt.Errorf("config is nil")
	}

	samlConfig := &SAMLConfig{}

	// Parse required fields
	if idpURL, ok := (*config)["idp_metadata_url"].(string); ok {
		samlConfig.IDPMetadataURL = idpURL
	} else {
		return nil, fmt.Errorf("idp_metadata_url is required")
	}

	if entityID, ok := (*config)["entity_id"].(string); ok {
		samlConfig.EntityID = entityID
	} else {
		return nil, fmt.Errorf("entity_id is required")
	}

	if rootURL, ok := (*config)["root_url"].(string); ok {
		samlConfig.RootURL = rootURL
	} else {
		return nil, fmt.Errorf("root_url is required")
	}

	if certFile, ok := (*config)["cert_file"].(string); ok {
		samlConfig.CertFile = certFile
	} else {
		return nil, fmt.Errorf("cert_file is required")
	}

	if keyFile, ok := (*config)["key_file"].(string); ok {
		samlConfig.KeyFile = keyFile
	} else {
		return nil, fmt.Errorf("key_file is required")
	}

	// Parse optional fields
	if signRequests, ok := (*config)["sign_requests"].(bool); ok {
		samlConfig.SignRequests = signRequests
	} else {
		samlConfig.SignRequests = false // Default to false
	}

	return samlConfig, nil
}

func init() {
	providers.Register("saml", &samlProvider{})
}
