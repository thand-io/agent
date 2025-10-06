package salesforce

import (
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/workflows/functions"
)

type salesforceCollection struct {
	config *config.Config
	functions.FunctionCollection
}

func NewSalesforceCollection(config *config.Config) *salesforceCollection {
	return &salesforceCollection{
		config: config,
	}
}

func (c *salesforceCollection) RegisterFunctions(r *functions.FunctionRegistry) {
	// logger := logrus.New()

	// Register functions
	r.RegisterFunctions()

}
