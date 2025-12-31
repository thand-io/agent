package cli

import (
	"context"
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

var loginCmd = &cobra.Command{
	Use:     "login",
	Short:   "Authenticate with the login server",
	Long:    "Opens a browser to authenticate with the login server and establishes a session",
	PreRunE: preRunClientConfigWithServerE,
	RunE:    runLogin,
}

func runLogin(cmd *cobra.Command, args []string) error {
	return authKickStart()
}

func authKickStart() error {
	// Set up signal handling for graceful cancellation
	ctx, cleanup := common.WithInterrupt(context.Background())
	defer cleanup()

	hostname := cfg.GetLoginServerHostname()
	fmt.Println("Login server hostname:", hostname)

	// Prepare callback URL with local server endpoint

	if !cfg.GetServices().HasEncryption() {
		return fmt.Errorf("encryption service is not configured")
	}

	callbackUrl := url.Values{
		"callback": {cfg.GetLocalServerUrl()},
		"code":     {createAuthCode()},
	}

	// Use the configured login server if no override provided
	authUrl := fmt.Sprintf("%s/auth?%s", cfg.GetLoginServerUrl(), callbackUrl.Encode())

	fmt.Printf("Opening browser to: %s with callback to: %s\n", authUrl, cfg.GetLocalServerUrl())

	// Open the browser to the auth endpoint
	err := openBrowser(authUrl)
	if err != nil {
		return fmt.Errorf("failed to open browser: %w", err)
	}

	fmt.Println("Waiting for authentication callback...")

	// Wait for the session to be established (using empty provider for general login)
	session := sessionManager.AwaitRefresh(
		ctx,
		cfg.GetLoginServerHostname(),
	)

	if session == nil {
		// Check if context was cancelled
		if ctx.Err() != nil {
			return fmt.Errorf("login cancelled")
		}
		return fmt.Errorf("authentication failed or timed out")
	}

	fmt.Println()
	fmt.Println(successStyle.Render("Login successful!"))
	fmt.Printf("Login server: %s\n", session.Timestamp.Format("2006-01-02 15:04:05"))
	fmt.Println()

	return nil
}

func init() {
	// Add the command to the root
	rootCmd.AddCommand(loginCmd)
}

func createAuthCode() string {
	code := models.EncodingWrapper{
		Type: models.ENCODED_SESSION_CODE,
		Data: models.NewCodeWrapper(
			cfg.GetLoginServerUrl(),
		),
	}.EncodeAndEncrypt(
		cfg.GetServices().GetEncryption(),
	)

	return code
}
