package thand

import (
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/workflows/functions"
)

type thandCollection struct {
	config *config.Config
	functions.FunctionCollection
}

func NewThandCollection(config *config.Config) *thandCollection {
	return &thandCollection{
		config: config,
	}
}

func (c *thandCollection) RegisterFunctions(r *functions.FunctionRegistry) {

	// Register functions
	r.RegisterFunctions(
		NewApprovalsFunction(c.config),
		NewAuthorizeFunction(c.config),
		NewMonitorFunction(c.config),
		NewNotifyFunction(c.config),
		NewRevokeFunction(c.config),
		NewValidateFunction(c.config),
	)

}
