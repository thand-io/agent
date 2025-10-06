package cli

import (
	"log"

	"github.com/spf13/cobra"
	"github.com/thand-io/agent/internal/agent"
	"github.com/thand-io/agent/internal/config"
)

var (
	configFile string
)

var rootCmd = &cobra.Command{
	Use:   "agent",
	Short: "Start the agent web service",
	Long: `Start the agent web service.

If no config file is specified, the agent will look for config files in the following locations:
  - ./config.yaml
  - ./config/config.yaml
  - /etc/thand/config.yaml
  - ~/.config/thand/config.yaml`,
	Run: func(cmd *cobra.Command, args []string) {
		// Load configuration
		cfg, err := config.Load(configFile)
		if err != nil {
			log.Fatalf("Failed to load configuration: %v", err)
		}

		if _, err := agent.StartWebService(cfg); err != nil {
			log.Fatalf("Failed to start web service: %v", err)
		}
	},
}

func init() {
	rootCmd.PersistentFlags().StringVarP(&configFile, "config", "c", "", "Path to the configuration file (optional)")
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		log.Fatalf("Failed to execute command: %v", err)
	}
}
