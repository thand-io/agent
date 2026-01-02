package aws

import (
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/functions"
)

type AWSCollection struct {
	config models.ConfigImpl
	functions.FunctionCollection
}

func NewAWSCollection(config models.ConfigImpl) *AWSCollection {
	return &AWSCollection{
		config: config,
	}
}

func (c *AWSCollection) RegisterFunctions(r *functions.FunctionRegistry) {
	//logger := logrus.New()

	// Register functions
	r.RegisterFunctions()
}
