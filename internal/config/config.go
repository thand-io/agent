package config

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/subosito/gotenv"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/config/environment"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/sessions"
)

var ErrNoActiveLoginSession = fmt.Errorf(
	"you must login first. No valid session found to sync with login server")

func DefaultConfig() *Config {

	v := viper.New()

	// Set default values
	setDefaults(v)

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		log.Fatalf("error unmarshaling default config: %v", err)
	}

	return &config
}

// Load loads the configuration from various sources
func Load(configFile string) (*Config, error) {
	if err := loadEnvFile(); err != nil {
		return nil, err
	}

	v := viper.New()

	if err := setupViperConfig(v, configFile); err != nil {
		return nil, err
	}

	bindEnvironmentVariables(v)

	config, err := readAndUnmarshalConfig(v)
	if err != nil {
		return nil, err
	}

	if err := setupLogging(config, v); err != nil {
		return nil, err
	}

	return config, nil
}

// loadEnvFile loads the .env file if it exists
func loadEnvFile() error {
	if err := gotenv.Load(); err != nil {
		// .env file not found, that's okay - continue with other sources
		if !os.IsNotExist(err) {
			fmt.Printf("Warning: Error loading .env file: %v\n", err)
		}
	}
	return nil
}

// setupViperConfig configures viper with file paths and defaults
func setupViperConfig(v *viper.Viper, configFile string) error {
	// Set configuration file details
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.AddConfigPath(".")
	v.AddConfigPath("./config")
	v.AddConfigPath("/etc/thand")
	v.AddConfigPath("~/.config/thand")

	if len(configFile) > 0 {
		v.SetConfigFile(configFile)
	}

	if err := setupHomeConfigPath(v); err != nil {
		return err
	}

	// Set default values
	setDefaults(v)

	// Set environment variable settings
	v.SetEnvPrefix("THAND")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	v.AllowEmptyEnv(true)

	return nil
}

// setupHomeConfigPath adds the home directory config path if available
func setupHomeConfigPath(v *viper.Viper) error {
	home := os.Getenv("HOME")
	if len(home) == 0 {
		return nil
	}

	// Get the user's home directory
	usr, err := user.Current()
	if err != nil {
		log.Fatalf("Failed to get current user: %v", err)
	}

	// Expand the session manager path to use the actual home directory
	sessionPath := filepath.Join(usr.HomeDir, ".config", "thand")
	v.AddConfigPath(sessionPath)

	// Check if the folder exists and create it if it does not exist
	if _, err := os.Stat(sessionPath); os.IsNotExist(err) {
		if err := os.MkdirAll(sessionPath, os.ModePerm); err != nil {
			logrus.Errorf("Failed to create config directory: %v", err)
		}
	}

	return nil
}

// bindEnvironmentVariables binds all environment variables to viper
func bindEnvironmentVariables(v *viper.Viper) {
	// Platform environment variables
	v.BindEnv("environment.platform", "THAND_ENVIRONMENT_PLATFORM")

	// Default api key and timeout
	v.BindEnv("environment.config.api_key", "THAND_ENVIRONMENT_CONFIG_API_KEY")
	v.BindEnv("environment.config.timeout", "THAND_ENVIRONMENT_CONFIG_TIMEOUT")

	bindCloudProviderEnvVars(v)
	bindVaultEnvVars(v)
	bindLoggingEnvVars(v)
	bindServiceEnvVars(v)
}

