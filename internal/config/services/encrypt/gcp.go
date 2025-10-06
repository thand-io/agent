package encrypt

import (
	"context"
	"fmt"

	kms "cloud.google.com/go/kms/apiv1"
	"cloud.google.com/go/kms/apiv1/kmspb"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	gcpProvider "github.com/thand-io/agent/internal/providers/gcp"
)

type gcpEncrypt struct {
	config    *models.BasicConfig
	service   *kms.KeyManagementClient
	projectID string
	location  string
	keyRing   string
	keyName   string
	creds     *gcpProvider.GcpConfigurationProvider
}

func NewGcpEncryptionFromConfig(config *models.BasicConfig) models.EncryptionImpl {
	return &gcpEncrypt{
		config: config,
	}
}

/*
Initialize() error
Shutdown() error
Encrypt(plaintext string) ([]byte, error)
Decrypt(ciphertext []byte) (string, error)
*/
func (g *gcpEncrypt) Initialize() error {

	// Create GCP credentials using the provider's CreateGcpConfig function
	creds, err := gcpProvider.CreateGcpConfig(g.config)
	if err != nil {
		return fmt.Errorf("failed to create GCP credential: %w", err)
	}

	g.creds = creds

	ctx := context.Background()

	projectId, foundProjectId := g.config.GetString("project_id")
	if !foundProjectId {
		logrus.Errorln("project_id not found in config")
		return fmt.Errorf("project_id not found in config")
	}
	g.projectID = projectId

	// Location is often required for GCP KMS, default to global but allow override
	g.location = g.config.GetStringWithDefault("location", "global")

	keyRing, foundKeyRing := g.config.GetString("key_ring")
	if !foundKeyRing {
		logrus.Errorln("key_ring not found in config")
		return fmt.Errorf("key_ring not found in config")
	}
	g.keyRing = keyRing

	keyName, foundKeyName := g.config.GetString("key_name")
	if !foundKeyName {
		logrus.Errorln("key_name not found in config")
		return fmt.Errorf("key_name not found in config")
	}

	g.keyName = keyName

	clientOptions := g.creds.ClientOptions

	keyService, err := kms.NewKeyManagementClient(ctx, clientOptions...)
	if err != nil {
		logrus.WithError(err).Errorln("Failed to create KMS client")
		return fmt.Errorf("failed to create KMS client: %w", err)
	}

	g.service = keyService

	return nil
}

func (g *gcpEncrypt) Shutdown() error {
	if g.service != nil {
		return g.service.Close()
	}
	return nil
}

func (g *gcpEncrypt) Encrypt(plaintext []byte) ([]byte, error) {
	ctx := context.Background()

	if g.service == nil {
		return nil, fmt.Errorf("GCP KMS client not initialized")
	}

	cryptoKeyName := fmt.Sprintf("projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s",
		g.projectID, g.location, g.keyRing, g.keyName)

	req := &kmspb.EncryptRequest{
		Name:      cryptoKeyName,
		Plaintext: plaintext,
	}

	result, err := g.service.Encrypt(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt data: %w", err)
	}

	return result.Ciphertext, nil
}

func (g *gcpEncrypt) Decrypt(ciphertext []byte) ([]byte, error) {
	ctx := context.Background()

	if g.service == nil {
		return nil, fmt.Errorf("GCP KMS client not initialized")
	}

	cryptoKeyName := fmt.Sprintf("projects/%s/locations/%s/keyRings/%s/cryptoKeys/%s",
		g.projectID, g.location, g.keyRing, g.keyName)

	req := &kmspb.DecryptRequest{
		Name:       cryptoKeyName,
		Ciphertext: ciphertext,
	}

	result, err := g.service.Decrypt(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	return result.Plaintext, nil
}
