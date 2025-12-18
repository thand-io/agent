package config

import (
	"context"
	"encoding/json"
	"errors"
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
	"time"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/subosito/gotenv"

	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/config/environment"
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
		logrus.Fatalf("Failed to get current user: %v", err)
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

	// Thand environment variables
	v.BindEnv("thand.endpoint", "THAND_ENDPOINT")
	v.BindEnv("thand.base", "THAND_BASE_PATH")
	v.BindEnv("thand.api_key", "THAND_API_KEY")

	// Set base environment variables
	v.BindEnv("login.endpoint", "THAND_LOGIN_ENDPOINT")
	v.BindEnv("login.endpoint", "THAND_BASE_URL")

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
	v.BindEnv("environment.config.access_key_id", "THAND_ENVIRONMENT_CONFIG_ACCESS_KEY_ID")
	v.BindEnv("environment.config.secret_access_key", "THAND_ENVIRONMENT_CONFIG_SECRET_ACCESS_KEY")
	v.BindEnv("environment.config.kms_arn", "THAND_ENVIRONMENT_CONFIG_KMS_ARN")
	v.BindEnv("environment.config.imds_disable", "THAND_ENVIRONMENT_CONFIG_IMDS_DISABLE")
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
	var foundErrors []error

	// Load roles in parallel
	wg.Go(func() {
		roles, err := c.LoadRoles()
		if err != nil {
			logrus.WithError(err).Errorln("Error loading roles")
			c.mu.Lock()
			foundErrors = append(foundErrors, fmt.Errorf("loading roles: %w", err))
			c.mu.Unlock()
		} else if len(roles) > 0 {
			logrus.Infoln("Loaded roles from external source:", len(roles))
			c.mu.Lock()
			c.Roles.Definitions = roles
			c.mu.Unlock()
		} else {
			logrus.Warningln("No roles loaded from external source")
		}
	})

	// Load workflows in parallel
	wg.Go(func() {
		workflows, err := c.LoadWorkflows()
		if err != nil {
			logrus.WithError(err).Errorln("Error loading workflows")
			c.mu.Lock()
			foundErrors = append(foundErrors, fmt.Errorf("loading workflows: %w", err))
			c.mu.Unlock()
		} else if len(workflows) > 0 {
			logrus.Infoln("Loaded workflows from external source:", len(workflows))
			c.mu.Lock()
			c.Workflows.Definitions = workflows
			c.mu.Unlock()
		} else {
			logrus.Warningln("No workflows loaded from external source")
		}
	})

	// Load providers in parallel
	wg.Go(func() {
		providers, err := c.LoadProviders()
		if err != nil {
			logrus.WithError(err).Errorln("Error loading providers")
			c.mu.Lock()
			foundErrors = append(foundErrors, fmt.Errorf("loading providers: %w", err))
			c.mu.Unlock()
		} else if len(providers) > 0 {
			logrus.Infoln("Loaded providers from external source:", len(providers))
			c.mu.Lock()
			c.Providers.Definitions = providers
			c.mu.Unlock()
		} else {
			logrus.Warningln("No providers loaded from external source")
		}
	})

	// Wait for all goroutines to complete
	wg.Wait()

	// Return first error if any occurred
	if len(foundErrors) > 0 {
		return errors.Join(foundErrors...)
	}

	if c.GetServices() != nil && c.GetServices().GetTemporal() != nil {

		logrus.Infoln("Setting up temporal services...")
		err := c.setupTemporalServices()

		if err != nil {
			return fmt.Errorf("setting up temporal services: %w", err)
		}

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

		logrus.Debugf("Looking for valid session to sync with login server at: %s", c.GetLoginServerUrl())
		localSessions := loginServer.GetSessions()

		// Find the first non-expired session token
		for providerName, session := range localSessions {
			if !session.IsExpired() {

				logrus.Debugf("Found valid session for provider '%s'", providerName)
				localToken = session.GetEncodedLocalSession()

				if len(session.Endpoint) > 0 && !strings.EqualFold(session.Endpoint, c.GetLoginServerUrl()) {
					logrus.Infof("Updating login server URL from session endpoint: %s", session.Endpoint)
					c.Login.Endpoint = session.Endpoint
				}

				break
			}
		}

		if len(localToken) == 0 {
			return ErrNoActiveLoginSession
		}

	}

	apiUrl := c.DiscoverLoginServerApiUrl(
		c.GetLoginServerUrl(),
	)

	// Lets make our registration request. This will pull down our
	// remote configuration and also register this instance with the login server

	_, err = c.RegisterWithLoginServer(localToken)

	if err != nil {
		return fmt.Errorf("failed to register with login server: %w", err)
	}

	logrus.Debugf("Syncing configuration with login server at: %s", apiUrl)

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

	// Now lets initialize our providers
	err = c.InitializeProviders()

	if err != nil {
		logrus.WithError(err).Errorln("Failed to initialize providers after login server sync")
	}

	return nil

}

