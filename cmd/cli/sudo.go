package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

var sudoCmd = &cobra.Command{
	Use:     "sudo [command...]",
	Short:   "Request local sudo access or run a privileged command",
	Long:    `Request time-bound local sudo access or run a single privileged command through the local provider.`,
	PreRunE: preRunClientConfigWithSessionE,
	RunE: func(cmd *cobra.Command, args []string) error {
		reason, _ := cmd.Flags().GetString("reason")
		duration, _ := cmd.Flags().GetString("duration")
		device, _ := cmd.Flags().GetString("device")
		if !cmd.Flags().Changed("device") {
			device = common.GetDeviceID().String()
		}
		request, err := buildLocalSudoElevationRequest(args, reason, duration, device)
		if err != nil {
			return err
		}

		return MakeElevationRequest(request)
	},
}

func buildLocalSudoElevationRequest(args []string, reason, duration, device string) (*models.ElevateRequest, error) {
	if len(reason) == 0 {
		return nil, fmt.Errorf("--reason is required")
	}
	if cfg == nil {
		return nil, fmt.Errorf("configuration is not loaded")
	}

	metadata := models.LocalSudoRequestMetadata{
		Mode: models.LocalSudoModeTimed,
	}

	if len(args) > 0 {
		metadata.Mode = models.LocalSudoModeCommand
		metadata.Command = append([]string(nil), args...)
	}

	role, err := cfg.GetRoleByName(models.LocalSudoRoleIdentifier)
	if err != nil {
		return nil, fmt.Errorf("local sudo role %q is not configured: %w", models.LocalSudoRoleIdentifier, err)
	}

	request := &models.ElevateRequest{
		Role:     models.CloneRole(role),
		Device:   device,
		Reason:   reason,
		Duration: duration,
		Metadata: metadata.AsMap(),
	}
	if err := models.NormalizeLocalSudoRequest(request, cfg.GetProviders().Definitions); err != nil {
		return nil, err
	}

	return request, nil
}

func init() {
	requestCmd.AddCommand(sudoCmd)

	sudoCmd.Flags().StringP("duration", "d", "", "Duration of timed sudo access (for example 30m or 1h)")
	sudoCmd.Flags().StringP("reason", "e", "", "Reason for the sudo request")
	sudoCmd.Flags().String("device", "", "Canonical device_id for local sudo execution (defaults to the current device when omitted)")
}
