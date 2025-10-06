package slack

import (
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/workflows/functions"
)

type slackCollection struct {
	config *config.Config
	functions.FunctionCollection
}

func NewSlackCollection(config *config.Config) *slackCollection {
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
