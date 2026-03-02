package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/go-playground/validator/v10"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
	providerSdk "github.com/thand-io/agent/sdk/providers"
)

var (
	// Global validator instance
	validate *validator.Validate
)

type validationResult struct {
	FilePath string
	FileType string
	Success  bool
	Error    error
}

type validationSummary struct {
	TotalFiles   int
	SuccessCount int
	FailCount    int
	Roles        []validationResult
	Workflows    []validationResult
	Providers    []validationResult
}

var validateCmd = &cobra.Command{
	Use:   "validate [directory]",
	Short: "Validate roles, providers, and workflows from YAML files",
	Long: `Validate configuration files in a directory. 
	
Automatically detects whether files contain roles, workflows, or providers and validates them.
Use --dry-run to only validate syntax without initializing providers.
By default (without --dry-run), providers will be initialized to test full functionality.`,
	Args:         cobra.ExactArgs(1),
	RunE:         runValidate,
	SilenceUsage: true, // Don't print usage when validation fails
}

var dryRun bool

func runValidate(cmd *cobra.Command, args []string) error {
	// Use the common singleton validator instance
	validate = common.GetValidator()

	dirPath := args[0]

	// Verify directory exists
	if _, err := os.Stat(dirPath); os.IsNotExist(err) {
		return fmt.Errorf("directory does not exist: %s", dirPath)
	}

	logrus.Infof("Validating configuration files in: %s", dirPath)
	if dryRun {
		logrus.Info("Running in dry-run mode (syntax validation only)")
	} else {
		logrus.Info("Running with full validation (will initialize providers)")
	}

	summary := &validationSummary{}

	// Walk through the directory and find all YAML/JSON files
	err := filepath.Walk(dirPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Check if file is YAML or JSON
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" && ext != ".json" {
			return nil // Skip non-YAML/JSON files
		}

		summary.TotalFiles++

		// Read the file
		data, err := os.ReadFile(path)
		if err != nil {
			result := validationResult{
				FilePath: path,
				FileType: "unknown",
				Success:  false,
				Error:    fmt.Errorf("failed to read file: %w", err),
			}
			summary.FailCount++
			logError(result)
			return nil // Continue with other files
		}

		// Detect and validate the file type
		if err := detectAndValidate(path, data, summary); err != nil {
			// Error already logged in detectAndValidate
			return nil // Continue with other files
		}

		return nil
	})

	if err != nil {
		return fmt.Errorf("error walking directory: %w", err)
	}

	// Print summary
	printSummary(summary)

	// Return error if any validation failed
	if summary.FailCount > 0 {
		return fmt.Errorf("validation failed for %d file(s)", summary.FailCount)
	}

	return nil
}

// detectAndValidate determines the type of configuration file and validates it
func detectAndValidate(path string, data []byte, summary *validationSummary) error {
	// Try to detect what type of file this is by attempting to parse it
	// We'll try in order: providers, roles, workflows

	// Try as provider definition
	if result := validateAsProvider(path, data); result.Success || result.Error != nil {
		if result.Success {
			summary.SuccessCount++
			logSuccess(result)
		} else {
			summary.FailCount++
			logError(result)
		}
		summary.Providers = append(summary.Providers, result)
		return nil
	}

	// Try as role definition
	if result := validateAsRole(path, data); result.Success || result.Error != nil {
		if result.Success {
			summary.SuccessCount++
			logSuccess(result)
		} else {
			summary.FailCount++
			logError(result)
		}
		summary.Roles = append(summary.Roles, result)
		return nil
	}

	// Try as workflow definition
	if result := validateAsWorkflow(path, data); result.Success || result.Error != nil {
		if result.Success {
			summary.SuccessCount++
			logSuccess(result)
		} else {
			summary.FailCount++
			logError(result)
		}
		summary.Workflows = append(summary.Workflows, result)
		return nil
	}

	// Could not determine file type
	result := validationResult{
		FilePath: path,
		FileType: "unknown",
		Success:  false,
		Error:    fmt.Errorf("could not determine file type (not a valid provider, role, or workflow definition)"),
	}
	summary.FailCount++
	logError(result)
	return result.Error
}

