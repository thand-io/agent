package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Configure the agent",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Agent Configuration")
		fmt.Println("Current settings:")

		fmt.Println("Server Host:", cfg.Server.Host)
		fmt.Println("Server Port:", cfg.Server.Port)
		fmt.Println("Login Endpoint:", cfg.Login.Endpoint)
		fmt.Println("Logging Level:", cfg.Logging.Level)

	},
}

func init() {
	rootCmd.AddCommand(configCmd)
}