// bindCloudProviderEnvVars binds cloud provider specific environment variables
func bindCloudProviderEnvVars(v *viper.Viper) {
	// GCP environment variables
	v.BindEnv("environment.config.project_id", "THAND_ENVIRONMENT_CONFIG_PROJECT_ID")
	v.BindEnv("environment.config.location", "THAND_ENVIRONMENT_CONFIG_LOCATION")
	v.BindEnv("environment.config.key_ring", "THAND_ENVIRONMENT_CONFIG_KEY_RING")
	v.BindEnv("environment.config.key_name", "THAND_ENVIRONMENT_CONFIG_KEY_NAME")

	// Azure environment variables
	v.BindEnv("environment.config.vault_url", "THAND_ENVIRONMENT_CONFIG_VAULT_URL")

	// AWS environment variables
	v.BindEnv("environment.config.profile", "THAND_ENVIRONMENT_CONFIG_PROFILE")
	v.BindEnv("environment.config.region", "THAND_ENVIRONMENT_CONFIG_REGION")
	v.BindEnv("environment.config.account_id", "THAND_ENVIRONMENT_CONFIG_ACCOUNT_ID")
	v.BindEnv("environment.config.account_secret", "THAND_ENVIRONMENT_CONFIG_ACCOUNT_SECRET")
}

// bindVaultEnvVars binds HashiCorp Vault and secret management environment variables
func bindVaultEnvVars(v *viper.Viper) {
	// HashiCorp Vault environment variables
	v.BindEnv("environment.config.secret_path", "THAND_ENVIRONMENT_CONFIG_SECRET_PATH")
	v.BindEnv("environment.config.mount_path", "THAND_ENVIRONMENT_CONFIG_MOUNT_PATH")

	// Define vault names for secret key lookups
	v.BindEnv("roles.vault", "THAND_ROLES_VAULT")
	v.BindEnv("workflows.vault", "THAND_WORKFLOWS_VAULT")
	v.BindEnv("providers.vault", "THAND_PROVIDERS_VAULT")
}

// bindLoggingEnvVars binds logging configuration environment variables
func bindLoggingEnvVars(v *viper.Viper) {
	v.BindEnv("logging.level", "THAND_LOGGING_LEVEL")
	v.BindEnv("logging.format", "THAND_LOGGING_FORMAT")
	v.BindEnv("logging.output", "THAND_LOGGING_OUTPUT")
}

// bindServiceEnvVars binds service configuration environment variables
func bindServiceEnvVars(v *viper.Viper) {
	// LLM service environment variables
	v.BindEnv("services.llm.provider", "THAND_SERVICES_LLM_PROVIDER")
	v.BindEnv("services.llm.api_key", "THAND_SERVICES_LLM_API_KEY")
	v.BindEnv("services.llm.base_url", "THAND_SERVICES_LLM_BASE_URL")
	v.BindEnv("services.llm.model", "THAND_SERVICES_LLM_MODEL")

	// Temporal service environment variables
	v.BindEnv("services.temporal.host", "THAND_SERVICES_TEMPORAL_HOST")
	v.BindEnv("services.temporal.port", "THAND_SERVICES_TEMPORAL_PORT")
	v.BindEnv("services.temporal.namespace", "THAND_SERVICES_TEMPORAL_NAMESPACE")
	v.BindEnv("services.temporal.mtls_pem", "THAND_SERVICES_TEMPORAL_MTLS_PEM")
	v.BindEnv("services.temporal.api_key", "THAND_SERVICES_TEMPORAL_API_KEY")
}

// readAndUnmarshalConfig reads the configuration file and unmarshals it
func readAndUnmarshalConfig(v *viper.Viper) (*Config, error) {
	// Read configuration file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config file: %w", err)
		}
		// Config file not found; proceed with defaults and environment variables
	}

	var config Config
	if err := v.Unmarshal(&config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return &config, nil
}

