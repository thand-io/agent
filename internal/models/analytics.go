package models

type Analytics interface {

	// Initialize sets up the Analytics service and prepares required resources
	Initialize() error

	// Shutdown closes connections and cleans up resources
	Shutdown() error

	// Capture records an event with associated metadata
	Capture(event string, metadata map[string]any) error
}

type AnalyticsConfig struct {
	// Provider - the Analytics service provider (e.g., "posthog")
	Provider string `mapstructure:"provider"`

	// Disable Analytics service
	Disabled bool `mapstructure:"disabled" default:"false"`

	// Config - additional configuration parameters for the Analytics service
	Config *BasicConfig `mapstructure:"config"`
}

func (e *AnalyticsConfig) GetProvider() string {
	if e == nil || len(e.Provider) == 0 {
		return "posthog"
	}
	return e.Provider
}
