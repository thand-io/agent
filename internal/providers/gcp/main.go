package gcp

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"

	"github.com/blevesearch/bleve/v2"
	"github.com/sirupsen/logrus"

	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"

	"google.golang.org/api/cloudresourcemanager/v1"
	iam "google.golang.org/api/iam/v1"
	"google.golang.org/api/option"
)

var DefaultStage = "GA"

// gcpProvider implements the ProviderImpl interface for GCP
type gcpProvider struct {
	*models.BaseProvider

	client           *GcpConfigurationProvider
	iamClient        *iam.Service
	crmClient        *cloudresourcemanager.Service
	permissions      []models.ProviderPermission
	permissionsIndex bleve.Index
	roles            []models.ProviderRole
	rolesIndex       bleve.Index
}

func (p *gcpProvider) Initialize(provider models.Provider) error {
	// Set the provider to the base provider
	p.BaseProvider = models.NewBaseProvider(
		provider,
		models.ProviderCapabilityRBAC,
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
	projectStage := gcpClient.Stage

	// Load GCP Permissions and Roles from embedded resources
	err = p.LoadPermissions(projectStage)
	if err != nil {
		return fmt.Errorf("failed to load permissions: %w", err)
	}

	err = p.LoadRoles(projectStage)
	if err != nil {
		return fmt.Errorf("failed to load roles: %w", err)
	}

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
	return p.client.ProjectID
}

func (p *gcpProvider) GetStage() string {
	return p.client.Stage
}

func init() {
	providers.Register("gcp", &gcpProvider{})
}

type gcpPredefinedRole struct {
	Description string `json:"description"`
	Etag        string `json:"etag"`
	Name        string `json:"name"`
	Stage       string `json:"stage"`
	Title       string `json:"title"`
}

func CreateGcpConfig(gcpConfig *models.BasicConfig) (*GcpConfigurationProvider, error) {
	var clientOptions []option.ClientOption

	projectId, foundProjectId := gcpConfig.GetString("project_id")

	if !foundProjectId {
		return nil, fmt.Errorf("project_id not found in config")
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
		clientOptions = append(clientOptions, option.WithCredentialsFile(serviceAccountKeyPath))
	} else if foundKey {
		logrus.Info("Using GCP service account key from config")
		clientOptions = append(clientOptions, option.WithCredentialsJSON([]byte(serviceAccountKey)))
	} else if foundCredentials {
		logrus.Info("Using GCP service account credentials from config")
		// Convert the credentials map to JSON
		credentialsJSON, err := json.Marshal(credentials)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal credentials to JSON: %w", err)
		}
		clientOptions = append(clientOptions, option.WithCredentialsJSON(credentialsJSON))
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
		ProjectID:     projectId,
		Stage:         projectStage,
		ClientOptions: clientOptions,
	}, nil
}

type GcpConfigurationProvider struct {
	ProjectID     string
	Stage         string
	ClientOptions []option.ClientOption
}
