package gcp

import (
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/sdk/workflows/functions"
)

type gcpCollection struct {
	config models.ConfigImpl
	functions.FunctionCollection
}

func NewGCPCollection(config models.ConfigImpl) *gcpCollection {
	return &gcpCollection{
		config: config,
	}
}

func (c *gcpCollection) RegisterFunctions(r *functions.FunctionRegistry) {
	//logger := logrus.New()

	// Register functions
	r.RegisterFunctions()
}
