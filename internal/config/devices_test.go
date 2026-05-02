package config

import (
	"strings"
	"testing"
	"time"

	"github.com/thand-io/agent/internal/models"
)

func TestGetFreshDeviceRouteUsesConnectedTaskQueue(t *testing.T) {
	cfg := &Config{
		Devices: DeviceDefinitionsConfig{
			Definitions: map[string]models.Device{
				"device-alpha": {
					ID:      "device-alpha",
					Name:    "Device Alpha",
					Enabled: true,
				},
			},
		},
	}

	cfg.SetDeviceConnectionState(models.DeviceConnectionState{
		DeviceID:  "device-alpha",
		TaskQueue: "connected-queue",
	})

	route, err := cfg.GetFreshDeviceRoute("device-alpha")
	if err != nil {
		t.Fatalf("GetFreshDeviceRoute returned error: %v", err)
	}

	if got, want := route.TaskQueue, "connected-queue"; got != want {
		t.Fatalf("task queue = %q, want %q", got, want)
	}
	if !route.Connected {
		t.Fatal("expected fresh route to be marked connected")
	}
}

func TestGetFreshDeviceRouteRejectsStaleConnection(t *testing.T) {
	cfg := &Config{
		Devices: DeviceDefinitionsConfig{
			Definitions: map[string]models.Device{
				"device-alpha": {
					ID:      "device-alpha",
					Name:    "Device Alpha",
					Enabled: true,
				},
			},
		},
	}

	cfg.SetDeviceConnectionState(models.DeviceConnectionState{
		DeviceID:   "device-alpha",
		TaskQueue:  "connected-queue",
		LastSeenAt: time.Now().UTC().Add(-deviceRouteFreshnessTTL - time.Second),
	})

	_, err := cfg.GetFreshDeviceRoute("device-alpha")
	if err == nil {
		t.Fatal("expected stale route error")
	}
	if !strings.Contains(err.Error(), `device "device-alpha" is not connected`) {
		t.Fatalf("unexpected error: %v", err)
	}

	state := cfg.GetDeviceConnectionState("device-alpha")
	if state == nil {
		t.Fatal("expected stored connection state")
	}
	if state.Connected {
		t.Fatal("expected stale connection state to be marked disconnected")
	}
}

func TestGetDeviceUsesCanonicalDeviceID(t *testing.T) {
	cfg := &Config{
		Devices: DeviceDefinitionsConfig{
			Definitions: map[string]models.Device{
				"workstation-alpha": {
					ID:      "device-alpha",
					Name:    "Device Alpha",
					Enabled: true,
				},
			},
		},
	}

	device, err := cfg.GetDevice("device-alpha")
	if err != nil {
		t.Fatalf("GetDevice returned error: %v", err)
	}

	if got, want := device.ID, "device-alpha"; got != want {
		t.Fatalf("device id = %q, want %q", got, want)
	}
}

func TestGetDeviceDoesNotTreatMapKeyAsIdentity(t *testing.T) {
	cfg := &Config{
		Devices: DeviceDefinitionsConfig{
			Definitions: map[string]models.Device{
				"workstation-alpha": {
					ID:      "device-alpha",
					Name:    "Device Alpha",
					Enabled: true,
				},
			},
		},
	}

	if _, err := cfg.GetDevice("workstation-alpha"); err == nil {
		t.Fatal("expected GetDevice to reject YAML map key as device identity")
	}
}
