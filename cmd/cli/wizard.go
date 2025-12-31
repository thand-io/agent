package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/charmbracelet/huh"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
)

// RunRequestWizard runs the interactive wizard and returns the collected data
func RunRequestWizard(config *config.Config) (*models.ElevateRequest, error) {
	fmt.Println(titleStyle.Render("Thand Agent - Access Request Wizard"))
	fmt.Println("Configure your elevation request interactively")
	fmt.Println()

	// TODO: If the wizard is using thand.io then default to LLM request
	// Otherwise use the request builder

	data := &models.ElevateRequest{}

	// Step 1: Select Provider
	provider, err := selectProvider(config)
	if err != nil {
		return nil, err
	}
	data.Providers = []string{provider}

	// Step 2: Select Role (filtered by provider)
	role, err := selectRole(config, provider)
	if err != nil {
		return nil, err
	}

	foundRole, err := cfg.GetRoleByName(role)
	if err != nil {
		return nil, err
	}

	data.Role = foundRole

	// Step 3: Select Duration
	duration, err := selectDuration()
	if err != nil {
		return nil, err
	}
	data.Duration = duration

	// Step 4: Enter Reason
	reason, err := selectReason()
	if err != nil {
		return nil, err
	}
	data.Reason = reason

	// Step 5: Optional - Select Identities
	identities, err := selectIdentities(provider)
	if err != nil {
		return nil, err
	}
	data.Identities = identities

	// Step 6: Optional - Select Tenants
	tenants, err := selectTenants(provider)
	if err != nil {
		return nil, err
	}
	data.Tenants = tenants

	// Display summary
	displaySummary(data)

	return data, nil
}

// selectProvider prompts user to select a provider
func selectProvider(config *config.Config) (string, error) {
	var selectedProvider string

	// Get provider options from config
	options := getProviderOptions(config)
	if len(options) == 0 {
		return "", fmt.Errorf("no providers available in configuration")
	}

	// If only one option, select it automatically
	if len(options) == 1 {
		selectedProvider = options[0].Value
		fmt.Printf("Auto-selected provider: %s\n", options[0].Key)
		return selectedProvider, nil
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select Provider:").
				Description("Choose the provider/platform you need access to").
				Options(options...).
				Value(&selectedProvider),
		),
	)

	err := form.Run()
	if err != nil {
		return "", fmt.Errorf("provider selection cancelled: %w", err)
	}

	return selectedProvider, nil
}

// selectRole prompts user to select a role based on the selected provider
func selectRole(config *config.Config, provider string) (string, error) {
	var selectedRole string

	// Get role options filtered by provider
	options := getRoleOptionsForProvider(config, provider)
	if len(options) == 0 {
		return "", fmt.Errorf("no roles available for provider: %s", provider)
	}

	// If only one option, select it automatically
	if len(options) == 1 {
		selectedRole = options[0].Value
		fmt.Printf("Auto-selected role: %s\n", options[0].Key)
		return selectedRole, nil
	}

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title(fmt.Sprintf("Select Role for %s:", provider)).
				Description("Choose the role/permission level you need").
				Options(options...).
				Value(&selectedRole),
		),
	)

	err := form.Run()
	if err != nil {
		return "", fmt.Errorf("role selection cancelled: %w", err)
	}

	return selectedRole, nil
}

// getProviderOptions returns provider options from configuration
func getProviderOptions(config *config.Config) []huh.Option[string] {
	var options []huh.Option[string]

	// Get providers from config
	for providerKey, provider := range config.Providers.Definitions {

		if len(provider.Name) == 0 { // Only providers with names
			logrus.Debugf("Skipping provider %s as it has no name", providerKey)
			continue
		}

		// Check if this provider has any roles associated with it
		hasRoles := false
		for _, role := range config.Roles.Definitions {
			if !role.Enabled {
				logrus.Debugf("Skipping disabled role %s for provider %s", role.Name, providerKey)
				continue
			}
			if slices.Contains(role.Providers, providerKey) {
				hasRoles = true
			}
			if hasRoles {
				break
			}
		}

		// Skip providers with no associated roles
		if !hasRoles {
			logrus.Debugf("Skipping provider %s as it has no associated roles", providerKey)
			continue
		}

		description := provider.Description
		if len(description) == 0 {
			description = fmt.Sprintf("%s provider", provider.Provider)
		}

		option := huh.NewOption(
			fmt.Sprintf("%s - %s", provider.Name, description),
			providerKey,
		)
		options = append(options, option)
	}

	return options
}

