package models

import (
	"context"
	"fmt"
)

type NotificationRequest map[string]any

type ProviderNotifier interface {

	// Allow this provider to send notifications.
	//
	// Notifier providers receive a ProviderContext (rather than a plain
	// context.Context) so that, when invoked from within a Temporal workflow,
	// they can dispatch the underlying API call as a Temporal activity for
	// retry/replay determinism. Providers that do not support workflow
	// dispatch should type-assert the context to context.Context and proceed
	// directly. See email/slack providers for the dual-mode pattern.
	SendNotification(ctx ProviderContext, notification NotificationRequest) error
}

/* Default implementations for notifiers */

func (p *BaseProvider) SendNotification(ctx ProviderContext, notification NotificationRequest) error {
	// Default implementation does nothing
	return fmt.Errorf("the provider '%s' does not implement SendNotification", p.GetProvider())
}

// ContextFromProviderContext extracts a context.Context from a ProviderContext.
// When ctx is already a context.Context it is returned unchanged; when ctx is a
// workflow.Context (or any other ProviderContext implementation) the helper
// falls back to context.Background() so callers get a usable context for
// outbound API calls. This mirrors the type-assertion pattern used by the AWS
// provider's exec* helpers.
func ContextFromProviderContext(ctx ProviderContext) context.Context {
	if ctx == nil {
		return context.Background()
	}
	if goCtx, ok := ctx.(context.Context); ok {
		return goCtx
	}
	return context.Background()
}
