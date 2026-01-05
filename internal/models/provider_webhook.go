package models

import (
	"context"
	"fmt"

	"github.com/gin-gonic/gin"
)


type WebhookRequest struct {
	Context  *gin.Context
	Endpoint string
	Session  *Session
}

type ProviderWebhook interface {

	// Allow this provider to send notifications
	HandleWebhook(ctx context.Context, request *WebhookRequest) error
}

/* Default implementations for webhooks */
func (p *BaseProvider) HandleWebhook(ctx context.Context, request *WebhookRequest) error {
	// Default implementation does nothing
	return fmt.Errorf("the provider '%s' does not implement HandleWebhook", p.GetProvider())
}
