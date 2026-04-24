package cli

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/thand-io/agent/internal/common"
	configpkg "github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
)

func TestBuildLocalSudoElevationRequestTimed(t *testing.T) {
	previousCfg := cfg
	t.Cleanup(func() { cfg = previousCfg })

	cfg = newTestSudoConfig("local-custom")

	request, err := buildLocalSudoElevationRequest(nil, "system maintenance", "30m", "device-alpha")
	if err != nil {
		t.Fatalf("buildLocalSudoElevationRequest returned error: %v", err)
	}

	if got, want := request.Workflow, models.LocalSudoTimedWorkflowName; got != want {
		t.Fatalf("workflow = %q, want %q", got, want)
	}
	if got, want := request.Duration, "30m"; got != want {
		t.Fatalf("duration = %q, want %q", got, want)
	}
	if got, want := request.Providers[0], "local-custom"; got != want {
		t.Fatalf("provider = %q, want %q", got, want)
	}
	if request.Metadata["mode"] != string(models.LocalSudoModeTimed) {
		t.Fatalf("mode = %#v, want %q", request.Metadata["mode"], models.LocalSudoModeTimed)
	}
	if got, want := request.Device, "device-alpha"; got != want {
		t.Fatalf("device = %#v, want %q", got, want)
	}
	if got, want := request.Metadata["device_id"], "device-alpha"; got != want {
		t.Fatalf("device_id = %#v, want %q", got, want)
	}
	if !containsString(request.Role.Providers, "local-custom") {
		t.Fatalf("request role providers = %#v, want provider alias included", request.Role.Providers)
	}
}

func TestBuildLocalSudoElevationRequestCommandUsesDefaultDuration(t *testing.T) {
	previousCfg := cfg
	t.Cleanup(func() { cfg = previousCfg })

	cfg = newTestSudoConfig("local-elevation")

	request, err := buildLocalSudoElevationRequestForOS("linux", []string{"whoami"}, "check user", "", "device-beta")
	if err != nil {
		t.Fatalf("buildLocalSudoElevationRequest returned error: %v", err)
	}

	if got, want := request.Workflow, models.LocalSudoCommandWorkflowName; got != want {
		t.Fatalf("workflow = %q, want %q", got, want)
	}
	if got, want := request.Duration, models.LocalSudoCommandDuration; got != want {
		t.Fatalf("duration = %q, want %q", got, want)
	}
	command, ok := request.Metadata["command"].([]string)
	if !ok {
		t.Fatalf("metadata command type = %T, want []string", request.Metadata["command"])
	}
	if len(command) != 1 || command[0] != "whoami" {
		t.Fatalf("metadata command = %#v, want [\"whoami\"]", command)
	}
	if got, want := request.Metadata["device_id"], "device-beta"; got != want {
		t.Fatalf("device_id = %#v, want %q", got, want)
	}
}

func TestBuildLocalSudoElevationRequestDarwinCommandIsUnsupported(t *testing.T) {
	previousCfg := cfg
	t.Cleanup(func() { cfg = previousCfg })

	cfg = newTestSudoConfig("local-elevation")

	if _, err := buildLocalSudoElevationRequestForOS("darwin", []string{"whoami"}, "check user", "", "device-beta"); err == nil {
		t.Fatal("expected Darwin command-mode error")
	}
}

func TestBuildLocalSudoElevationRequestRequiresTimedDuration(t *testing.T) {
	previousCfg := cfg
	t.Cleanup(func() { cfg = previousCfg })

	cfg = newTestSudoConfig("local-elevation")

	if _, err := buildLocalSudoElevationRequest(nil, "missing duration", "", "device-alpha"); err == nil {
		t.Fatal("expected error for missing timed duration")
	}
}

