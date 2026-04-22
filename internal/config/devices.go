package config

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/thand-io/agent/internal/models"
)

var ErrDeviceRouteUnavailable = errors.New("device route unavailable")

const (
	deviceRouteRefreshInterval = models.DeviceRouteRefreshInterval
	deviceRouteFreshnessTTL    = models.DeviceRouteFreshnessTTL
)

func (c *Config) GetDevice(deviceID string) (*models.Device, error) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil, fmt.Errorf("device id is required")
	}

	for _, device := range c.Devices.Definitions {
		configuredDeviceID := strings.TrimSpace(device.ID)
		if configuredDeviceID == "" {
			continue
		}
		if strings.EqualFold(configuredDeviceID, deviceID) {
			deviceCopy := device
			return &deviceCopy, nil
		}
	}

	return nil, fmt.Errorf("device %q is not configured", deviceID)
}

func (c *Config) SetDeviceConnectionState(state models.DeviceConnectionState) {
	if strings.TrimSpace(state.DeviceID) == "" {
		return
	}

	state.DeviceID = strings.TrimSpace(state.DeviceID)
	state.TaskQueue = strings.TrimSpace(state.TaskQueue)
	if state.LastSeenAt.IsZero() {
		state.LastSeenAt = time.Now().UTC()
	}
	state.Connected = c.isFreshDeviceConnectionState(&state)

	c.deviceConnectionsMu.Lock()
	defer c.deviceConnectionsMu.Unlock()

	if c.deviceConnections == nil {
		c.deviceConnections = make(map[string]*models.DeviceConnectionState)
	}

	stateCopy := state
	c.deviceConnections[state.DeviceID] = &stateCopy
}

func (c *Config) GetDeviceConnectionState(deviceID string) *models.DeviceConnectionState {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" {
		return nil
	}

	c.deviceConnectionsMu.RLock()
	defer c.deviceConnectionsMu.RUnlock()

	state, ok := c.deviceConnections[deviceID]
	if !ok || state == nil {
		return nil
	}

	stateCopy := *state
	stateCopy.Connected = c.isFreshDeviceConnectionState(&stateCopy)
	return &stateCopy
}

func (c *Config) GetFreshDeviceRoute(deviceID string) (*models.DeviceConnectionState, error) {
	device, err := c.GetDevice(deviceID)
	if err != nil {
		return nil, err
	}
	if !device.Enabled {
		return nil, fmt.Errorf("%w: device %q is disabled", ErrDeviceRouteUnavailable, device.ID)
	}
	connectionState := c.GetDeviceConnectionState(device.ID)
	if connectionState == nil || !connectionState.Connected {
		return nil, fmt.Errorf("%w: device %q is not connected", ErrDeviceRouteUnavailable, device.ID)
	}
	if strings.TrimSpace(connectionState.TaskQueue) == "" {
		return nil, fmt.Errorf("%w: device %q has no live task queue", ErrDeviceRouteUnavailable, device.ID)
	}

	return connectionState, nil
}

func (c *Config) isFreshDeviceConnectionState(state *models.DeviceConnectionState) bool {
	if state == nil {
		return false
	}
	if strings.TrimSpace(state.TaskQueue) == "" {
		return false
	}
	if state.LastSeenAt.IsZero() {
		return false
	}
	return time.Since(state.LastSeenAt) <= deviceRouteFreshnessTTL
}
