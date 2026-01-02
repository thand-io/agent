package salesforce

import (
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/functions"
)

type salesforceCollection struct {
	config models.ConfigImpl
	functions.FunctionCollection
}

func NewSalesforceCollection(config models.ConfigImpl) *salesforceCollection {
	return &salesforceCollection{
		config: config,
	}
}

func (c *salesforceCollection) RegisterFunctions(r *functions.FunctionRegistry) {
	// logger := logrus.New()

	// Register functions
	r.RegisterFunctions()

}