func TestBuildLocalSudoElevationRequestPrefersLocalElevationProvider(t *testing.T) {
	previousCfg := cfg
	t.Cleanup(func() { cfg = previousCfg })

	cfg = &configpkg.Config{
		Providers: configpkg.ProviderDefinitionsConfig{
			Definitions: map[string]models.ProviderConfig{
				"local": {
					Name:     "Local",
					Provider: "local",
					Enabled:  true,
				},
				"local-elevation": {
					Name:     "Local Elevation",
					Provider: "local",
					Enabled:  true,
				},
			},
		},
		Roles: configpkg.RoleConfig{
			Definitions: map[string]models.Role{
				models.LocalSudoRoleIdentifier: {
					Name:       "Local Sudo",
					Identifier: models.LocalSudoRoleIdentifier,
					Providers:  []string{"local", "local-elevation"},
					Workflows:  []string{models.LocalSudoTimedWorkflowName, models.LocalSudoCommandWorkflowName},
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{Operations: []string{"local:sudo:*"}},
						},
					},
				},
			},
		},
	}

	request, err := buildLocalSudoElevationRequest(nil, "system maintenance", "30m", "device-alpha")
	if err != nil {
		t.Fatalf("buildLocalSudoElevationRequest returned error: %v", err)
	}

	if got, want := request.Providers[0], "local-elevation"; got != want {
		t.Fatalf("provider = %q, want %q", got, want)
	}
}

func newTestSudoConfig(providerName string) *configpkg.Config {
	return &configpkg.Config{
		Providers: configpkg.ProviderDefinitionsConfig{
			Definitions: map[string]models.ProviderConfig{
				providerName: {
					Name:     "Local",
					Provider: "local",
					Enabled:  true,
				},
			},
		},
		Roles: configpkg.RoleConfig{
			Definitions: map[string]models.Role{
				models.LocalSudoRoleIdentifier: {
					Name:       "Local Sudo",
					Identifier: models.LocalSudoRoleIdentifier,
					Providers:  []string{"local", "local-elevation"},
					Workflows:  []string{models.LocalSudoTimedWorkflowName, models.LocalSudoCommandWorkflowName},
					Permissions: models.RolePermissions{
						Allow: models.RoleStatements{
							{Operations: []string{"local:sudo:*"}},
						},
					},
				},
			},
		},
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestBuildLocalSudoElevationRequestRequiresConfiguredEnvironment(t *testing.T) {
	previousCfg := cfg
	t.Cleanup(func() { cfg = previousCfg })

	cfg = nil

	if _, err := buildLocalSudoElevationRequest(nil, "system maintenance", "30m", ""); err == nil {
		t.Fatal("expected error when config is unavailable")
	}
}

func TestSudoCommandDefaultsDeviceToCurrentMachineWhenFlagOmitted(t *testing.T) {
	previousCfg := cfg
	t.Cleanup(func() { cfg = previousCfg })

	cfg = newTestSudoConfig("local-elevation")

	cmd := &cobra.Command{Use: "sudo"}
	cmd.Flags().String("device", "", "")

	device, err := cmd.Flags().GetString("device")
	if err != nil {
		t.Fatalf("GetString(device) returned error: %v", err)
	}
	if cmd.Flags().Changed("device") {
		t.Fatal("device flag should not be marked changed when omitted")
	}
	if !cmd.Flags().Changed("device") {
		device = common.GetDeviceID().String()
	}

	request, err := buildLocalSudoElevationRequest(nil, "system maintenance", "30m", device)
	if err != nil {
		t.Fatalf("buildLocalSudoElevationRequest returned error: %v", err)
	}

	if got, want := request.Device, common.GetDeviceID().String(); got != want {
		t.Fatalf("device = %q, want %q", got, want)
	}
	if got, want := request.Metadata["device_id"], common.GetDeviceID().String(); got != want {
		t.Fatalf("metadata device_id = %#v, want %q", got, want)
	}
}

func TestSudoCommandPreservesExplicitEmptyDeviceFlag(t *testing.T) {
	previousCfg := cfg
	t.Cleanup(func() { cfg = previousCfg })

	cfg = newTestSudoConfig("local-elevation")

	cmd := &cobra.Command{Use: "sudo"}
	cmd.Flags().String("device", "", "")
	if err := cmd.Flags().Set("device", ""); err != nil {
		t.Fatalf("Set(device) returned error: %v", err)
	}

	device, err := cmd.Flags().GetString("device")
	if err != nil {
		t.Fatalf("GetString(device) returned error: %v", err)
	}
	if !cmd.Flags().Changed("device") {
		t.Fatal("device flag should be marked changed when explicitly set")
	}

	if _, err := buildLocalSudoElevationRequest(nil, "system maintenance", "30m", device); err == nil {
		t.Fatal("expected explicit empty device flag to remain empty and fail validation")
	}
}
