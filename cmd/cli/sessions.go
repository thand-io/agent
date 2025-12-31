package cli

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/go-resty/resty/v2"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/thand-io/agent/internal/common"
)

// Session manager to let users from the command line add and remove sessions

// SessionAction represents available actions in the session manager
type SessionAction string

const (
	ActionListSessions   SessionAction = "list"
	ActionCreateSession  SessionAction = "create"
	ActionRemoveSession  SessionAction = "remove"
	ActionRefreshSession SessionAction = "refresh"
	ActionExit           SessionAction = "exit"
)

// sessionCmd represents the session command
var sessionCmd = &cobra.Command{
	Use:               "sessions",
	Short:             "Interactive sessions management",
	Long:              `Manage authentication sessions with providers interactively.`,
	PersistentPreRunE: preRunClientConfigWithServerE,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteractiveSessionManager()
	},
}

// sessionRegisterCmd represents the session register command
var sessionRegisterCmd = &cobra.Command{
	Use:   "register",
	Short: "Register a session from an encoded token",
	Long: `Register a session by pasting an encoded session token.
This allows you to import a session that was provided by another source.

Example:
  thand sessions register --provider thand`,
	PreRunE: preRunClientConfigE,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runSessionRegister(cmd)
	},
}

// sessionListCmd represents the session list command
var sessionListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all active sessions",
	Long: `Display all current authentication sessions with their status.

Shows provider name, session status (active/expired), expiry time,
and version information for each session.

Example:
  thand sessions list`,
	PreRunE: preRunClientConfigE,
	RunE: func(cmd *cobra.Command, args []string) error {
		return listSessions()
	},
}

// sessionCreateCmd represents the session create command
var sessionCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new authentication session",
	Long: `Create a new authentication session for a provider.

Guides you through selecting a provider and completing the
authentication flow in your browser.

Example:
  thand sessions create`,
	PreRunE: preRunServerE,
	RunE: func(cmd *cobra.Command, args []string) error {
		return createNewSession()
	},
}

// sessionRemoveCmd represents the session remove command
var sessionRemoveCmd = &cobra.Command{
	Use:   "remove",
	Short: "Remove an existing session",
	Long: `Remove an authentication session for a provider.

Displays a list of active sessions and prompts for confirmation
before removing the selected session.

Example:
  thand sessions remove`,
	PreRunE: preRunClientConfigE,
	RunE: func(cmd *cobra.Command, args []string) error {
		return removeSession()
	},
}

// sessionRefreshCmd represents the session refresh command
var sessionRefreshCmd = &cobra.Command{
	Use:   "refresh",
	Short: "Refresh an existing session",
	Long: `Refresh or re-authenticate an existing session.

Initiates the authentication flow again for the selected provider
to obtain a new session token with extended expiry.

Example:
  thand sessions refresh`,
	PreRunE: preRunServerE,
	RunE: func(cmd *cobra.Command, args []string) error {
		return refreshSession()
	},
}

func init() {
	rootCmd.AddCommand(sessionCmd)
	sessionCmd.AddCommand(sessionRegisterCmd)
	sessionCmd.AddCommand(sessionListCmd)
	sessionCmd.AddCommand(sessionCreateCmd)
	sessionCmd.AddCommand(sessionRemoveCmd)
	sessionCmd.AddCommand(sessionRefreshCmd)

	// Add flags for register command
	sessionRegisterCmd.Flags().String("provider", "", "Provider name (e.g., thand)")
}

// runInteractiveSessionManager starts the interactive session management interface
func runInteractiveSessionManager() error {
	fmt.Println(titleStyle.Render("Interactive Session Manager"))
	fmt.Println()

	for {
		action, err := promptForAction()
		if err != nil {
			return fmt.Errorf("failed to get action: %w", err)
		}

		switch action {
		case ActionListSessions:
			if err := listSessions(); err != nil {
				fmt.Println(errorStyle.Render("Failed to list sessions: " + err.Error()))
			}
		case ActionCreateSession:
			if err := createNewSession(); err != nil {
				fmt.Println(errorStyle.Render("Failed to create session: " + err.Error()))
			}
		case ActionRemoveSession:
			if err := removeSession(); err != nil {
				fmt.Println(errorStyle.Render("Failed to remove session: " + err.Error()))
			}
		case ActionRefreshSession:
			if err := refreshSession(); err != nil {
				fmt.Println(errorStyle.Render("Failed to refresh session: " + err.Error()))
			}
		case ActionExit:
			fmt.Println(successStyle.Render("Goodbye!"))
			return nil
		}

		fmt.Println()
		// Add a small pause for better UX
		time.Sleep(500 * time.Millisecond)
	}
}