// RegisterWithThandServer registers the agent with the thand.io server
func (c *Config) RegisterWithThandServer() error {

	if !c.IsServer() {
		return fmt.Errorf("thand services can only be set up in server mode")
	}

	if !c.HasThandService() {
		logrus.Debugln("Thand endpoint not configured, skipping registration with thand server")
		return nil
	}

	/*

		1. Discover thand server API URL
		2. Register with thand server
		3. Setup temporal services if enabled
		4. Get back the roles, providers and workflow definitions
		5. Check this incoming config version against local version
		6. If remote version is newer then update local config
		6.1. Commit the new config version to memory
		7. If local version is newer then push local config to server
		8. Check for provider roles, users, permissions etc. If these
		   do not exist on the server then create them.
		9. Reload config afterwards. This will start to pull down all,
		   provider, users, permisions roles etc.
		10. Upload all these provider roles, permissions etc to the server
		11. Return

	*/

	thandLoginUrl := c.DiscoverThandServerApiUrl()
	registration, err := c.syncWithEndpoint(thandLoginUrl, c.Thand.ApiKey)

	if err != nil {
		return fmt.Errorf("failed to register with thand server: %w", err)
	}

	// Now that we are registered and configured. We need to compare our local config
	// with the remote config and update if needed. We need to compare the version
	// numbers

	err = c.MergeConfiguration(registration)

	if err != nil {
		logrus.WithError(err).Errorln("Failed to merge configuration from thand server")
	}

	return nil

}

func (c *Config) RegisterWithLoginServer(localToken string) (*RegistrationResponse, error) {

	loginUrl := c.DiscoverLoginServerApiUrl(
		c.GetLoginServerUrl(),
	)

	return c.syncWithEndpoint(loginUrl, localToken)

}