// getRoleOptionsForProvider returns role options filtered by provider
func getRoleOptionsForProvider(config *config.Config, providerKey string) []huh.Option[string] {
	var options []huh.Option[string]

	// Get roles that include the selected provider
	for roleKey, role := range config.Roles.Definitions {
		if !role.Enabled {
			continue
		}

		// Check if this role supports the selected provider
		if slices.Contains(role.Providers, providerKey) {
			// Build display name with inheritance info
			displayName := buildRoleDisplayName(role, config.Roles.Definitions)

			option := huh.NewOption(displayName, roleKey)
			options = append(options, option)
		}
	}

	return options
}

// buildRoleDisplayName creates a display name for a role including inheritance information
func buildRoleDisplayName(role models.Role, allRoles map[string]models.Role) string {
	name := role.Name
	if len(name) == 0 {
		name = "Unnamed Role"
	}

	description := role.Description
	if len(description) > 0 {
		name += fmt.Sprintf(" - %s", description)
	}

	// Add inheritance information if role inherits from others
	if len(role.Inherits) > 0 {
		inheritanceInfo := make([]string, 0, len(role.Inherits))
		for _, inheritedRoleKey := range role.Inherits {
			if inheritedRole, exists := allRoles[inheritedRoleKey]; exists {
				inheritedName := inheritedRole.Name
				if len(inheritedName) == 0 {
					inheritedName = inheritedRoleKey
				}
				inheritanceInfo = append(inheritanceInfo, inheritedName)
			} else {
				inheritanceInfo = append(inheritanceInfo, inheritedRoleKey)
			}
		}
		name += fmt.Sprintf(" (inherits: %s)", strings.Join(inheritanceInfo, ", "))
	}

	return name
}

// selectDuration prompts for duration selection
func selectDuration() (string, error) {
	durationOptions := []huh.Option[string]{
		huh.NewOption("15 minutes", "PT15M"),
		huh.NewOption("30 minutes", "PT30M"),
		huh.NewOption("1 hour", "PT1H"),
		huh.NewOption("2 hours", "PT2H"),
		huh.NewOption("4 hours", "PT4H"),
		huh.NewOption("8 hours", "PT8H"),
		huh.NewOption("1 day", "P1D"),
		huh.NewOption("Custom", "custom"),
	}

	var selectedDuration string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Select access duration:").
				Description("How long do you need access? (15m minimum, 8h maximum)").
				Options(durationOptions...).
				Value(&selectedDuration),
		),
	)

	err := form.Run()
	if err != nil {
		return "", fmt.Errorf("duration selection cancelled: %w", err)
	}

	// Handle custom duration
	if selectedDuration == "custom" {
		var customDuration string
		customForm := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Enter custom duration:").
					Description("Format: pt30m, pt1h30m, t2h (15m minimum, 8h maximum)").
					Value(&customDuration).
					Validate(validateDuration),
			),
		)

		err = customForm.Run()
		if err != nil {
			return "", fmt.Errorf("custom duration input cancelled: %w", err)
		}
		selectedDuration = customDuration
	}

	return selectedDuration, nil
}

// selectReason prompts for reason input
func selectReason() (string, error) {
	var reason string

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewText().
				Title("Enter detailed reason for access:").
				Description("Provide specific justification (minimum 10 characters)").
				Value(&reason).
				Validate(validateReason),
		),
	)

	err := form.Run()
	if err != nil {
		return "", fmt.Errorf("reason input cancelled: %w", err)
	}

	return reason, nil
}

// validateDuration validates duration format and bounds
func validateDuration(val string) error {
	_, err := common.ValidateDuration(val)
	return err
}

// validateReason validates reason input
func validateReason(val string) error {
	reason := strings.TrimSpace(val)
	if len(reason) == 0 {
		return fmt.Errorf("reason cannot be empty")
	}

	if len(reason) < 10 {
		return fmt.Errorf("reason must be at least 10 characters long")
	}

	// Basic validation for reasonable content
	resourcePattern := regexp.MustCompile(`^[a-zA-Z0-9:._/\-\s]+$`)
	if !resourcePattern.MatchString(reason) {
		return fmt.Errorf("reason contains invalid characters")
	}

	return nil
}

// selectIdentities prompts for optional identity input
func selectIdentities(provider string) ([]string, error) {
	var wantIdentities bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Specify identities?").
				Description("Do you want to specify specific identities for this request? (Optional)").
				Value(&wantIdentities).
				Affirmative("Yes").
				Negative("Skip"),
		),
	)

	err := form.Run()
	if err != nil {
		return nil, fmt.Errorf("identity selection cancelled: %w", err)
	}

	if !wantIdentities {
		return []string{}, nil
	}

	// Get identities from providers
	identityOptions, err := getIdentityOptions()
	if err != nil || len(identityOptions) == 0 {
		fmt.Println(err)
		// Fallback to manual input if no identities available
		return selectIdentitiesManual()
	}

	var selectedIdentities []string

	// Show searchable list of identities
	selectForm := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select identities:").
				Description("Search and select identities (use arrow keys, space to select, enter to confirm)").
				Options(identityOptions...).
				Filterable(true).
				Limit(10).
				Value(&selectedIdentities),
		),
	)

	err = selectForm.Run()
	if err != nil {
		return nil, fmt.Errorf("identity selection cancelled: %w", err)
	}

	return selectedIdentities, nil
}