// validateAsProvider attempts to parse and validate as a provider definition
func validateAsProvider(path string, data []byte) validationResult {
	result := validationResult{
		FilePath: path,
		FileType: "provider",
		Success:  false,
	}

	// Try to parse as provider
	providerDef, err := common.ReadDataToInterface(data, models.ProviderDefinitions{})
	if err != nil {
		// Not a provider or invalid syntax
		return validationResult{Success: false} // Try next type
	}

	// Check if it has the providers field
	if providerDef == nil || len(providerDef.Providers) == 0 {
		return validationResult{Success: false} // Try next type
	}

	// Successfully parsed as provider
	result.Success = true

	// Validate using the struct's built-in validation
	if err := providerDef.Validate(); err != nil {
		result.Success = false
		result.Error = err
		return result
	}

	// Initialize each provider
	for providerKey, provider := range providerDef.Providers {
		if !provider.Enabled {
			logrus.Debugf("  Provider '%s' is disabled, skipping initialization", providerKey)
			continue
		}

		// Validate the provider configuration against its schema (without initialization) --- IGNORE ---

		// Resolve environment variables in config
		err := provider.ResolveConfig(map[string]any{})

		if err != nil {
			result.Success = false
			result.Error = fmt.Errorf("failed to resolve config for provider '%s': %w", providerKey, err)
			return result
		}

		err = providerSdk.ValidateConfig(provider.Provider, provider.Config)

		if err != nil {
			result.Success = false
			result.Error = fmt.Errorf("config validation failed for provider '%s': %w", providerKey, err)
			return result
		}

		// If not dry-run, try to initialize the providers
		if !dryRun {
			if err := initializeProvider(providerKey, &provider); err != nil {
				result.Success = false
				result.Error = fmt.Errorf("provider '%s' failed initialization: %w", providerKey, err)
				return result
			}
			logrus.Debugf("  Provider '%s' initialized successfully", providerKey)
		}
	}

	return result
}

// validateAsRole attempts to parse and validate as a role definition
func validateAsRole(path string, data []byte) validationResult {
	result := validationResult{
		FilePath: path,
		FileType: "role",
		Success:  false,
	}

	// Try to parse as role
	roleDef, err := common.ReadDataToInterface(data, models.RoleDefinitions{})
	if err != nil {
		// Not a role or invalid syntax
		return validationResult{Success: false} // Try next type
	}

	// Check if it has the roles field
	if roleDef == nil || len(roleDef.Roles) == 0 {
		return validationResult{Success: false} // Try next type
	}

	// Successfully parsed as role
	result.Success = true

	// Validate using the struct's built-in validation
	if err := roleDef.Validate(); err != nil {
		result.Success = false
		result.Error = err
		return result
	}

	return result
}

// validateAsWorkflow attempts to parse and validate as a workflow definition
func validateAsWorkflow(path string, data []byte) validationResult {
	result := validationResult{
		FilePath: path,
		FileType: "workflow",
		Success:  false,
	}

	// Try to parse as workflow
	workflowDef, err := common.ReadDataToInterface(data, models.WorkflowDefinitions{})
	if err != nil {
		// Not a workflow or invalid syntax
		return validationResult{Success: false} // Try next type
	}

	// Check if it has the workflows field
	if workflowDef == nil || len(workflowDef.Workflows) == 0 {
		return validationResult{Success: false} // Try next type
	}

	// Successfully parsed as workflow
	result.Success = true

	// Validate using the struct's built-in validation
	if err := workflowDef.Validate(); err != nil {
		result.Success = false
		result.Error = err
		return result
	}

	return result
}

// initializeProvider initializes a single provider for testing
func initializeProvider(providerKey string, provider *models.ProviderConfig) error {

	// Create provider implementation instance
	impl, err := providers.CreateInstance(strings.ToLower(provider.Provider))
	if err != nil {
		return fmt.Errorf("failed to create provider instance: %w", err)
	}

	// Initialize the provider
	if err := impl.Initialize(providerKey, *provider); err != nil {
		return fmt.Errorf("initialization failed: %w", err)
	}

	return nil
}

// Logging helpers
func logSuccess(result validationResult) {
	logrus.Infof("✓ [%s] %s", strings.ToUpper(result.FileType), result.FilePath)
}

func logError(result validationResult) {
	if result.Error != nil {
		logrus.Errorf("✗ [%s] %s: %v", strings.ToUpper(result.FileType), result.FilePath, result.Error)
	} else {
		logrus.Errorf("✗ [%s] %s: validation failed", strings.ToUpper(result.FileType), result.FilePath)
	}
}

// printSummary prints a summary of the validation results
func printSummary(summary *validationSummary) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("VALIDATION SUMMARY")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("Total files processed: %d\n", summary.TotalFiles)
	fmt.Printf("Successful:            %d\n", summary.SuccessCount)
	fmt.Printf("Failed:                %d\n", summary.FailCount)
	fmt.Println()

	if len(summary.Providers) > 0 {
		fmt.Printf("Providers:   %d file(s)\n", len(summary.Providers))
	}
	if len(summary.Roles) > 0 {
		fmt.Printf("Roles:       %d file(s)\n", len(summary.Roles))
	}
	if len(summary.Workflows) > 0 {
		fmt.Printf("Workflows:   %d file(s)\n", len(summary.Workflows))
	}

	fmt.Println(strings.Repeat("=", 60))

	if summary.FailCount == 0 {
		fmt.Println("All validations passed!")
	} else {
		fmt.Printf("%d validation(s) failed\n", summary.FailCount)
	}
}

func init() {
	// Add --dry-run flag (default false)
	validateCmd.Flags().BoolVar(&dryRun, "dry-run", false, "Only validate syntax without initializing providers")

	// Register the command
	rootCmd.AddCommand(validateCmd)
}
