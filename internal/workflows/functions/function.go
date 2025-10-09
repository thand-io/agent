package functions

import (
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/internal/models"
)

type FunctionCollection interface {
	RegisterFunctions(registry *FunctionRegistry)
}

// Function defines the interface that all Thand Functions must implement
type Function interface {
	// GetName returns the unique name/identifier for this Function
	GetName() string

	// GetDescription returns a human-readable description of what this Function does
	GetDescription() string

	// GetVersion returns the version of this Function implementation
	GetVersion() string

	// ValidateRequest validates the input parameters for this Function
	// Security: This method should perform thorough input validation
	ValidateRequest(
		workflowTask *models.WorkflowTask,
		call *model.CallFunction,
		input any,
	) error

	// Execute performs the main Function logic
	// Security: All inputs should be pre-validated by ValidateRequest
	Execute(
		workflowTask *models.WorkflowTask,
		call *model.CallFunction,
		input any,
	) (any, error)

	// GetRequiredParameters returns the list of required parameter names
	GetRequiredParameters() []string

	// GetOptionalParameters returns the list of optional parameter names with their default values
	GetOptionalParameters() map[string]any

	// Handler to transform the output before exporting
	GetOutput() *model.Output
	// Handler to export the output into the workflow context
	GetExport() *model.Export
}

// BaseFunction provides common functionality for all Functions
type BaseFunction struct {
	name        string
	description string
	version     string
}

// NewBaseFunction creates a new base Function with common fields
func NewBaseFunction(name, description, version string) *BaseFunction {
	return &BaseFunction{
		name:        name,
		description: description,
		version:     version,
	}
}

// GetName returns the Function name
func (t *BaseFunction) GetName() string {
	return t.name
}

// GetDescription returns the Function description
func (t *BaseFunction) GetDescription() string {
	return t.description
}

// GetVersion returns the Function version
func (t *BaseFunction) GetVersion() string {
	return t.version
}

// GetOutput just returns nil - override in specific functions
func (t *BaseFunction) GetOutput() *model.Output {
	return nil
}

// GetExport just returns nil - override in specific functions
func (t *BaseFunction) GetExport() *model.Export {
	return nil
}
