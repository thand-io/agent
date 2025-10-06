package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/charmbracelet/huh"
	"github.com/spf13/cobra"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
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
	Use:     "sessions",
	Short:   "Interactive sessions management",
	Long:    `Manage authentication sessions with providers interactively.`,
	PreRunE: preRunServerE,
	RunE: func(cmd *cobra.Command, args []string) error {
		return runInteractiveSessionManager()
	},
}

func init() {
	rootCmd.AddCommand(sessionCmd)
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

// listSessions displays all current sessions with their status
func listSessions() error {
	fmt.Println(headerStyle.Render("Current Sessions"))
	fmt.Println()

	// Reload sessions to get the latest state
	if err := sessionManager.Load(cfg.GetLoginServerHostname()); err != nil {
		return fmt.Errorf("failed to load sessions: %w", err)
	}

	loginServer, err := sessionManager.GetLoginServer(cfg.GetLoginServerHostname())

	if err != nil {
		return fmt.Errorf("failed to get sessions for logon server: %w", err)
	}

	sessions := loginServer.GetSessions()

	if len(sessions) == 0 {
		fmt.Println(infoStyle.Render("ℹ️  No active sessions found"))
		return nil
	}

	currentTime := time.Now().UTC()

	for provider, session := range sessions {
		providerDisplay := headerStyle.Render(fmt.Sprintf("Provider: %s", provider))

		var statusDisplay string
		var expiryDisplay string

		sessionExpiryTime := session.Expiry.UTC()
		if sessionExpiryTime.Before(currentTime) {
			statusDisplay = expiredStyle.Render("EXPIRED")
			expiryDisplay = expiredStyle.Render(fmt.Sprintf("Expired: %s", session.Expiry.Format("2006-01-02 15:04:05")))
		} else {
			statusDisplay = activeStyle.Render("ACTIVE")
			timeUntilExpiry := time.Until(session.Expiry)
			expiryDisplay = activeStyle.Render(fmt.Sprintf("Expires: %s (%s)",
				session.Expiry.Format("2006-01-02 15:04:05"),
				formatDuration(timeUntilExpiry)))
		}

		versionDisplay := infoStyle.Render(fmt.Sprintf("Version: %d", session.Version))

		fmt.Println(providerDisplay)
		fmt.Println("  " + statusDisplay)
		fmt.Println("  " + expiryDisplay)
		fmt.Println("  " + versionDisplay)
		fmt.Println()
	}

	return nil
}

// createNewSession guides the user through creating a new session
func createNewSession() error {
	fmt.Println(headerStyle.Render("Create New Session"))
	fmt.Println()

	// Get available providers from config
	providers := getAvailableProviders()
	if len(providers) == 0 {
		fmt.Println(warningStyle.Render("No providers available in configuration"))
		return nil
	}

	var selectedProvider string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select a provider:").
				Description("Choose the provider you want to create a session for").
				Options(providers...).
				Value(&selectedProvider),
		),
	)

	err := form.Run()
	if err != nil {
		return err
	}

	if len(selectedProvider) == 0 {
		fmt.Println(warningStyle.Render("No provider selected"))
		return nil
	}

	// Check if session already exists
	existingSession, _ := sessionManager.GetSession(cfg.GetLoginServerHostname(), selectedProvider)
	if existingSession != nil {
		now := time.Now().UTC()
		if existingSession.Expiry.After(now) {
			var overwrite bool
			overwriteForm := huh.NewForm(
				huh.NewGroup(
					huh.NewConfirm().
						Title("Session already exists").
						Description(fmt.Sprintf("An active session for %s already exists. Do you want to replace it?", selectedProvider)).
						Value(&overwrite),
				),
			)

			if err := overwriteForm.Run(); err != nil {
				return err
			}

			if !overwrite {
				fmt.Println(infoStyle.Render("ℹ️  Session creation cancelled"))
				return nil
			}
		}
	}

	fmt.Println(infoStyle.Render("Starting authentication flow..."))
	fmt.Println(infoStyle.Render("Please complete the authentication in your browser"))
	fmt.Println()

	// Get provider implementation
	providerImpl, err := cfg.GetProviderByName(selectedProvider)
	if err != nil {
		return fmt.Errorf("provider not found: %w", err)
	}

	localCallbackUrl := cfg.GetLocalServerUrl()
	client := common.GetClientIdentifier()

	// TODO FIX ME
	// Start the authentication flow by getting the authorization URL
	authResponse, err := providerImpl.GetClient().AuthorizeSession(context.Background(), &models.AuthorizeUser{
		Scopes: []string{"email", "profile"},
		State: models.EncodingWrapper{
			Type: models.ENCODED_AUTH,
			Data: models.NewAuthWrapper(localCallbackUrl, client, selectedProvider),
		}.Encode(),
		RedirectUri: cfg.GetAuthCallbackUrl(selectedProvider),
	})

	if err != nil {
		return fmt.Errorf("failed to start authorization: %w", err)
	}

	if len(authResponse.Url) > 0 {
		err := openBrowser(authResponse.Url)

		if err != nil {
			fmt.Println(infoStyle.Render("Please open this URL in your browser:"))
			fmt.Println(authResponse.Url)
			fmt.Println()
		}

		fmt.Println(infoStyle.Render("Waiting for authentication to complete..."))
	}

	// Wait for the session to be created (polling)
	fmt.Println(infoStyle.Render("Waiting for session to be created..."))
	newSession := sessionManager.AwaitProviderRefresh(
		cfg.GetLoginServerHostname(),
		selectedProvider,
	)

	if newSession == nil {
		return fmt.Errorf("authentication timed out or failed")
	}

	fmt.Println(successStyle.Render("Session created successfully!"))
	fmt.Printf("Provider: %s\n", selectedProvider)
	fmt.Printf("Expires: %s\n", newSession)

	return nil
}

