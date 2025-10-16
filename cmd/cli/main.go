package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/kardianos/service"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/thand-io/agent/internal/agent"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/sessions"
)

// Global configuration instance
var cfg *config.Config
var sessionManager *sessions.SessionManager

// loadConfig loads the configuration based on the --config flag or default locations
func loadConfig(cmd *cobra.Command) (*config.Config, error) {
	configFile, err := cmd.Flags().GetString("config")

	if err != nil {
		return nil, fmt.Errorf("failed to get config flag: %w", err)
	}

	return config.Load(configFile)
}

func loadUserSessionState(logonServer string) *sessions.SessionManager {
	sessions := sessions.GetSessionManager()
	sessions.Load(logonServer)
	return sessions
}

func preRunClientConfigE(cmd *cobra.Command, _ []string) error {
	return preRunConfigE(cmd, config.ModeClient)
}

func preRunServerConfigE(cmd *cobra.Command, _ []string) error {
	return preRunConfigE(cmd, config.ModeServer)
}

func preRunAgentConfigE(cmd *cobra.Command, _ []string) error {
	return preRunConfigE(cmd, config.ModeAgent)
}

func preRunConfigE(cmd *cobra.Command, mode config.Mode) error {
	// Load configuration before any command runs
	var err error
	cfg, err = loadConfig(cmd)

	if err != nil {
		return fmt.Errorf("failed to load configuration: %w", err)
	}

	cfg.SetMode(mode)

	switch mode {
	case config.ModeClient:

		// Get the login server override from the flag
		loginServer, err := cmd.Flags().GetString("login-server")
		if err == nil && len(loginServer) > 0 {
			err := cfg.SetLoginServer(loginServer)
			if err != nil {
				return fmt.Errorf("failed to set login server: %w", err)
			}
		}

		// Get the API key override from the flag
		apiKey, err := cmd.Flags().GetString("api-key")
		if err == nil && len(apiKey) > 0 {
			err := cfg.SetAPIKey(apiKey)
			if err != nil {
				return fmt.Errorf("failed to set API key: %w", err)
			}
		}

		// Load users session state before any command runs
		sessionManager = loadUserSessionState(cfg.GetLoginServerHostname())

	case config.ModeServer:

		err = cfg.ReloadConfig()

		if err != nil {
			logrus.WithError(err).Errorln("Failed to sync configuration with login server")
		}

	}
	return nil
}

func preRunServerE(cmd *cobra.Command, args []string) error {
	// Load configuration before any command runs
	// Check if there is a registered daemon running
	// if not then spin up the local service
	daemon, err := agent.CreateService(cfg)

	if err == nil {

		status, err := daemon.Status()

		if err == nil {

			if status == service.StatusRunning {
				logrus.Debug("Daemon service is running, connecting to it...")
				return err
			}
		}
	}

	// If the service isn't running then just spin up a local one
	fmt.Println("Service not running, starting local web service...")
	agent.StartWebService(cfg)

	return nil
}

// promptAndLogin prompts the user if they want to login and handles the login process
func promptAndLogin(cmd *cobra.Command) error {
	fmt.Println()
	fmt.Println(titleStyle.Render("Authentication Required"))
	fmt.Println("No active login session found.")
	fmt.Println()

	var shouldLogin bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Would you like to login now?").
				Description("This will open your browser to authenticate with the login server").
				Value(&shouldLogin),
		),
	)

	err := form.Run()
	if err != nil {
		return fmt.Errorf("login prompt cancelled: %w", err)
	}

	if shouldLogin {
		fmt.Println()
		fmt.Println("Starting login process...")

		// Call the login function directly
		err = runLogin(cmd, []string{})
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}

		// After successful login, try to sync again
		err = cfg.SyncWithLoginServer()
		if err != nil {
			return fmt.Errorf("failed to sync configuration after login: %w", err)
		}
	} else {
		return fmt.Errorf("authentication required but login was declined")
	}

	return nil
}

func preAuthenticateE(cmd *cobra.Command, _ []string) error {

	// Now we have our session sync the remote state
	err := cfg.SyncWithLoginServer()

	if err != nil {
		if errors.Is(err, config.ErrNoActiveLoginSession) {
			return promptAndLogin(cmd)
		} else {
			logrus.WithError(err).Errorln("Failed to sync configuration with login server")
			return fmt.Errorf("failed to sync configuration with login server: %w", err)
		}
	}

	return nil
}

func preAgentE(cmd *cobra.Command, args []string) error {

	err := preAuthenticateE(cmd, args)
	if err != nil {
		return err
	}
	err = preRunServerE(cmd, args) // load config and authenticate using
	if err != nil {
		return err
	}
	return nil
}

var rootCmd = &cobra.Command{
	Use:   "agent",
	Short: "Thand Agent - Just-in-time access to cloud infrastructure and SaaS applications",
	Long: `Thand Agent eliminates standing access to critical infrastructure and SaaS apps.
Instead of permanent admin rights, users request access when needed, for only as long as needed.

Complete documentation is available at https://docs.thand.io`,
	PersistentPreRunE: preRunClientConfigE,
	PreRunE:           preAgentE,
	RunE: func(cmd *cobra.Command, args []string) error {

		// When nothing is specified. First check if a login-server is configured
		// if not then start the setup.
		if cfg == nil || len(cfg.Login.Endpoint) == 0 {
			fmt.Println("No login server configured. Starting setup...")
			fmt.Println("Please configure your login server endpoint in config.yaml")
			fmt.Println("Example:")
			fmt.Println("  login:")
			fmt.Println("    endpoint: https://your-login-server.com")
			return fmt.Errorf("no login server configured")
		}

		// if a login-server has been configured then start the cli in interactive mode
		// to ask what role the user wants and what resource they want access to.
		data, err := RunRequestWizard(cfg)
		if err != nil {
			fmt.Printf("Wizard failed: %v\n", err)
			os.Exit(1)
		}

		err = MakeElevationRequest(data)
		if err != nil {
			fmt.Printf("Elevation request failed: %v\n", err)
			os.Exit(1)
		}

		return nil
	},
}

/*
The agent runs in two modes:

A local CLI mode that allows the user to request access
to a given provider as defined by the remote login-server.

The agent mode which takes a defined set of available
roles and workflows which outlines what the user can do with
the given provider. The agent can be configured to provide access to
any of the configured integrations such as AWS, GCP, Azure, Snowflake.
Agents can run anywhere. Their configuration defines what they have access to
and what access they can provide by the workflows defined.

- SaaS access (Salesforce, Zendesk, Jira)
- Cloud access (AWS, GCP, Azure)
- Kubernetes access (EKS, GKE, AKS)
- Local access elevations (Linux, MacOS, Windows)
*/

func init() {

	// Add global flags
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable verbose output")
	rootCmd.PersistentFlags().String("config", "", "Config file (default is $HOME/.thand/config.yaml)")
	// Add the login-server flag
	rootCmd.PersistentFlags().String("login-server", "", "Override the default login server URL (e.g., http://localhost:8080)")
	rootCmd.PersistentFlags().String("api-key", "", "Provide an API key for authentication with the login server")

}

func GetCommandOptions() *cobra.Command {
	return rootCmd
}
