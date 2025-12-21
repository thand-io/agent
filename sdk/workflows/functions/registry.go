package functions

import (
	internal "github.com/thand-io/agent/internal/workflows/functions"
)

// FunctionRegistry registers all available workflow functions.
// See internal/workflows/functions/RegisterFunctions
// for full documentation.
type FunctionRegistry = internal.FunctionRegistry

type FunctionHandler = internal.FunctionHandler