// removeSession allows the user to remove an existing session
func removeSession() error {
	fmt.Println(headerStyle.Render("Remove Session"))
	fmt.Println()

	// Reload sessions to get the latest state
	if err := sessionManager.Load(cfg.GetLoginServerHostname()); err != nil {
		return fmt.Errorf("failed to load sessions: %w", err)
	}

	loginServer, err := sessionManager.GetLoginServer(cfg.GetLoginServerHostname())
	if err != nil {
		return fmt.Errorf("failed to get sessions: %w", err)
	}

	sessions := loginServer.GetSessions()

	if len(sessions) == 0 {
		fmt.Println(infoStyle.Render("ℹ️  No sessions to remove"))
		return nil
	}

	// Build options from existing sessions
	var options []huh.Option[string]
	now := time.Now().UTC()

	for provider, session := range sessions {
		var label string
		if session.Expiry.Before(now) {
			label = fmt.Sprintf("%s (EXPIRED - %s)", provider, session.Expiry.Format("2006-01-02 15:04"))
		} else {
			timeUntilExpiry := time.Until(session.Expiry)
			label = fmt.Sprintf("%s (expires in %s)", provider, formatDuration(timeUntilExpiry))
		}
		options = append(options, huh.NewOption(label, provider))
	}

	var selectedProvider string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select session to remove:").
				Options(options...).
				Value(&selectedProvider),
		),
	)

	err = form.Run()
	if err != nil {
		return err
	}

	if len(selectedProvider) == 0 {
		fmt.Println(warningStyle.Render("No session selected"))
		return nil
	}

	// Confirm removal
	var confirm bool
	confirmForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Confirm removal").
				Description(fmt.Sprintf("Are you sure you want to remove the session for %s?", selectedProvider)).
				Value(&confirm),
		),
	)

	if err := confirmForm.Run(); err != nil {
		return err
	}

	if !confirm {
		fmt.Println(infoStyle.Render("ℹ️  Session removal cancelled"))
		return nil
	}

	// Remove the session
	if err := sessionManager.RemoveSession(cfg.GetLoginServerHostname(), selectedProvider); err != nil {
		return fmt.Errorf("failed to remove session: %w", err)
	}

	fmt.Println(successStyle.Render("Session removed successfully!"))

	return nil
}

