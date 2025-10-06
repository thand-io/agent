package cli

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"github.com/thand-io/agent/internal/agent"
)

// serverCmd represents the server command
var serverCmd = &cobra.Command{
	Use:   "server",
	Short: "Run the agent server",
	Long: `Start the Thand Agent server directly in the foreground.
This will run the web service that handles authentication and authorization requests.`,
	PersistentPreRunE: preRunServerConfigE,
	Run: func(cmd *cobra.Command, args []string) {
		// Check if configuration is loaded
		if cfg == nil {
			fmt.Println("Configuration not loaded")
			os.Exit(1)
		}

		// Print out environment information
		fmt.Printf("Environment Name: %s\n", cfg.Environment.Name)
		fmt.Printf("Environment Platform: %s\n", cfg.Environment.Platform)
		fmt.Printf("Environment OS: %s\n", cfg.Environment.OperatingSystem)
		fmt.Printf("Environment OS Version: %s\n", cfg.Environment.OperatingSystemVersion)
		fmt.Printf("Environment Architecture: %s\n", cfg.Environment.Architecture)

		// Set up signal handling for graceful shutdown
		sigChan := make(chan os.Signal, 1)
		signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

		// Start the web service in a goroutine
		errChan := make(chan error, 1)
		fmt.Println("Starting Thand Agent server...")

		server, err := agent.StartWebService(cfg)
		if err != nil {
			fmt.Printf("Server failed to start: %v\n", err)
			os.Exit(1)
		}

		// Wait for either an error or a shutdown signal
		select {
		case err := <-errChan:
			fmt.Printf("Server error: %v\n", err)
			os.Exit(1)
		case sig := <-sigChan:
			fmt.Printf("\nReceived signal %v, shutting down gracefully...\n", sig)
			server.Stop()
			fmt.Println("Server stopped")
		}
	},
}

func init() {
	rootCmd.AddCommand(serverCmd) // Run server directly
}
