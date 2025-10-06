package aws

import (
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/workflows/functions"
)

type AWSCollection struct {
	config *config.Config
	functions.FunctionCollection
}

func NewAWSCollection(config *config.Config) *AWSCollection {
	return &AWSCollection{
		config: config,
	}
}

func (c *AWSCollection) RegisterFunctions(r *functions.FunctionRegistry) {
	//logger := logrus.New()

	// Register functions
	r.RegisterFunctions()
}