// selectIdentitiesManual allows manual entry of identities
func selectIdentitiesManual() ([]string, error) {
	var identitiesInput string

	inputForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter identities (comma-separated):").
				Description("e.g., user@example.com,admin@example.com or provider:identity").
				Value(&identitiesInput).
				Validate(func(val string) error {
					if strings.TrimSpace(val) == "" {
						return fmt.Errorf("identities cannot be empty when specified")
					}
					return nil
				}),
		),
	)

	err := inputForm.Run()
	if err != nil {
		return nil, fmt.Errorf("identity input cancelled: %w", err)
	}

	// Split by comma and trim spaces
	identities := []string{}
	for id := range strings.SplitSeq(identitiesInput, ",") {
		trimmed := strings.TrimSpace(id)
		if trimmed != "" {
			identities = append(identities, trimmed)
		}
	}

	return identities, nil
}

// getIdentityOptions returns identity options from a specific provider
func getIdentityOptions() ([]huh.Option[string], error) {
	var options []huh.Option[string]

	loginSessions, err := sessionManager.GetLoginServer(cfg.GetLoginServerHostname())

	if err != nil {
		return nil, fmt.Errorf("failed to get login sessions: %w", err)
	}

	_, session, err := loginSessions.GetFirstActiveSession()

	if err != nil {
		return nil, fmt.Errorf("failed to get active session: %w", err)
	}

	bodyBytes, err := json.Marshal(models.SearchRequest{
		Limit: 100, // Limit to prevent overwhelming the UI
	})

	fmt.Println(session.Session)

	params := url.Values{
		"q": {""},
	}

	identityUrl := fmt.Sprintf("%s/identities?%s",
		cfg.DiscoverLoginServerApiUrl(
			cfg.GetLoginServerUrl(),
		),
		params.Encode(),
	)

	resp, err := common.InvokeHttpRequest(
		&model.HTTPArguments{
			Method: http.MethodGet,
			Endpoint: &model.Endpoint{
				EndpointConfig: &model.EndpointConfiguration{
					URI: &model.LiteralUri{
						Value: identityUrl,
					},
					Authentication: &model.ReferenceableAuthenticationPolicy{
						AuthenticationPolicy: &model.AuthenticationPolicy{
							Bearer: &model.BearerAuthenticationPolicy{
								Token: session.GetEncodedLocalSession(),
							},
						},
					},
				},
			},
			Body:    bodyBytes,
			Headers: map[string]string{"Content-Type": "application/json"},
		},
	)

	if err != nil {
		return nil, fmt.Errorf("failed to list identities: %w", err)
	}

	responseBody := resp.Body()

	fmt.Println(string(responseBody))

	identities := models.IdentitiesResponse{}
	err = json.Unmarshal(responseBody, &identities)

	if err != nil {
		return nil, fmt.Errorf("failed to parse identities response: %w", err)
	}

	// Add identities as options
	for _, identity := range identities.Identities {
		displayName := identity.Result.ID
		if len(identity.Result.Label) > 0 {
			displayName = fmt.Sprintf("%s (%s)", identity.Result.Label, identity.Result.ID)
		}

		option := huh.NewOption(displayName, identity.Result.ID)
		options = append(options, option)
	}

	return options, nil
}

// selectTenants prompts for optional tenant input
func selectTenants(provider string) ([]string, error) {
	var wantTenants bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Specify tenants?").
				Description("Do you want to specify specific tenants/accounts for this request? (Optional)").
				Value(&wantTenants).
				Affirmative("Yes").
				Negative("Skip"),
		),
	)

	err := form.Run()
	if err != nil {
		return nil, fmt.Errorf("tenant selection cancelled: %w", err)
	}

	if !wantTenants {
		return []string{}, nil
	}

	// Get tenants from providers
	tenantOptions, err := getTenantOptions(provider)

	if err != nil || len(tenantOptions) == 0 {
		// Fallback to manual input if no tenants available
		return selectTenantsManual()
	}

	var selectedTenants []string

	// Show searchable list of tenants
	selectForm := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Select tenants/accounts:").
				Description("Search and select tenants (use arrow keys, space to select, enter to confirm)").
				Options(tenantOptions...).
				Filterable(true).
				Limit(10).
				Value(&selectedTenants),
		),
	)

	err = selectForm.Run()
	if err != nil {
		return nil, fmt.Errorf("tenant selection cancelled: %w", err)
	}

	return selectedTenants, nil
}

