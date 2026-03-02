package models

import (
	"encoding/json"
	"fmt"

	"github.com/mitchellh/mapstructure"
)

// ConfigSchema is the base interface for all provider config schemas
type ConfigSchema interface {
	Unmarshal(config *BasicConfig) error
	Validate() error
}

// BaseConfigSchema provides common unmarshal functionality
type BaseConfigSchema struct{}

// Unmarshal converts BasicConfig to a typed struct using mapstructure.
// A nil config is treated as an empty configuration (all fields take their zero/default values).
func (b *BaseConfigSchema) Unmarshal(config *BasicConfig, target any) error {
	configMap := map[string]any{}
	if config != nil {
		configMap = config.AsMap()
	}

	decoder, err := mapstructure.NewDecoder(&mapstructure.DecoderConfig{
		WeaklyTypedInput: true,
		Result:           target,
		TagName:          "json",
	})
	if err != nil {
		return fmt.Errorf("failed to create decoder: %w", err)
	}

	if err := decoder.Decode(configMap); err != nil {
		return fmt.Errorf("failed to decode config: %w", err)
	}

	return nil
}

// AWSCredentials represents common AWS authentication configuration
type AWSCredentials struct {
	Region          string `json:"region" mapstructure:"region"`
	AccessKeyID     string `json:"access_key_id" mapstructure:"access_key_id" sensitive:"true"`
	SecretAccessKey string `json:"secret_access_key" mapstructure:"secret_access_key" sensitive:"true"`
	SessionToken    string `json:"session_token" mapstructure:"session_token" sensitive:"true"`
	Profile         string `json:"profile" mapstructure:"profile"`
	SSOStartURL     string `json:"sso_start_url" mapstructure:"sso_start_url" validate:"omitempty,url"`
	Endpoint        string `json:"endpoint" mapstructure:"endpoint" validate:"omitempty,url"`
	IMDSDisable     bool   `json:"imds_disable" mapstructure:"imds_disable"`
}

// AzureCredentials represents common Azure authentication configuration
type AzureCredentials struct {
	SubscriptionID string `json:"subscription_id" mapstructure:"subscription_id" validate:"required"`
	ResourceGroup  string `json:"resource_group" mapstructure:"resource_group"`
	TenantID       string `json:"tenant_id" mapstructure:"tenant_id"`
	ClientID       string `json:"client_id" mapstructure:"client_id"`
	ClientSecret   string `json:"client_secret" mapstructure:"client_secret" sensitive:"true"`
}

// GCPServiceAccountCredentials represents a GCP service account JSON key structure
type GCPServiceAccountCredentials struct {
	Type                    string `json:"type" mapstructure:"type"`
	ProjectID               string `json:"project_id" mapstructure:"project_id"`
	PrivateKeyID            string `json:"private_key_id" mapstructure:"private_key_id" sensitive:"true"`
	PrivateKey              string `json:"private_key" mapstructure:"private_key" sensitive:"true"`
	ClientEmail             string `json:"client_email" mapstructure:"client_email" validate:"omitempty,email"`
	ClientID                string `json:"client_id" mapstructure:"client_id"`
	AuthURI                 string `json:"auth_uri" mapstructure:"auth_uri" validate:"omitempty,url"`
	TokenURI                string `json:"token_uri" mapstructure:"token_uri" validate:"omitempty,url"`
	AuthProviderX509CertURL string `json:"auth_provider_x509_cert_url" mapstructure:"auth_provider_x509_cert_url" validate:"omitempty,url"`
	ClientX509CertURL       string `json:"client_x509_cert_url" mapstructure:"client_x509_cert_url" validate:"omitempty,url"`
}

// GCPCredentials represents common GCP authentication configuration
type GCPCredentials struct {
	ProjectID             string                        `json:"project_id" mapstructure:"project_id" validate:"required"`
	Stage                 string                        `json:"stage" mapstructure:"stage" validate:"omitempty,oneof=GA BETA ALPHA"`
	ServiceAccountKeyPath string                        `json:"service_account_key_path" mapstructure:"service_account_key_path"`
	ServiceAccountKey     string                        `json:"service_account_key" mapstructure:"service_account_key" sensitive:"true"`
	Credentials           *GCPServiceAccountCredentials `json:"credentials" mapstructure:"credentials"`
}

// OAuth2Config represents common OAuth2 configuration
type OAuth2Config struct {
	ClientID     string   `json:"client_id" mapstructure:"client_id" validate:"required"`
	ClientSecret string   `json:"client_secret" mapstructure:"client_secret" validate:"required" sensitive:"true"`
	Scopes       []string `json:"scopes" mapstructure:"scopes"`
	RedirectURL  string   `json:"redirect_url" mapstructure:"redirect_url" validate:"omitempty,url"`
	AuthURL      string   `json:"auth_url" mapstructure:"auth_url" validate:"omitempty,url"`
	TokenURL     string   `json:"token_url" mapstructure:"token_url" validate:"omitempty,url"`
}

// SMTPConfig represents SMTP email configuration
type SMTPConfig struct {
	Host     string `json:"host" mapstructure:"host" validate:"required"`
	Port     int    `json:"port" mapstructure:"port" validate:"required,min=1,max=65535"`
	User     string `json:"user" mapstructure:"user"`
	Password string `json:"password" mapstructure:"password" sensitive:"true"`
	From     string `json:"from" mapstructure:"from" validate:"required,email"`
	UseTLS   bool   `json:"use_tls" mapstructure:"use_tls"`
}

// EndpointConfig represents a service endpoint configuration
type EndpointConfig struct {
	Endpoint string `json:"endpoint" mapstructure:"endpoint" validate:"required,url"`
}

// TokenConfig represents a simple token-based authentication
type TokenConfig struct {
	Token string `json:"token" mapstructure:"token" validate:"required" sensitive:"true"`
}

// UnmarshalJSON helper for GCPServiceAccountCredentials
func (g *GCPServiceAccountCredentials) UnmarshalJSON(data []byte) error {
	type Alias GCPServiceAccountCredentials
	aux := &struct {
		*Alias
	}{
		Alias: (*Alias)(g),
	}
	if err := json.Unmarshal(data, &aux); err != nil {
		return err
	}
	return nil
}