// promptForAction prompts the user to select an action
func promptForAction() (SessionAction, error) {
	var action string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("What would you like to do?").
				Options(
					huh.NewOption("List all sessions", string(ActionListSessions)),
					huh.NewOption("Create new session", string(ActionCreateSession)),
					huh.NewOption("Remove session", string(ActionRemoveSession)),
					huh.NewOption("Refresh/Re-auth session", string(ActionRefreshSession)),
					huh.NewOption("Exit", string(ActionExit)),
				).
				Value(&action),
		),
	)

	err := form.Run()
	if err != nil {
		return ActionExit, err
	}

	return SessionAction(action), nil
}

// getAvailableProviders returns a list of available providers from the configuration
func getAvailableProviders() []huh.Option[string] {
	var options []huh.Option[string]

	for providerKey, provider := range cfg.Providers.Definitions {
		if len(provider.Name) == 0 {
			continue
		}

		description := provider.Description
		if len(description) == 0 {
			description = fmt.Sprintf("%s provider", provider.Provider)
		}

		label := fmt.Sprintf("%s - %s", provider.Name, description)
		options = append(options, huh.NewOption(label, providerKey))
	}

	return options
}

// formatDuration formats a duration in a human-readable way
func formatDuration(d time.Duration) string {
	if d < 0 {
		return "expired"
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60

	if hours > 24 {
		days := hours / 24
		hours = hours % 24
		if days == 1 {
			return fmt.Sprintf("%d day, %d hours", days, hours)
		}
		return fmt.Sprintf("%d days, %d hours", days, hours)
	}

	if hours > 0 {
		if hours == 1 {
			return fmt.Sprintf("%d hour, %d minutes", hours, minutes)
		}
		return fmt.Sprintf("%d hours, %d minutes", hours, minutes)
	}

	if minutes == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", minutes)
}

// authProviderKickStart initiates the authorization process for a given provider
func authProviderKickStart(
	selectedProvider string,
) error {

	localCallbackUrl := cfg.GetLocalServerUrl()

	loginServerUrl := fmt.Sprintf(
		"%s/auth/request/%s",
		cfg.DiscoverLoginServerApiUrl(cfg.GetLoginServerUrl()),
		selectedProvider,
	)

	clientIdentifier := common.GetClientIdentifier()

	client := resty.New()
	client.SetRedirectPolicy(handleProviderAuthRedirect())

	_, err := common.InvokeHttpRequestWithClient(
		client,
		&model.HTTPArguments{
			Method: http.MethodGet,
			Endpoint: &model.Endpoint{
				EndpointConfig: &model.EndpointConfiguration{
					URI: &model.LiteralUri{Value: loginServerUrl},
				},
			},
			Headers: map[string]string{
				"X-Client": clientIdentifier.String(),
			},
			Query: map[string]any{
				"callback": localCallbackUrl,
				"code":     createAuthCode(),
				"provider": strings.ToLower(selectedProvider),
			},
		},
	)

	if err != nil {

		// Check if resty.ErrAutoRedirectDisabled
		if errors.Is(err, resty.ErrAutoRedirectDisabled) {
			return nil
		}

		// Check if it's a resty error with response auto redirect is disabled
		return fmt.Errorf("failed to invoke kickstart request: %w", err)
	}

	return nil

}

func handleProviderAuthRedirect() resty.RedirectPolicy {
	return resty.RedirectPolicyFunc(func(req *http.Request, via []*http.Request) error {

		authUrl := req.URL.String()

		logrus.WithFields(logrus.Fields{
			"auth_url": authUrl,
		}).Debugln("Auth redirect")

		err := openBrowser(authUrl)

		if err != nil {

			fmt.Println(infoStyle.Render("Please open this URL in your browser:"))
			fmt.Println(authUrl)
			fmt.Println()

			return fmt.Errorf("failed to open browser: %w", err)
		}

		// Once we have opened the browser we can stop following redirects
		fmt.Println(infoStyle.Render("Waiting for authentication to complete..."))
		return resty.ErrAutoRedirectDisabled

	})
}
