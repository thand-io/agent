package cli

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/updater"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Update the agent to the latest version",
	Long: `Check for and install the latest version of the Thand Agent.
This command will check the GitHub repository for the latest release
and automatically update the binary if a newer version is available.`,
	Run: func(cmd *cobra.Command, args []string) {
		// Get current version
		version, gitCommit, ok := common.GetModuleBuildInfo()
		if !ok {
			fmt.Println("Unable to determine current version")
			os.Exit(1)
		}

		fmt.Printf("Current version: %s", version)
		if len(gitCommit) > 0 {
			fmt.Printf(" (commit: %s)", gitCommit[:8])
		}
		fmt.Println()

		// Check if we should force update
		force, _ := cmd.Flags().GetBool("force")
		checkOnly, _ := cmd.Flags().GetBool("check")

		// Create updater instance
		u := updater.NewUpdater("thand-io", "agent", version)

		// Create context with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()

		fmt.Println("Checking for updates...")

		// Check for updates
		release, err := u.CheckForUpdate(ctx)
		if err != nil {
			fmt.Printf("Failed to check for updates: %v\n", err)
			os.Exit(1)
		}

		if release == nil {
			fmt.Println("You are already running the latest version!")
			return
		}

		fmt.Printf("🆕 New version available: %s\n", release.GetTagName())
		if len(release.GetBody()) > 0 {
			fmt.Printf("Release notes:\n%s\n\n", release.GetBody())
		}

		// If check-only flag is set, just show info and exit
		if checkOnly {
			fmt.Println("ℹ️  Use 'agent update' to install the update")
			return
		}

		// Ask for confirmation unless force flag is set
		if !force {
			fmt.Print("Do you want to update now? (y/N): ")
			var response string
			fmt.Scanln(&response)
			if response != "y" && response != "Y" && response != "yes" {
				fmt.Println("Update cancelled")
				return
			}
		}

		fmt.Printf("⬇️  Downloading and installing version %s...\n", release.GetTagName())

		// Perform the update
		err = u.Update(ctx, release)
		if err != nil {
			fmt.Printf("Update failed: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("Successfully updated to version %s!\n", release.GetTagName())
		fmt.Println("Please restart the agent to use the new version")
	},
}

func init() {
	// Add flags
	updateCmd.Flags().BoolP("force", "f", false, "Force update without confirmation")
	updateCmd.Flags().BoolP("check", "c", false, "Only check for updates, don't install")

	// Add command to root
	rootCmd.AddCommand(updateCmd)
}