// refreshSession allows the user to refresh/re-authenticate an existing session
func refreshSession() error {
	fmt.Println(headerStyle.Render("Refresh Session"))
	fmt.Println()

	// Reload sessions to get the latest state
	if err := sessionManager.Load(cfg.GetLoginServerHostname()); err != nil {
		return fmt.Errorf("failed to load sessions: %w", err)
	}

	loginServer, err := sessionManager.GetLoginServer(cfg.GetLoginServerHostname())
	if err != nil {
		return fmt.Errorf("failed to get sessions: %w", err)
	}

	sessions := loginServer.GetSessions()

	if len(sessions) == 0 {
		fmt.Println(infoStyle.Render("ℹ️  No sessions to refresh"))
		return nil
	}

	// Build options from existing sessions
	var options []huh.Option[string]
	currentTime := time.Now().UTC()

	for provider, session := range sessions {
		var label string
		sessionExpiryTime := session.Expiry.UTC()
		if sessionExpiryTime.Before(currentTime) {
			label = fmt.Sprintf("%s (EXPIRED - %s)", provider, session.Expiry.Format("2006-01-02 15:04"))
		} else {
			timeUntilExpiry := time.Until(session.Expiry)
			label = fmt.Sprintf("%s (expires in %s)", provider, formatDuration(timeUntilExpiry))
		}
		options = append(options, huh.NewOption(label, provider))
	}

	var selectedProvider string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select session to refresh:").
				Options(options...).
				Value(&selectedProvider),
		),
	)

	err = form.Run()
	if err != nil {
		return err
	}

	if len(selectedProvider) == 0 {
		fmt.Println(warningStyle.Render("No session selected"))
		return nil
	}

	fmt.Println(infoStyle.Render("Starting re-authentication flow..."))
	fmt.Println(infoStyle.Render("Please complete the authentication in your browser"))
	fmt.Println()

	// Get provider implementation
	providerImpl, err := cfg.GetProviderByName(selectedProvider)
	if err != nil {
		return fmt.Errorf("provider not found: %w", err)
	}

	localCallbackUrl := cfg.GetLocalServerUrl()
	client := common.GetClientIdentifier()

	// Start the authentication flow
	// TODO FIX ME
	authResponse, err := providerImpl.GetClient().AuthorizeSession(context.Background(), &models.AuthorizeUser{
		Scopes: []string{"email", "profile"},
		State: models.EncodingWrapper{
			Type: models.ENCODED_AUTH,
			Data: models.NewAuthWrapper(localCallbackUrl, client, selectedProvider),
		}.Encode(),
		RedirectUri: cfg.GetAuthCallbackUrl(selectedProvider),
	})

	if err != nil {
		return fmt.Errorf("failed to start authorization: %w", err)
	}

	if len(authResponse.Url) > 0 {
		err := openBrowser(authResponse.Url)

		if err != nil {
			fmt.Println(infoStyle.Render("Please open this URL in your browser:"))
			fmt.Println(authResponse.Url)
			fmt.Println()
		}

		fmt.Println(infoStyle.Render("Waiting for authentication to complete..."))
	}

	// Wait for the session to be refreshed
	fmt.Println(infoStyle.Render("Waiting for session to be refreshed..."))
	refreshedSession := sessionManager.AwaitProviderRefresh(cfg.GetLoginServerHostname(), selectedProvider)

	if refreshedSession == nil {
		return fmt.Errorf("authentication timed out or failed")
	}

	fmt.Println(successStyle.Render("Session refreshed successfully!"))
	fmt.Printf("Provider: %s\n", selectedProvider)
	fmt.Printf("New expiry: %s\n", refreshedSession)

	return nil
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
