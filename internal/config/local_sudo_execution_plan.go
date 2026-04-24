package config

import (
	"fmt"
	"strings"

	"github.com/thand-io/agent/internal/models"
)

type localSudoExecutionPlanDecorator struct{}

func (localSudoExecutionPlanDecorator) Applies(elevateRequest *models.ElevateRequestInternal) bool {
	return elevateRequest != nil && models.IsLocalSudoRequest(&elevateRequest.ElevateRequest)
}

func (localSudoExecutionPlanDecorator) Decorate(
	cfg models.ConfigImpl,
	req *models.WorkflowRoleRequest,
	elevateRequest *models.ElevateRequestInternal,
	opts executionPlanBuildOptions,
) error {
	meta, err := buildLocalSudoRequestMetadata(cfg, elevateRequest, req.Identity, req.ResolvedIdentity, opts.LookupDeviceDefinition)
	if err != nil {
		return err
	}

	req.DeviceID = meta.DeviceID
	req.Metadata = meta.AsMap()
	return nil
}

func (localSudoExecutionPlanDecorator) Finalize(
	req *models.WorkflowRoleRequest,
	elevateRequest *models.ElevateRequestInternal,
	entryID string,
) error {
	meta, err := models.DecodeLocalSudoRequestMetadata(req.Metadata)
	if err != nil {
		return err
	}
	meta.GrantID = entryID
	req.Metadata = meta.AsMap()
	return nil
}

func buildLocalSudoRequestMetadata(
	cfg models.ConfigImpl,
	elevateRequest *models.ElevateRequestInternal,
	identityID string,
	resolvedIdentity *models.Identity,
	lookupDeviceDefinition func(deviceID string) (*models.Device, error),
) (models.LocalSudoRequestMetadata, error) {
	meta, err := models.DecodeLocalSudoRequestMetadata(elevateRequest.Metadata)
	if err != nil {
		return meta, err
	}

	deviceID := strings.TrimSpace(elevateRequest.Device)
	if deviceID == "" {
		deviceID = strings.TrimSpace(meta.DeviceID)
	}
	if deviceID == "" {
		return meta, fmt.Errorf("local sudo request is missing a device_id")
	}

	if lookupDeviceDefinition == nil {
		lookupDeviceDefinition = cfg.GetDevice
	}

	device, err := lookupDeviceDefinition(deviceID)
	if err != nil {
		return meta, err
	}
	if !device.Enabled {
		return meta, fmt.Errorf("device %q is disabled", deviceID)
	}
	if device.LocalElevation == nil {
		return meta, fmt.Errorf("device %q does not have local elevation configured", deviceID)
	}
	if !device.LocalElevation.AllowsMode(string(meta.Mode)) {
		return meta, fmt.Errorf("device %q does not allow local sudo mode %q", deviceID, meta.Mode)
	}

	identity := resolvedIdentity
	if identity == nil {
		identity, err = cfg.GetIdentity(identityID)
	}
	if err != nil || identity == nil {
		identity = &models.Identity{
			ID: identityID,
			User: &models.User{
				Email: identityID,
			},
		}
	}

	localUsername, err := device.LocalElevation.ResolveLocalUsername(identityID, identity)
	if err != nil {
		return meta, err
	}

	meta.DeviceID = device.ID
	meta.LocalUsername = localUsername
	meta.DeniedUsernames = append([]string(nil), device.LocalElevation.DeniedUsernames...)
	meta.AllowedUIDRanges = append([]string(nil), device.LocalElevation.AllowedUIDRanges...)
	return meta, nil
}
