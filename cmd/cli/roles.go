package cli

import (
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

var rolesCmd = &cobra.Command{
	Use:     "roles",
	Short:   "List available roles",
	Long:    "List all available roles from the remote login server",
	PreRunE: preAgentE, // load agent
	RunE:    runListRoles,
}

func runListRoles(cmd *cobra.Command, args []string) error {
	// Get the provider filter from the flag
	provider, err := cmd.Flags().GetString("provider")
	if err != nil {
		return fmt.Errorf("failed to get provider flag: %w", err)
	}

	// Display the roles
	displayRoles(provider)

	return nil
}

func displayRoles(provider string) {

	roles := cfg.GetRoles().Definitions

	if len(roles) == 0 {
		if len(provider) > 0 {
			fmt.Printf("No roles found for provider: %s\n", provider)
		} else {
			fmt.Println("No roles found")
		}
		return
	}

	// Header
	if len(provider) > 0 {
		fmt.Printf("Available roles for provider '%s':\n\n", provider)
	} else {
		fmt.Println("Available roles:")
		fmt.Println()
	}

	// Display roles in a table-like format
	fmt.Printf("%-20s %-15s %s\n", "NAME", "PROVIDERS", "DESCRIPTION")
	fmt.Printf("%-20s %-15s %s\n", "----", "---------", "-----------")

	for roleName, role := range roles {

		if len(provider) > 0 && !hasAnyProvider(role.Providers, []string{provider}) {
			continue
		}

		providers := strings.Join(role.Providers, ",")
		if len(providers) > 13 {
			providers = providers[:10] + "..."
		}

		description := role.Description
		if len(description) > 50 {
			description = description[:47] + "..."
		}

		fmt.Printf("%-20s %-15s %s\n", roleName, providers, description)
	}

	fmt.Printf("\nTotal: %d roles\n", len(roles))
}

func hasAnyProvider(roleProviders []string, requestedProviders []string) bool {
	for _, rp := range roleProviders {
		if slices.Contains(requestedProviders, rp) {
			return true
		}
	}
	return false
}

func init() {
	// Add the provider flag
	rolesCmd.Flags().String("provider", "", "Filter roles by provider (e.g., aws, gcp, azure)")

	// Add the command to the root
	rootCmd.AddCommand(rolesCmd)
}
