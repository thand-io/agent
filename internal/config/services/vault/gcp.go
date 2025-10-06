package vault

import (
	"context"
	"fmt"
	"time"

	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/thand-io/agent/internal/models"
	gcpProvider "github.com/thand-io/agent/internal/providers/gcp"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type gcpVault struct {
	config *models.BasicConfig
	client *secretmanager.Client
	creds  *gcpProvider.GcpConfigurationProvider
}

func NewGcpVaultFromConfig(config *models.BasicConfig) models.VaultImpl {
	return &gcpVault{
		config: config,
	}
}

/*
Initialize(config map[string]any) error

GetSecret(key string) (string, error)
StoreSecret(key string, value string) error
*/
func (g *gcpVault) Initialize() error {

	// Create GCP credentials using the provider's CreateGcpConfig function
	creds, err := gcpProvider.CreateGcpConfig(g.config)
	if err != nil {
		return fmt.Errorf("failed to create GCP credential: %w", err)
	}

	g.creds = creds

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	clientOptions := g.creds.ClientOptions

	smClient, err := secretmanager.NewClient(ctx, clientOptions...)

	if err != nil {
		return fmt.Errorf("failed to create Secret Manager client: %w", err)
	}

	g.client = smClient

	return nil
}

func (g *gcpVault) Shutdown() error {
	if g.client != nil {
		return g.client.Close()
	}
	return nil
}

func (g *gcpVault) GetSecret(key string) ([]byte, error) {
	if g.client == nil {
		return nil, fmt.Errorf("GCP Secret Manager client not initialized")
	}

	projectId := g.creds.ProjectID

	if len(projectId) == 0 {
		return nil, fmt.Errorf("project_id not found in config")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Build the resource name: projects/{project_id}/secrets/{secret_id}/versions/latest
	name := fmt.Sprintf("projects/%s/secrets/%s/versions/latest", projectId, key)

	req := &secretmanagerpb.AccessSecretVersionRequest{
		Name: name,
	}

	result, err := g.client.AccessSecretVersion(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s: %w", key, err)
	}

	return result.Payload.Data, nil
}

func (g *gcpVault) StoreSecret(key string, value []byte) error {
	if g.client == nil {
		return fmt.Errorf("GCP Secret Manager client not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	projectId := g.creds.ProjectID

	if len(projectId) == 0 {
		return fmt.Errorf("project_id not found in config")
	}

	// First, try to create a new secret version if the secret exists
	secretName := fmt.Sprintf("projects/%s/secrets/%s", projectId, key)

	addVersionReq := &secretmanagerpb.AddSecretVersionRequest{
		Parent: secretName,
		Payload: &secretmanagerpb.SecretPayload{
			Data: value,
		},
	}

	_, err := g.client.AddSecretVersion(ctx, addVersionReq)
	if err != nil {
		// If the secret doesn't exist, create it first
		if status.Code(err) == codes.NotFound {
			// Create the secret
			createReq := &secretmanagerpb.CreateSecretRequest{
				Parent:   fmt.Sprintf("projects/%s", projectId),
				SecretId: key,
				Secret: &secretmanagerpb.Secret{
					Replication: &secretmanagerpb.Replication{
						Replication: &secretmanagerpb.Replication_Automatic_{
							Automatic: &secretmanagerpb.Replication_Automatic{},
						},
					},
				},
			}

			_, createErr := g.client.CreateSecret(ctx, createReq)
			if createErr != nil {
				return fmt.Errorf("failed to create secret %s: %w", key, createErr)
			}

			// Now add the secret version
			_, addErr := g.client.AddSecretVersion(ctx, addVersionReq)
			if addErr != nil {
				return fmt.Errorf("failed to add version to secret %s: %w", key, addErr)
			}

			return nil
		}
		return fmt.Errorf("failed to add version to secret %s: %w", key, err)
	}

	return nil
}
