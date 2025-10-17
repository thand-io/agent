package encrypt

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/kms"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
	awsProvider "github.com/thand-io/agent/internal/providers/aws"
)

type awsEncrypt struct {
	config  *models.BasicConfig
	service *kms.Client
	kmsArn  string
}

func NewAwsEncryptionFromConfig(config *models.BasicConfig) models.EncryptionImpl {
	return &awsEncrypt{
		config: config,
	}
}

/*
Initialize(config map[string]any) error

GetSecret(key string) (string, error)
StoreSecret(key string, value string) error
*/
func (a *awsEncrypt) Initialize() error {

	// Initialize AWS KMS client

	sdkConfig, err := awsProvider.CreateAwsConfig(a.config)

	if err != nil {
		return fmt.Errorf("failed to create AWS config: %w", err)
	}

	a.service = kms.NewFromConfig(sdkConfig.Config)

	// Get the KMS Key ARN from config
	kmsArn, found := a.config.GetString("kms_arn")

	if !found || len(kmsArn) == 0 {
		return fmt.Errorf("missing required AWS KMS configuration: kms_arn is required")
	}

	a.kmsArn = kmsArn

	return nil
}

func (a *awsEncrypt) Shutdown() error {
	return nil
}

func (a *awsEncrypt) Decrypt(ctx context.Context, cipherText []byte) ([]byte, error) {

	if a.service == nil {
		return nil, fmt.Errorf("AWS KMS service not initialized")
	}

	if len(a.kmsArn) == 0 {
		return nil, fmt.Errorf("KMS ARN is not configured")
	}

	if len(cipherText) == 0 {
		return nil, fmt.Errorf("cipher text cannot be empty")
	}

	// Implementation for getting a secret from AWS KMS
	// use the AWS KMS client to retrive the secret

	output, err := a.service.Decrypt(ctx, &kms.DecryptInput{
		CiphertextBlob: cipherText,
		KeyId:          aws.String(a.kmsArn),
	})

	if err != nil {
		logrus.WithError(err).Errorln("Failed to decrypt secret")
		return nil, fmt.Errorf("failed to decrypt secret: %w", err)
	}

	return output.Plaintext, nil
}

func (a *awsEncrypt) Encrypt(ctx context.Context, plainText []byte) ([]byte, error) {

	if a.service == nil {
		return nil, fmt.Errorf("AWS KMS service not initialized")
	}

	if len(a.kmsArn) == 0 {
		return nil, fmt.Errorf("KMS ARN is not configured")
	}

	if len(plainText) == 0 {
		return nil, fmt.Errorf("plain text cannot be empty")
	}

	output, err := a.service.Encrypt(ctx, &kms.EncryptInput{
		KeyId:     aws.String(a.kmsArn), // Replace with your KMS key alias or ID
		Plaintext: plainText,
	})

	if err != nil {
		logrus.WithError(err).Errorln("Failed to encrypt secret")
		return nil, fmt.Errorf("failed to encrypt secret: %w", err)
	}

	return output.CiphertextBlob, nil
}