func (c *Config) syncWithEndpoint(loginUrl, localToken string) (*RegistrationResponse, error) {

	version, commit, _ := common.GetModuleBuildInfo()

	preflightBody, err := json.Marshal(PreflightRequest{
		Mode:       c.GetMode(),
		Version:    version,
		Commit:     commit,
		Identifier: common.GetClientIdentifier(),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to marshal registration request: %w", err)
	}

	authentication := &model.ReferenceableAuthenticationPolicy{
		AuthenticationPolicy: &model.AuthenticationPolicy{
			Bearer: &model.BearerAuthenticationPolicy{
				Token: localToken,
			},
		},
	}

	// Pre-flight check
	preflightRes, err := common.InvokeHttpRequest(&model.HTTPArguments{
		Method: http.MethodPost,
		Endpoint: &model.Endpoint{
			EndpointConfig: &model.EndpointConfiguration{
				URI:            &model.LiteralUri{Value: loginUrl + "/preflight"},
				Authentication: authentication,
			},
		},
		Body: preflightBody,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to invoke preflight: %w", err)
	}

	if preflightRes.StatusCode() != 200 {
		return nil, fmt.Errorf("preflight %s failed with status: %s", loginUrl+"/preflight", preflightRes.Status())
	}

	reqBody, err := json.Marshal(RegistrationRequest{
		Mode:        c.GetMode(),
		Environment: &c.Environment,
		Version:     version,
		Commit:      commit,
		Identifier:  common.GetClientIdentifier(),
	})

	if err != nil {
		return nil, fmt.Errorf("failed to marshal registration request: %w", err)
	}

	// No need for an API key we need to use the session
	// info
	registerRes, err := common.InvokeHttpRequest(&model.HTTPArguments{
		Method: http.MethodPost,
		Endpoint: &model.Endpoint{
			EndpointConfig: &model.EndpointConfiguration{
				URI:            &model.LiteralUri{Value: loginUrl + "/register"},
				Authentication: authentication,
			},
		},
		Body: reqBody,
	})

	if err != nil {
		return nil, fmt.Errorf("failed to invoke registration request: %w", err)
	}

	if registerRes.StatusCode() != 200 {
		return nil, fmt.Errorf("registration request %s failed with status: %s", loginUrl+"/register", registerRes.Status())
	}

	var registrationResponse RegistrationResponse
	err = json.Unmarshal(registerRes.Body(), &registrationResponse)

	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal registration response: %w", err)
	}

	if !registrationResponse.Success {
		return nil, fmt.Errorf("registration request was not successful")
	}

	logrus.Infoln("Successfully registered with login server")

	// Setup OpenTelemetry remote logging if endpoint is provided
	if registrationResponse.Logging != nil {

		logrus.Debugln("Configuring remote logging from login server configuration")

		loggingConfig := registrationResponse.Logging

		if loggingConfig.OpenTelemetry.Enabled && loggingConfig.OpenTelemetry.Endpoint.EndpointConfig != nil {

			endpointConfig := loggingConfig.OpenTelemetry.Endpoint.EndpointConfig

			if endpointConfig.Authentication == nil {
				endpointConfig.Authentication = authentication
			}

			logrus.Infof("Enabling remote logging to endpoint: %s", endpointConfig.URI.String())

			// Create context with cancellation and timeout
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()

			// Enable remote logging
			err := c.logger.EnableRemoteLogging(
				ctx,
				loggingConfig.OpenTelemetry.Endpoint,
				common.GetClientIdentifier(),
			)
			if err != nil {
				logrus.WithError(err).Warn("Failed to setup remote logging, continuing without it")
			}
		}
	}

	return &registrationResponse, nil
}

// setDefaults sets default configuration values
func setDefaults(v *viper.Viper) {

	v.SetDefault("config.path", "./config")

	v.SetDefault("environment.name", environment.DetectSystemName())
	v.SetDefault("environment.hostname", environment.DetectHostname())
	v.SetDefault("environment.os", environment.DetectOperatingSystem())
	v.SetDefault("environment.os_version", environment.DetectOSVersion())
	v.SetDefault("environment.arch", runtime.GOARCH)
	v.SetDefault("environment.platform", environment.DetectPlatform())
	v.SetDefault("environment.ephemeral", environment.IsEphemeralEnvironment())

	// Environment config defaults
	v.SetDefault("environment.config.timeout", "5s")                    // Timeout for any config fetch operations
	v.SetDefault("environment.config.key", common.DefaultServerSecret)  // Default encryption key name
	v.SetDefault("environment.config.salt", common.DefaultServerSecret) // Default encryption salt

	// Thand upstream service defaults
	v.SetDefault("thand.endpoint", common.DefaultThandEndpoint)
	v.SetDefault("thand.base", "/")
	v.SetDefault("thand.api_key", "")

	// Login server defaults
	v.SetDefault("login.endpoint", common.DefaultLoginServerEndpoint)
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

	// Ready defaults
	v.SetDefault("server.ready.enabled", true)
	v.SetDefault("server.ready.path", "/ready")

	// Security defaults
	v.SetDefault("server.security.cors.allowed_origins", []string{"https://thand.io", "https://*.thand.io", "https://app.thand.io", "https://*.app.thand.io"})
	v.SetDefault("server.security.cors.allowed_methods", []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"})
	v.SetDefault("server.security.cors.allowed_headers", []string{"Authorization", "Content-Type", "X-Requested-With"})
	v.SetDefault("server.security.cors.max_age", 86400)

	// API defaults
	v.SetDefault("server.limits.read_timeout", "30s")
	v.SetDefault("server.limits.write_timeout", "30s")
	v.SetDefault("server.limits.idle_timeout", "120s")
	v.SetDefault("server.limits.requests_per_minute", 100)
	v.SetDefault("server.limits.burst", 10)

	// OIDC defaults
	v.SetDefault("oidc.scopes", []string{"openid", "profile", "email"})

	// Session defaults
	v.SetDefault("secret", common.DefaultServerSecret)

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
