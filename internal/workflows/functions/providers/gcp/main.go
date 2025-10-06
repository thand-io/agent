package gcp

import (
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/workflows/functions"
)

type gcpCollection struct {
	config *config.Config
	functions.FunctionCollection
}

func NewGCPCollection(config *config.Config) *gcpCollection {
	return &gcpCollection{
		config: config,
	}
}

func (c *gcpCollection) RegisterFunctions(r *functions.FunctionRegistry) {
	//logger := logrus.New()

	// Register functions
	r.RegisterFunctions()
}
