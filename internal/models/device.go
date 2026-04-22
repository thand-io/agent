package models

import (
	"time"
)

const (
	TemporalResolveFreshDeviceRouteActivityName = "resolve-fresh-device-route"
	TemporalBuildExecutionPlanActivityName      = "build-execution-plan"

	TemporalDeviceRegistryTaskQueue = "thand_device_registry"

	TemporalDeviceRouteRegistryWorkflowName = "DeviceRouteRegistryWorkflow"
	TemporalDeviceRouteRegistryWorkflowID   = "thand-device-route-registry"
	TemporalDeviceRouteUpsertSignalName     = "upsert-device-route"
	TemporalGetDeviceRouteQueryName         = "get-device-route"

	TemporalDeviceDefinitionRegistryWorkflowName = "DeviceDefinitionRegistryWorkflow"
	TemporalDeviceDefinitionRegistryWorkflowID   = "thand-device-definition-registry"
	TemporalDeviceDefinitionUpsertSignalName     = "upsert-device-definition"
	TemporalGetDeviceDefinitionQueryName         = "get-device-definition"
)

const (
	DeviceRouteRefreshInterval = 30 * time.Second
	DeviceRouteFreshnessTTL    = 2 * time.Minute
)

type Device struct {
	// Device is a first-class execution target managed by the server.
	// It is intentionally separate from provider tenants because device routing,
	// local policy, and local lease enforcement have different lifecycle needs.
	ID             string                      `json:"device_id,omitempty" yaml:"device_id,omitempty" mapstructure:"device_id"`
	Name           string                      `json:"name" yaml:"name" mapstructure:"name"`
	Description    string                      `json:"description,omitempty" yaml:"description,omitempty" mapstructure:"description"`
	Platform       string                      `json:"platform,omitempty" yaml:"platform,omitempty" mapstructure:"platform"`
	Enabled        bool                        `json:"enabled" yaml:"enabled" mapstructure:"enabled"`
	LocalElevation *DeviceLocalElevationPolicy `json:"local_elevation,omitempty" yaml:"local_elevation,omitempty" mapstructure:"local_elevation"`
}

type DeviceConnectionState struct {
	DeviceID   string    `json:"device_id,omitempty" yaml:"device_id,omitempty" mapstructure:"device_id"`
	TaskQueue  string    `json:"task_queue,omitempty" yaml:"task_queue,omitempty" mapstructure:"task_queue"`
	Name       string    `json:"name,omitempty" yaml:"name,omitempty" mapstructure:"name"`
	Hostname   string    `json:"hostname,omitempty" yaml:"hostname,omitempty" mapstructure:"hostname"`
	Platform   string    `json:"platform,omitempty" yaml:"platform,omitempty" mapstructure:"platform"`
	LastSeenAt time.Time `json:"last_seen_at,omitempty" yaml:"last_seen_at,omitempty" mapstructure:"last_seen_at"`
	Connected  bool      `json:"connected,omitempty" yaml:"connected,omitempty" mapstructure:"connected"`
}
