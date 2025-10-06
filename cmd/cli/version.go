package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Thand Agent v0.1.0")
		fmt.Println("Built with love by the Thand team")
	},
}

func init() {

	rootCmd.AddCommand(versionCmd)
}