// selectTenantsManual allows manual entry of tenants
func selectTenantsManual() ([]string, error) {
	var tenantsInput string

	inputForm := huh.NewForm(
		huh.NewGroup(
			huh.NewInput().
				Title("Enter tenants (comma-separated):").
				Description("e.g., tenant-id-1,account-123,org-456").
				Value(&tenantsInput).
				Validate(func(val string) error {
					if strings.TrimSpace(val) == "" {
						return fmt.Errorf("tenants cannot be empty when specified")
					}
					return nil
				}),
		),
	)

	err := inputForm.Run()
	if err != nil {
		return nil, fmt.Errorf("tenant input cancelled: %w", err)
	}

	// Split by comma and trim spaces
	tenants := []string{}
	for t := range strings.SplitSeq(tenantsInput, ",") {
		trimmed := strings.TrimSpace(t)
		if trimmed != "" {
			tenants = append(tenants, trimmed)
		}
	}

	return tenants, nil
}

// getTenantOptions returns tenant options from a specific provider
func getTenantOptions(providerKey string) ([]huh.Option[string], error) {
	var options []huh.Option[string]
	ctx := context.Background()

	// Get the specific provider
	providerDef, err := cfg.GetProviderByName(providerKey)
	if err != nil {
		return nil, fmt.Errorf("provider %s not found: %w", providerKey, err)
	}

	provider := providerDef.GetClient()
	if provider == nil {
		return nil, fmt.Errorf("provider %s client not initialized", providerKey)
	}

	// Check if provider supports tenants
	if !provider.HasCapability(models.ProviderCapabilityTenants) {
		return nil, fmt.Errorf("provider %s does not support tenants", providerKey)
	}

	// Search for tenants (empty query returns all, with limit)
	searchReq := &models.SearchRequest{
		Limit: 100, // Limit to prevent overwhelming the UI
	}

	loginSessions, err := sessionManager.GetLoginServer(cfg.GetLoginServerHostname())

	if err != nil {
		return nil, fmt.Errorf("failed to get login sessions: %w", err)
	}

	_, session, err := loginSessions.GetFirstActiveSession()

	if err != nil {
		return nil, fmt.Errorf("failed to get active session: %w", err)
	}

	// Set session key in context for provider proxy
	ctx = context.WithValue(ctx, providers.ProviderProxySessionKey, session)

	tenants, err := provider.ListTenants(ctx, searchReq)
	if err != nil {
		return nil, fmt.Errorf("failed to list tenants: %w", err)
	}

	// Add tenants as options
	for _, tenant := range tenants {
		displayName := tenant.Result.Name
		if len(tenant.Result.ID) > 0 && tenant.Result.ID != tenant.Result.Name {
			displayName = fmt.Sprintf("%s (%s)", tenant.Result.Name, tenant.Result.ID)
		}
		if len(tenant.Result.Type) > 0 {
			displayName = fmt.Sprintf("%s [%s]", displayName, tenant.Result.Type)
		}

		// Use the tenant ID as the value
		value := tenant.Result.ID

		option := huh.NewOption(displayName, value)
		options = append(options, option)
	}

	return options, nil
}

// displaySummary shows a summary of the configured request
func displaySummary(data *models.ElevateRequest) {
	fmt.Println()
	fmt.Println(successStyle.Render("Request Configuration Complete!"))
	fmt.Println()

	fmt.Printf("Providers: %s\n", data.Providers)
	fmt.Printf("Role: %s\n", data.Role.Name)
	fmt.Printf("Duration: %s\n", data.Duration)
	fmt.Printf("Reason: %s\n", data.Reason)

	if len(data.Identities) > 0 {
		fmt.Printf("Identities: %s\n", strings.Join(data.Identities, ", "))
	}

	if len(data.Tenants) > 0 {
		fmt.Printf("Tenants: %s\n", strings.Join(data.Tenants, ", "))
	}

	fmt.Println()
}

var wizardCmd = &cobra.Command{
	Use:     "wizard",
	Short:   "Interactive wizard to configure access requests",
	Long:    `Launch an interactive wizard that guides you through creating an access request with proper validation and configuration from your workflows, roles, and providers.`,
	Hidden:  true,
	PreRunE: preRunClientConfigWithSessionE,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("Starting Interactive Access Request Wizard...")
		fmt.Println()

		data, err := RunRequestWizard(cfg)
		if err != nil {
			fmt.Printf("Wizard failed: %v\n", err)
			os.Exit(1)
		}

		// Right kick off our workflow
		err = MakeElevationRequest(data)
		if err != nil {
			fmt.Printf("Elevation request failed: %v\n", err)
			os.Exit(1)
		}

	},
}

func init() {

	rootCmd.AddCommand(wizardCmd)
}