// setupLogging configures the logging system based on the config
func setupLogging(config *Config, v *viper.Viper) error {
	// Set logging level
	logrusLevel, err := logrus.ParseLevel(config.Logging.Level)
	if err != nil {
		return fmt.Errorf("error parsing log level: %w", err)
	}

	logrus.SetLevel(logrusLevel)
	config.logger = *NewThandLogger()
	logrus.AddHook(&config.logger)

	// Set logging format
	switch strings.ToLower(config.Logging.Format) {
	case "json":
		logrus.SetFormatter(&logrus.JSONFormatter{})
	case "text":
		logrus.SetFormatter(&logrus.TextFormatter{
			FullTimestamp: true,
		})
	default:
		logrus.WithFields(logrus.Fields{
			"format": config.Logging.Format,
		}).Warn("Unknown log format")
	}

	// Dump out the config settings if in debug mode
	if logrusLevel >= logrus.DebugLevel {
		for key, value := range v.AllSettings() {
			logrus.Debugf("Config '%s': %v\n", key, value)
		}
	}

	return nil
}

func (c *Config) ReloadConfig() error {
	var wg sync.WaitGroup
	var mu sync.Mutex
	var errors []error

	// Load roles in parallel
	if c.Roles.IsExternal() {
		wg.Go(func() {
			roles, err := c.LoadRoles()
			if err != nil {
				logrus.WithError(err).Errorln("Error loading roles")
				mu.Lock()
				errors = append(errors, fmt.Errorf("loading roles: %w", err))
				mu.Unlock()
			} else if len(roles) > 0 {
				logrus.Infoln("Loaded roles from external source:", len(roles))
				mu.Lock()
				c.Roles.Definitions = roles
				mu.Unlock()
			} else {
				logrus.Warningln("No roles loaded from external source")
			}
		})
	}

	// Load workflows in parallel
	if c.Workflows.IsExternal() {
		wg.Go(func() {
			workflows, err := c.LoadWorkflows()
			if err != nil {
				logrus.WithError(err).Errorln("Error loading workflows")
				mu.Lock()
				errors = append(errors, fmt.Errorf("loading workflows: %w", err))
				mu.Unlock()
			} else if len(workflows) > 0 {
				logrus.Infoln("Loaded workflows from external source:", len(workflows))
				mu.Lock()
				c.Workflows.Definitions = workflows
				mu.Unlock()
			} else {
				logrus.Warningln("No workflows loaded from external source")
			}
		})
	}

	// Load providers in parallel
	if c.Providers.IsExternal() {
		wg.Go(func() {
			providers, err := c.LoadProviders()
			if err != nil {
				logrus.WithError(err).Errorln("Error loading providers")
				mu.Lock()
				errors = append(errors, fmt.Errorf("loading providers: %w", err))
				mu.Unlock()
			} else if len(providers) > 0 {
				logrus.Infoln("Loaded providers from external source:", len(providers))
				mu.Lock()
				c.Providers.Definitions = providers
				mu.Unlock()
			} else {
				logrus.Warningln("No providers loaded from external source")
			}
		})
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Return first error if any occurred
	if len(errors) > 0 {
		return errors[0]
	}

	return nil
}

func (c *Config) HasLoginServer() bool {
	return len(c.Login.Endpoint) > 0
}

func (c *Config) SyncWithLoginServer() error {

	if len(c.Login.Endpoint) == 0 {
		return fmt.Errorf("no login server endpoint configured")
	}

	// Providers need to be hard synced. Everything else
	// can be done async

	apiUrl := c.GetLoginServerApiUrl()

	sessionManager := sessions.GetSessionManager()

	loginServer, err := sessionManager.GetLoginServer(c.GetLoginServerHostname())

	if err != nil {
		return fmt.Errorf("failed to get login server session: %w", err)
	}

	localToken := ""

	if c.HasAPIKey() {

		logrus.Debugln("Using API key for login server authentication")
		localToken = c.GetAPIKey()

	} else {

		logrus.Debugf("Looking for valid session to sync with login server at: %s", apiUrl)
		localSessions := loginServer.GetSessions()

		// Find the first non-expired session token
		for providerName, session := range localSessions {
			if !session.IsExpired() {
				logrus.Debugf("Found valid session for provider '%s'", providerName)
				localToken = session.GetEncodedLocalSession()
				break
			}
		}

		if len(localToken) == 0 {
			return ErrNoActiveLoginSession
		}

	}

	// Lets make our registration request. This will pull down our
	// remote configuration and also register this instance with the login server

	regResponse, err := c.RegisterWithLoginServer(localToken)

	if err != nil {
		return fmt.Errorf("failed to register with login server: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"response": regResponse,
	}).Debugf("Syncing configuration with login server at: %s", apiUrl)

	// Overwrite everything.
	c.Providers = ProviderConfig{
		URL: &model.Endpoint{
			EndpointConfig: &model.EndpointConfiguration{
				URI: &model.LiteralUri{Value: fmt.Sprintf("%s/providers", apiUrl)},
				Authentication: &model.ReferenceableAuthenticationPolicy{
					AuthenticationPolicy: &model.AuthenticationPolicy{
						Bearer: &model.BearerAuthenticationPolicy{
							Token: localToken,
						},
					},
				},
			},
		},
	}
	c.Roles = RoleConfig{
		URL: &model.Endpoint{
			EndpointConfig: &model.EndpointConfiguration{
				URI: &model.LiteralUri{Value: fmt.Sprintf("%s/roles", apiUrl)},
				Authentication: &model.ReferenceableAuthenticationPolicy{
					AuthenticationPolicy: &model.AuthenticationPolicy{
						Bearer: &model.BearerAuthenticationPolicy{
							Token: localToken,
						},
					},
				},
			},
		},
	}
	c.Workflows = WorkflowConfig{
		URL: &model.Endpoint{
			EndpointConfig: &model.EndpointConfiguration{
				URI: &model.LiteralUri{Value: fmt.Sprintf("%s/workflows", apiUrl)},
				Authentication: &model.ReferenceableAuthenticationPolicy{
					AuthenticationPolicy: &model.AuthenticationPolicy{
						Bearer: &model.BearerAuthenticationPolicy{
							Token: localToken,
						},
					},
				},
			},
		},
	}

	err = c.ReloadConfig()

	if err != nil {
		logrus.WithError(err).Errorln("Failed to sync configuration with login server")
	}

	// Update all providers, roles and workflows to be enabled

	// TODO Reload environment?

	return nil

}

func (c *Config) RegisterWithLoginServer(localToken string) (*RegistrationResponse, error) {

	reqBody, err := json.Marshal(RegistrationRequest{
		Environment: &c.Environment,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to marshal registration request: %w", err)
	}

	// No need for an API key we need to use the session
	// info
	res, err := common.InvokeHttpRequest(&model.HTTPArguments{
		Method: http.MethodPost,
		Endpoint: &model.Endpoint{
			EndpointConfig: &model.EndpointConfiguration{
				URI: &model.LiteralUri{Value: c.GetLoginServerApiUrl() + "/register"},
				Authentication: &model.ReferenceableAuthenticationPolicy{
					AuthenticationPolicy: &model.AuthenticationPolicy{
						Bearer: &model.BearerAuthenticationPolicy{
							Token: localToken,
						},
					},
				},
			},
		},
		Body: reqBody,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to invoke registration request: %w", err)
	}

	if res.StatusCode() != 200 {
		return nil, fmt.Errorf("registration request failed with status: %s", res.Status())
	}

	var registrationResponse RegistrationResponse

	err = json.Unmarshal(res.Body(), &registrationResponse)

	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal registration response: %w", err)
	}

	if !registrationResponse.Success {
		return nil, fmt.Errorf("registration request was not successful")
	}

	logrus.Infoln("Successfully registered with login server")

	return &registrationResponse, nil
}

// RoleDefinitions represents the structure for roles YAML/JSON
type RoleDefinitions struct {
	Version string                 `yaml:"version" json:"version"`
	Roles   map[string]models.Role `yaml:"roles" json:"roles"`
}

// WorkflowDefinitions represents the structure for workflows YAML/JSON
type WorkflowDefinitions struct {
	Version   string                     `yaml:"version" json:"version"`
	Workflows map[string]models.Workflow `yaml:"workflows" json:"workflows"`
}

type ProviderDefinitions struct {
	Version   string                     `yaml:"version" json:"version"`
	Providers map[string]models.Provider `yaml:"providers" json:"providers"`
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {

	v.SetDefault("config.path", "./config")

	v.SetDefault("environment.name", environment.DetectSystemName())
	v.SetDefault("environment.os", environment.DetectOperatingSystem())
	v.SetDefault("environment.os_version", environment.DetectOSVersion())
	v.SetDefault("environment.arch", runtime.GOARCH)
	v.SetDefault("environment.platform", environment.DetectPlatform())
	v.SetDefault("environment.ephemeral", environment.IsEphemeralEnvironment())

	// Environment config defaults
	v.SetDefault("environment.config.timeout", "5s")    // Timeout for any config fetch operations
	v.SetDefault("environment.config.key", "changeme")  // Default encryption key name
	v.SetDefault("environment.config.salt", "changeme") // Default encryption salt

	// Login server defaults
	v.SetDefault("login.endpoint", "https://login.thand.io")
	v.SetDefault("login.base", "/")

	// Server defaults
	v.SetDefault("server.host", "0.0.0.0")
	v.SetDefault("server.port", 5225)

	// API defaults
	v.SetDefault("api.version", "v1")

	// Metrics defaults
	v.SetDefault("server.metrics.enabled", true)
	v.SetDefault("server.metrics.path", "/metrics")
	v.SetDefault("server.metrics.namespace", "thand")

	// Health defaults
	v.SetDefault("server.health.enabled", true)
	v.SetDefault("server.health.path", "/health")

	// Security defaults
	v.SetDefault("server.cors.allowed_origins", []string{"https://thand.io", "https://*.thand.io"})
	v.SetDefault("server.cors.allowed_methods", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"})
	v.SetDefault("server.cors.allowed_headers", []string{"Authorization", "Content-Type", "X-Requested-With"})
	v.SetDefault("server.cors.max_age", 86400)

	// API defaults
	v.SetDefault("server.limits.read_timeout", "30s")
	v.SetDefault("server.limits.write_timeout", "30s")
	v.SetDefault("server.limits.idle_timeout", "120s")
	v.SetDefault("server.limits.requests_per_minute", 100)
	v.SetDefault("server.limits.burst", 10)

	// OIDC defaults
	v.SetDefault("oidc.scopes", []string{"openid", "profile", "email"})

	// Session defaults
	v.SetDefault("secret", "changeme")

	// Logging defaults
	v.SetDefault("logging.level", "info")
	v.SetDefault("logging.format", "json")
	v.SetDefault("logging.output", "stdout")

	// Where to load in roles and workflows from
	v.SetDefault("workflows.path", "./examples/workflows") // load any json or yaml files from this directory
	v.SetDefault("roles.path", "./examples/roles")         // load any json or yaml files from this directory
	v.SetDefault("providers.path", "./examples/providers") // load any json or yaml files from this directory

	// Allow a url to pull in roles and workflows
	// v.SetDefault("roles.url", "https://raw.githubusercontent.com/thand-io/agent/refs/heads/main/examples/roles/roles.yaml")
	// v.SetDefault("workflows.url", "https://raw.githubusercontent.com/thand-io/agent/refs/heads/main/examples/workflows/workflows.yaml")
	// v.SetDefault("providers.url", "https://raw.githubusercontent.com/thand-io/agent/refs/heads/main/examples/providers/providers.example.yaml")

}

func GetModuleBuildInfo() (string, string, bool) {
	if info, ok := debug.ReadBuildInfo(); ok {
		version := info.Main.Version
		var gitCommit string

		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				gitCommit = setting.Value
				break
			}
		}

		return version, gitCommit, true
	}
	return "", "", false
}
