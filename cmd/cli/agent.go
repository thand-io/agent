package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
	"github.com/thand-io/agent/internal/agent"
	"github.com/thand-io/agent/internal/common"
)

// agentCmd represents the agent command
var agentCmd = &cobra.Command{
	Use:   "agent",
	Short: "Run the Thand Agent",
	Long: `Start the Thand Agent directly in the foreground.
This will run the web service that handles authentication and authorization requests.`,
	Hidden:            true,
	PersistentPreRunE: preRunAgentConfigE,
	Run: func(cmd *cobra.Command, args []string) {
		// Check if configuration is loaded
		if cfg == nil {
			fmt.Println("Configuration not loaded")
			os.Exit(1)
		}

		// Print out environment information
		fmt.Printf("Environment Name: %s\n", cfg.Environment.Name)
		fmt.Printf("Environment Hostname: %s\n", cfg.Environment.Hostname)
		fmt.Printf("Environment Platform: %s\n", cfg.Environment.Platform)
		fmt.Printf("Environment OS: %s\n", cfg.Environment.OperatingSystem)
		fmt.Printf("Environment OS Version: %s\n", cfg.Environment.OperatingSystemVersion)
		fmt.Printf("Environment Architecture: %s\n", cfg.Environment.Architecture)

		// Set up signal handling for graceful shutdown
		sigChan, cleanup := common.NewInterruptChannel()
		defer cleanup()

		// Start the web service in a goroutine
		errChan := make(chan error, 1)
		fmt.Println("Starting Thand Agent...")

		agent, err := agent.StartWebService(cfg)
		if err != nil {
			fmt.Printf("Agent failed to start: %v\n", err)
			os.Exit(1)
		}

		// Wait for either an error or a shutdown signal
		select {
		case err := <-errChan:
			fmt.Printf("Server error: %v\n", err)
			os.Exit(1)
		case sig := <-sigChan:
			fmt.Printf("\nReceived signal %v, shutting down gracefully...\n", sig)
			agent.Stop()
			fmt.Println("Agent stopped")
		}
	},
}

func GetAgentCommand() *cobra.Command {
	return agentCmd
}

func init() {
	rootCmd.AddCommand(agentCmd) // Run agent directly
}
