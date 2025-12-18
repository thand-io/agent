package gcp

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"os"

	"github.com/sirupsen/logrus"
	"golang.org/x/oauth2/google"
	"golang.org/x/oauth2/jwt"

	"cloud.google.com/go/compute/metadata"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"

	"google.golang.org/api/cloudresourcemanager/v1"
	iam "google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
)

const GcpProviderName = "gcp"

var DefaultStage = "GA"

// gcpProvider implements the ProviderImpl interface for GCP
type gcpProvider struct {
	*models.BaseProvider

	client    *GcpConfigurationProvider
	iamClient *iam.Service
	crmClient *cloudresourcemanager.Service
}

func (p *gcpProvider) Initialize(identifier string, provider models.Provider) error {
	// Set the provider to the base provider
	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		GcpCapabilities,
	)

	ctx := context.Background()

	// Configure GCP client options based on available credentials
	gcpConfig := p.GetConfig()

	gcpClient, err := CreateGcpConfig(gcpConfig)
	if err != nil {
		return fmt.Errorf("failed to create GCP config: %w", err)
	}

	p.client = gcpClient

	clientOptions := gcpClient.ClientOptions

	iamService, err := iam.NewService(ctx, clientOptions...)
	if err != nil {
		return fmt.Errorf("failed to create IAM client: %w", err)
	}
	p.iamClient = iamService

	crmService, err := cloudresourcemanager.NewService(ctx, clientOptions...)
	if err != nil {
		return fmt.Errorf("failed to create Resource Manager client: %w", err)
	}
	p.crmClient = crmService

	return nil
}

func (p *gcpProvider) GetIamClient() *iam.Service {
	return p.iamClient
}

func (p *gcpProvider) GetProjectId() string {
	if p.client == nil {
		return ""
	}
	return p.client.ProjectID
}

func (p *gcpProvider) GetStage() string {
	return p.client.Stage
}

func CreateGcpConfig(gcpConfig *models.BasicConfig) (*GcpConfigurationProvider, error) {
	var clientOptions []option.ClientOption
	var credentialsData []byte

	projectId, foundProjectId := gcpConfig.GetString("project_id")

	if !foundProjectId {

		// Try and figure out the project ID from the environment
		if metadata.OnGCE() {
			id, err := metadata.ProjectIDWithContext(context.Background())
			if err != nil {
				return nil, fmt.Errorf("project_id not found in config and failed to get project_id from GCE metadata: %w", err)
			}
			projectId = id
		} else {
			return nil, fmt.Errorf("project_id not found in config and not running on GCE")
		}
	}

	if len(projectId) == 0 {
		return nil, fmt.Errorf("project_id must be specified in GCP provider configuration")
	}

	projectStage := gcpConfig.GetStringWithDefault("stage", DefaultStage)

	// Check for service account key file path
	serviceAccountKeyPath, foundKeyPath := gcpConfig.GetString("service_account_key_path")
	// Check for service account key JSON content (legacy format)
	serviceAccountKey, foundKey := gcpConfig.GetString("service_account_key")
	// Check for nested credentials object
	credentials, foundCredentials := gcpConfig.GetMap("credentials")

	if foundKeyPath {
		logrus.Info("Using GCP service account key file")

		// Read service account credentials
		var err error
		credentialsData, err = os.ReadFile(serviceAccountKeyPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read service account key file: %w", err)
		}

		clientOptions = append(clientOptions, option.WithCredentialsFile(serviceAccountKeyPath))
	} else if foundKey {
		logrus.Info("Using GCP service account key from config")
		credentialsData = []byte(serviceAccountKey)
		clientOptions = append(clientOptions, option.WithCredentialsJSON(credentialsData))
	} else if foundCredentials {
		logrus.Info("Using GCP service account credentials from config")
		// Convert the credentials map to JSON
		var err error
		credentialsData, err = json.Marshal(credentials)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal credentials to JSON: %w", err)
		}
		clientOptions = append(clientOptions, option.WithCredentialsJSON(credentialsData))
	} else {
		logrus.Info("No GCP credentials provided, using Application Default Credentials (ADC)")
		// No explicit credentials provided, will use Application Default Credentials
		// This includes:
		// - GOOGLE_APPLICATION_CREDENTIALS environment variable
		// - gcloud auth application-default login
		// - Compute Engine/GKE metadata service
		// - Cloud Shell credentials
	}

	return &GcpConfigurationProvider{
		ProjectID:       projectId,
		Stage:           projectStage,
		ClientOptions:   clientOptions,
		credentialsData: credentialsData, // do not allow exporting this field
	}, nil
}

// CreateJWTConfig creates a JWT config for domain-wide delegation with the given scopes
func (g *GcpConfigurationProvider) CreateJWTConfig(scopes ...string) (*jwt.Config, error) {
	if g.credentialsData == nil {
		return nil, fmt.Errorf("no credentials data available for JWT config creation")
	}

	// Create JWT config for domain-wide delegation
	conf, err := google.JWTConfigFromJSON(g.credentialsData, scopes...)
	if err != nil {
		return nil, fmt.Errorf("failed to create JWT config: %w", err)
	}

	return conf, nil
}

type GcpConfigurationProvider struct {
	ProjectID     string
	Stage         string
	ClientOptions []option.ClientOption

	// Unexported: only accessible within this package
	credentialsData []byte
}

func init() {
	providers.Register(GcpProviderName, &gcpProvider{})
}
