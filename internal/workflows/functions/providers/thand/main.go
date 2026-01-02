package thand

import (
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/functions"
)

type thandCollection struct {
	config models.ConfigImpl
	functions.FunctionCollection
}

func NewThandCollection(config models.ConfigImpl) *thandCollection {
	return &thandCollection{
		config: config,
	}
}

func (c *thandCollection) RegisterFunctions(r *functions.FunctionRegistry) {

	// Register functions
	r.RegisterFunctions(
		NewNotifyFunction(c.config),
		NewAuthorizeFunction(c.config),
		NewRevokeFunction(c.config),
	)

}
