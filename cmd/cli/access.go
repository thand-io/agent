package cli

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/thand-io/agent/internal/models"
)

/*
This handles access requests to a given agent. It allows users to request
access to specific resources with defined roles and durations.

 1. Bounce user to the SSO login page provided by the login-server.
 2. After login the agent gets back its session JWT
 3. The requested workflow workflow is then executed. The remote agent via the login-server
    will then execute the workflow workflow.
 4. The response / status of the workflow workflow is returned to the user in the CLI.
*/
var accessCmd = &cobra.Command{
	Use:     "access",
	Short:   "Request access to a specific resource",
	Long:    `Request access to a specific resource with role and duration`,
	PreRunE: preAgentE, // load agent
	Run: func(cmd *cobra.Command, args []string) {
		resource, _ := cmd.Flags().GetString("resource")
		role, _ := cmd.Flags().GetString("role")
		duration, _ := cmd.Flags().GetString("duration")
		reason, _ := cmd.Flags().GetString("reason")

		if len(resource) == 0 || len(role) == 0 || len(duration) == 0 || len(reason) == 0 {
			fmt.Println("Error: --resource, --role, --duration, and --reason are required")
			fmt.Println("Example: agent request access --resource snowflake-prod --role analyst --duration 4h --reason 'Need access for analysis'")
			return
		}

		foundRole, err := cfg.GetRoleByName(role)

		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}

		err = MakeElevationRequest(&models.ElevateRequest{
			Role:      foundRole,
			Providers: []string{resource},
			// Let the system pick the workflow based on role and provider
			Reason:   reason,
			Duration: duration,
		})

		if err != nil {
			fmt.Printf("Error: %v\n", err)
			return
		}
	},
}

func init() {

	// Add access subcommand to request
	requestCmd.AddCommand(accessCmd) // Builds the request based on roles and providers

	// Add flags for access command
	accessCmd.Flags().StringP("resource", "r", "", "Resource to access (e.g., snowflake-prod, aws-prod)")
	accessCmd.Flags().StringP("role", "o", "", "Role to assume (e.g., analyst, admin, readonly)")
	accessCmd.Flags().StringP("duration", "d", "", "Duration of access (e.g., 1h, 4h, 8h)")
	accessCmd.Flags().StringP("reason", "e", "", "Reason for access request (e.g., 'Need access for analysis')")

}
