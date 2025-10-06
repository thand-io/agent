package vault

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
	"github.com/thand-io/agent/internal/models"
	awsProvider "github.com/thand-io/agent/internal/providers/aws"
)

type awsVault struct {
	config  *models.BasicConfig
	service *secretsmanager.Client
}

func NewAwsVaultFromConfig(config *models.BasicConfig) models.VaultImpl {
	return &awsVault{
		config: config,
	}
}

/*
Initialize(config map[string]any) error

GetSecret(key string) (string, error)
StoreSecret(key string, value string) error
*/
func (a *awsVault) Initialize() error {

	// Initialize AWS SDK configuration

	sdkConfig, err := awsProvider.CreateAwsConfig(a.config)

	if err != nil {
		return fmt.Errorf("failed to create AWS config: %w", err)
	}

	a.service = secretsmanager.NewFromConfig(sdkConfig.Config)

	return nil
}

func (a *awsVault) Shutdown() error {
	return nil
}

func (a *awsVault) GetSecret(key string) ([]byte, error) {
	if a.service == nil {
		return nil, fmt.Errorf("AWS Secrets Manager service not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	input := &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(key),
	}

	result, err := a.service.GetSecretValue(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to get secret %s: %w", key, err)
	}

	if result.SecretString == nil {
		return nil, fmt.Errorf("secret %s has no string value", key)
	}

	return []byte(*result.SecretString), nil
}

func (a *awsVault) StoreSecret(key string, value []byte) error {
	if a.service == nil {
		return fmt.Errorf("AWS Secrets Manager service not initialized")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// First, try to update the secret if it exists
	updateInput := &secretsmanager.UpdateSecretInput{
		SecretId:     aws.String(key),
		SecretString: aws.String(string(value)),
	}

	_, err := a.service.UpdateSecret(ctx, updateInput)
	if err != nil {
		// If the secret doesn't exist, create it
		var resourceNotFound *types.ResourceNotFoundException
		if errors.As(err, &resourceNotFound) {
			createInput := &secretsmanager.CreateSecretInput{
				Name:         aws.String(key),
				SecretString: aws.String(string(value)),
				Description:  aws.String("Secret managed by thand-io agent"),
			}

			_, createErr := a.service.CreateSecret(ctx, createInput)
			if createErr != nil {
				return fmt.Errorf("failed to create secret %s: %w", key, createErr)
			}
			return nil
		}
		return fmt.Errorf("failed to update secret %s: %w", key, err)
	}

	return nil
}
