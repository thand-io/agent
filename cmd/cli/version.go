package cli

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/updater"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run: func(cmd *cobra.Command, args []string) {
		version, gitCommit, ok := common.GetModuleBuildInfo()

		if !ok {
			fmt.Println("Failed to get version information")
			return
		}

		fmt.Printf("Thand Agent %s", version)
		if gitCommit != "unknown" && gitCommit != "" {
			if len(gitCommit) > 8 {
				fmt.Printf(" (git: %s)", gitCommit[:8])
			} else {
				fmt.Printf(" (git: %s)", gitCommit)
			}
		}
		fmt.Println()
		fmt.Println("Built with love by the Thand team")

		// Create updater instance
		u := updater.NewUpdater("thand-io", "agent", version)

		// Create context with short timeout for version command
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Check for updates
		release, err := u.CheckForUpdate(ctx)
		if err != nil {
			fmt.Println("(failed to check)")
			return
		}

		if release == nil {
			fmt.Println("✅ You're running the latest version!")
		} else {
			fmt.Printf("🆕 New version available: %s\n", release.GetTagName())
			fmt.Println("   Run 'agent update' to upgrade")
		}
	},
}

func init() {

	rootCmd.AddCommand(versionCmd)
}
