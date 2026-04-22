package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/thand-io/agent/internal/common"
)

var configDeviceIDCmd = &cobra.Command{
	Use:           "device-id",
	Short:         "Print the effective device ID for this machine",
	Args:          cobra.NoArgs,
	SilenceUsage:  true,
	SilenceErrors: true,
	RunE: func(cmd *cobra.Command, _ []string) error {
		_, err := fmt.Fprintln(cmd.OutOrStdout(), common.GetDeviceID().String())
		return err
	},
}

func init() {
	configCmd.AddCommand(configDeviceIDCmd)
}
