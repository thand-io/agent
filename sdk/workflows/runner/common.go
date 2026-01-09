package runner

import (
	"errors"

	swctx "github.com/serverlessworkflow/sdk-go/v3/impl/ctx"
)

var ErrorAwaitSignal = errors.New(string(swctx.PendingStatus))
