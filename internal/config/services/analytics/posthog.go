package analytics

import (
	"fmt"

	"github.com/posthog/posthog-go"
	"github.com/sirupsen/logrus"
	common "github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

const (
	// Project API keys (starting "phc_") are safe to be stored publicly
	// @link https://posthog.com/docs/privacy
	//nolint:gosec
	apiKey   = "phc_BBiNnASLM0RDrxRwyx2UAbbmoXqrpeYVQ86fZckkSoU"
	endpoint = "https://us.ph.thand.io"
)

type posthogAnalytics struct {
	config *models.AnalyticsConfig
	client posthog.Client
}

func NewPosthogAnalytics(config *models.AnalyticsConfig) models.Analytics {
	return &posthogAnalytics{
		config: config,
	}
}

func (a *posthogAnalytics) Initialize() error {

	if a.config != nil && a.config.Disabled {
		logrus.Info("Posthog Analytics is disabled")
		return fmt.Errorf("Posthog Analytics is disabled")
	}

	client, err := posthog.NewWithConfig(apiKey, posthog.Config{
		Endpoint: endpoint,
	})

	if err != nil {
		return fmt.Errorf("error creating posthog connection: %w", err)
	}

	a.client = client

	if err := a.Capture("initialized", map[string]any{
		"version": common.GetBuildIdentifier(),
	}); err != nil {
		return fmt.Errorf("error capturing posthog initialization event: %w", err)
	}

	return nil
}

func (a *posthogAnalytics) Shutdown() error {
	if a.client != nil {
		a.client.Close()
	}
	return nil
}

func (a *posthogAnalytics) Capture(event string, metadata map[string]any) error {

	distinctID := common.GetClientIdentifier()
	properties := posthog.NewProperties()

	for key, value := range metadata {
		properties.Set(key, value)
	}

	data := posthog.Capture{
		DistinctId: distinctID.String(),
		Event:      fmt.Sprintf("agent-%s", event),
		Properties: properties,
	}

	if err := a.client.Enqueue(data); err != nil {
		return fmt.Errorf("error sending posthog telemetry: %w", err)
	}

	return nil
}
