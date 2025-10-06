package aws

import (
	"context"
	"fmt"

	"github.com/blevesearch/bleve/v2"
	"github.com/sirupsen/logrus"

	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/iam"
)

// awsProvider implements the ProviderImpl interface for AWS
type awsProvider struct {
	*models.BaseProvider
	region           string
	service          *iam.Client
	permissions      []models.ProviderPermission
	permissionsIndex bleve.Index
	roles            []models.ProviderRole
	rolesIndex       bleve.Index
}

func (p *awsProvider) Initialize(provider models.Provider) error {
	p.BaseProvider = models.NewBaseProvider(
		provider,
		models.ProviderCapabilityRBAC,
	)

	// Load EC2 Permissions. This loads from third_party/iam-dataset/aws/docs.json
	// this is an embedded resource
	err := p.LoadPermissions()
	if err != nil {
		return fmt.Errorf("failed to load permissions: %w", err)
	}

	err = p.LoadRoles()
	if err != nil {
		return fmt.Errorf("failed to load roles: %w", err)
	}

	// Right lets figure out how to initialize the AWS SDK
	awsConfig := p.GetConfig()

	sdkConfig, err := CreateAwsConfig(awsConfig)

	if err != nil {
		return fmt.Errorf("failed to create AWS config: %w", err)
	}

	p.service = iam.NewFromConfig(sdkConfig.Config)
	return nil
}

func CreateAwsConfig(awsConfig *models.BasicConfig) (*AwsConfigurationProvider, error) {

	awsOptions := []func(*config.LoadOptions) error{}

	awsProfile, foundProfile := awsConfig.GetString("profile")

	awsAccountId, foundAccountId := awsConfig.GetString("account_id")
	awsAccountSecret, foundAccountSecret := awsConfig.GetString("account_secret")

	if foundProfile {
		logrus.Info("Using shared AWS config profile")
		awsOptions = append(awsOptions, config.WithSharedConfigProfile(awsProfile))
	} else if foundAccountId && foundAccountSecret {
		logrus.Info("Using static AWS credentials")
		awsOptions = append(awsOptions, config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(awsAccountId, awsAccountSecret, ""),
		))
	} else {
		logrus.Info("No AWS credentials provided, using IAM role or default profile")
	}

	awsOptions = append(awsOptions,
		config.WithRegion(
			awsConfig.GetStringWithDefault("region", "us-east-1")))

	// Initialize AWS SDK clients here
	ctx := context.Background()

	awsSdkConfig, err := config.LoadDefaultConfig(
		ctx,
		awsOptions...,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return &AwsConfigurationProvider{
		Config: awsSdkConfig,
	}, nil

}

type AwsConfigurationProvider struct {
	Config aws.Config
}

func init() {
	providers.Register("aws", &awsProvider{})
}
