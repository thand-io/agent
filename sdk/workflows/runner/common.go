package runner

import (
	"errors"
	"time"

	swctx "github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
	"go.temporal.io/sdk/temporal"
)

var ErrorAwaitSignal = errors.New(string(swctx.PendingStatus))

var DefaultRetryPolicy = &temporal.RetryPolicy{
	InitialInterval:    time.Second,
	BackoffCoefficient: 2.0,
	MaximumInterval:    time.Minute,
	MaximumAttempts:    5,
}

var CriticalPathRetryPolicy = &temporal.RetryPolicy{
	InitialInterval:    time.Second,
	BackoffCoefficient: 2.0,
	MaximumInterval:    time.Minute,
	MaximumAttempts:    0, // infinite retries for critical path tasks
}
