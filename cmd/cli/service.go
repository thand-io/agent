package cli

import (
	"fmt"
	"os"

	"github.com/kardianos/service"
	"github.com/spf13/cobra"
	"github.com/thand-io/agent/internal/agent"
)

var serviceCmd = &cobra.Command{
	Use:   "service",
	Short: "Service management commands",
	Long:  `Manage the Thand Agent as a system service`,
}

var installCmd = &cobra.Command{
	Use:   "install",
	Short: "Install the agent as a system service",
	Long:  `Install the Thand Agent as a system service that will start automatically on boot`,
	Run: func(cmd *cobra.Command, args []string) {
		s, err := agent.CreateService(cfg)
		if err != nil {
			fmt.Printf("Failed to create service: %v\n", err)
			os.Exit(1)
		}

		err = s.Install()
		if err != nil {
			fmt.Printf("Failed to install service: %v\n", err)
			printInstallInstructions()
			os.Exit(1)
		}

		fmt.Println("Thand Agent service installed successfully")
		fmt.Println("   Use 'thand agent start' to start the service")
	},
}

var startCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the agent service",
	Long:  `Start the Thand Agent system service`,
	Run: func(cmd *cobra.Command, args []string) {
		s, err := agent.CreateService(cfg)
		if err != nil {
			fmt.Printf("Failed to create service: %v\n", err)
			os.Exit(1)
		}

		err = s.Start()
		if err != nil {
			fmt.Printf("Failed to start service: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Thand Agent service started successfully")
	},
}

var stopCmd = &cobra.Command{
	Use:   "stop",
	Short: "Stop the agent service",
	Long:  `Stop the Thand Agent system service`,
	Run: func(cmd *cobra.Command, args []string) {
		s, err := agent.CreateService(cfg)
		if err != nil {
			fmt.Printf("Failed to create service: %v\n", err)
			os.Exit(1)
		}

		err = s.Stop()
		if err != nil {
			fmt.Printf("Failed to stop service: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Thand Agent service stopped successfully")
	},
}

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Check the agent service status",
	Long:  `Check the status of the Thand Agent system service`,
	Run: func(cmd *cobra.Command, args []string) {
		s, err := agent.CreateService(cfg)
		if err != nil {
			fmt.Printf("Failed to create service: %v\n", err)
			os.Exit(1)
		}

		status, err := s.Status()
		if err != nil {
			fmt.Printf("Failed to get service status: %v\n", err)
			os.Exit(1)
		}

		var statusText string
		switch status {
		case service.StatusRunning:
			statusText = "🟢 Running"
		case service.StatusStopped:
			statusText = "Stopped"
		case service.StatusUnknown:
			statusText = "🟡 Unknown"
		default:
			statusText = "Unknown state"
		}

		fmt.Printf("Thand Agent Service Status: %s\n", statusText)
	},
}

var removeCmd = &cobra.Command{
	Use:   "remove",
	Short: "Uninstall the agent service",
	Long:  `Uninstall the Thand Agent system service`,
	Run: func(cmd *cobra.Command, args []string) {
		s, err := agent.CreateService(cfg)
		if err != nil {
			fmt.Printf("Failed to create service: %v\n", err)
			os.Exit(1)
		}

		// Stop the service first if it's running
		err = s.Stop()
		if err != nil {
			// Don't fail if service is already stopped
			fmt.Println("Service was not running")
		}

		err = s.Uninstall()
		if err != nil {
			fmt.Printf("Failed to uninstall service: %v\n", err)
			os.Exit(1)
		}

		fmt.Println("Thand Agent service uninstalled successfully")
	},
}

func printInstallInstructions() {
	exePath, _ := os.Executable()
	fmt.Println("\nService installation failed. You may need to run with elevated privileges:")
	fmt.Println("\nLinux:")
	fmt.Printf("   sudo %s agent install\n", exePath)
	fmt.Println("\n🪟 Windows:")
	fmt.Printf("   Run as Administrator: %s agent install\n", exePath)
	fmt.Println("\nmacOS:")
	fmt.Printf("   sudo %s agent install\n", exePath)
}

func init() {

	rootCmd.AddCommand(serviceCmd) // Service management commands
	// Add subcommands to agent
	serviceCmd.AddCommand(installCmd)
	serviceCmd.AddCommand(startCmd)
	serviceCmd.AddCommand(stopCmd)
	serviceCmd.AddCommand(statusCmd)
	serviceCmd.AddCommand(removeCmd)
}
