package cli

import (
	"fmt"
	"net/url"

	"github.com/spf13/cobra"
)

var loginCmd = &cobra.Command{
	Use:   "login",
	Short: "Authenticate with the login server",
	Long:  "Opens a browser to authenticate with the login server and establishes a session",
	PreRunE: func(cmd *cobra.Command, args []string) error {

		err := preRunClientConfigE(cmd, args)
		if err != nil {
			return err
		}
		err = preRunServerE(cmd, args)
		if err != nil {
			return err
		}
		return nil
	},
	RunE: runLogin,
}

func runLogin(cmd *cobra.Command, args []string) error {

	hostname := cfg.GetLoginServerHostname()
	fmt.Println("Login server hostname:", hostname)

	callbackUrl := url.Values{
		"callback": {cfg.GetLocalServerUrl()},
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
		cfg.GetLoginServerHostname(),
	)

	if session == nil {
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
