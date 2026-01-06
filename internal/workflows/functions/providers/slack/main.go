package slack

import (
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/sdk/workflows/functions"
)

type slackCollection struct {
	config models.ConfigImpl
	functions.FunctionCollection
}

func NewSlackCollection(config models.ConfigImpl) *slackCollection {
	return &slackCollection{
		config: config,
	}
}

func (c *slackCollection) RegisterFunctions(r *functions.FunctionRegistry) {
	// logger := logrus.New()

	// Register functions
	r.RegisterFunctions(
		NewSlackPostMessageFunction(c.config),
	)

}
