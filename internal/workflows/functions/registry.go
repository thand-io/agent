package functions

import (
	"context"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/models"
)

// FunctionHandler represents a custom function that can be called from workflows
type FunctionHandler func(ctx context.Context, call model.CallFunction, input map[string]any) (map[string]any, error)

// FunctionRegistry manages custom functions for workflow execution
type FunctionRegistry struct {
	functions map[string]Function
	config    models.ConfigImpl
}

func (r *FunctionRegistry) GetConfig() models.ConfigImpl {
	return r.config
}

// NewFunctionRegistry creates a new function registry
func NewFunctionRegistry(config models.ConfigImpl) *FunctionRegistry {
	registry := &FunctionRegistry{
		functions: make(map[string]Function),
		config:    config,
	}

	return registry
}

// RegisterFunction registers a custom function
func (r *FunctionRegistry) RegisterFunction(handler Function) {
	r.functions[handler.GetName()] = handler
	logrus.WithField("function", handler.GetName()).Debug("Registered custom function")
}

func (r *FunctionRegistry) RegisterFunctions(handlers ...Function) {
	for _, handler := range handlers {
		r.RegisterFunction(handler)
	}
}

// GetFunction returns a registered function handler
func (r *FunctionRegistry) GetFunction(name string) (Function, bool) {
	handler, exists := r.functions[name]
	return handler, exists
}

// GetRegisteredFunctions returns all registered function names
func (r *FunctionRegistry) GetRegisteredFunctions() []string {
	names := make([]string, 0, len(r.functions))
	for name := range r.functions {
		names = append(names, name)
	}
	return names
}
