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
	"github.com/aws/aws-sdk-go-v2/service/identitystore"
	"github.com/aws/aws-sdk-go-v2/service/ssoadmin"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

// awsProvider implements the ProviderImpl interface for AWS
type awsProvider struct {
	*models.BaseProvider
	region              string
	accountID           string
	service             *iam.Client
	stsService          *sts.Client
	ssoAdminService     *ssoadmin.Client
	identityStoreClient *identitystore.Client
	permissions         []models.ProviderPermission
	permissionsIndex    bleve.Index
	roles               []models.ProviderRole
	rolesIndex          bleve.Index
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

	p.region = awsConfig.GetStringWithDefault("region", "us-east-1")
	p.service = iam.NewFromConfig(sdkConfig.Config)
	p.stsService = sts.NewFromConfig(sdkConfig.Config)
	p.ssoAdminService = ssoadmin.NewFromConfig(sdkConfig.Config)
	p.identityStoreClient = identitystore.NewFromConfig(sdkConfig.Config)

	// Set the account ID from config or retrieve it via STS
	err = p.GetAccountId(awsConfig)
	if err != nil {
		return fmt.Errorf("failed to set account ID: %w", err)
	}

	return nil
}

func CreateAwsConfig(awsConfig *models.BasicConfig) (*AwsConfigurationProvider, error) {

	awsOptions := []func(*config.LoadOptions) error{}

	awsProfile, foundProfile := awsConfig.GetString("profile")

	awsAccountId, foundAccountId := awsConfig.GetString("access_key_id")
	awsAccountSecret, foundAccountSecret := awsConfig.GetString("secret_access_key")

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

	configRegion := awsConfig.GetStringWithDefault("region", "us-east-1")

	logrus.WithField("region", configRegion).Info("Setting AWS region")

	awsOptions = append(awsOptions,
		config.WithRegion(
			configRegion,
		))

	// Support custom endpoint for testing (e.g., LocalStack)
	if endpoint, found := awsConfig.GetString("endpoint"); found {
		logrus.WithField("endpoint", endpoint).Info("Using custom AWS endpoint")
		awsOptions = append(awsOptions, config.WithEndpointResolverWithOptions(
			aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
				return aws.Endpoint{
					URL:           endpoint,
					SigningRegion: region,
				}, nil
			}),
		))
	}

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

func (p *awsProvider) GetIamClient() *iam.Client {
	return p.service
}

func (p *awsProvider) GetRegion() string {
	return p.region
}

// GetAccountId sets the AWS account ID from config or retrieves it via STS
func (p *awsProvider) GetAccountId(config *models.BasicConfig) error {

	ctx := context.Background()

	accountId, found := config.GetString("account_id")

	if found && len(accountId) > 0 {
		p.accountID = accountId
		logrus.WithField("account_id", p.accountID).Info("Using configured AWS account ID")
		return nil
	}

	// If not in config, retrieve via STS
	callerIdentity, err := p.stsService.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err != nil {
		return fmt.Errorf("failed to get AWS account ID via STS: %w", err)
	}

	foundAccount := callerIdentity.Account

	if foundAccount == nil || *foundAccount == "" {
		return fmt.Errorf("failed to retrieve valid AWS account ID via STS")
	}

	p.accountID = *foundAccount
	logrus.WithField("account_id", p.accountID).Info("Retrieved account ID via STS")
	return nil
}

// GetAccountID returns the cached AWS account ID
func (p *awsProvider) GetAccountID() string {
	return p.accountID
}

type AwsConfigurationProvider struct {
	Config aws.Config
}

func init() {
	providers.Register("aws", &awsProvider{})
}
