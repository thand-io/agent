package thand

import (
	"fmt"
	"strings"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	localnotification "github.com/thand-io/agent/internal/providers/localnotification"
	thandFunction "github.com/thand-io/agent/internal/workflows/functions/providers/thand"
)

func (t *thandTask) isLocalNotificationProvider(providerName string) bool {
	providerName = strings.TrimSpace(providerName)
	if providerName == localnotification.ProviderName {
		return true
	}
	if t.config == nil || providerName == "" {
		return false
	}
	provider, err := t.config.GetProviderByName(providerName)
	if err == nil {
		return provider.GetProvider() == localnotification.ProviderName
	}
	if definition, ok := t.config.GetProviderDefinitions()[providerName]; ok {
		return definition.Provider == localnotification.ProviderName
	}
	return false
}

func (t *thandTask) localNotificationActivityName(providerName string) string {
	providerName = strings.TrimSpace(providerName)
	if t.config != nil && providerName != "" {
		if provider, err := t.config.GetProviderByName(providerName); err == nil &&
			provider.GetProvider() == localnotification.ProviderName {
			providerName = provider.GetIdentifier()
		}
	}
	if providerName == "" {
		providerName = localnotification.ProviderName
	}
	return models.CreateTemporalProviderWorkflowName(providerName, localnotification.SendNotificationActivityName)
}

func localNotificationPayload(
	req thandFunction.NotifierRequest,
	elevationReq *models.ElevateRequestInternal,
	title string,
	fallbackBody string,
	threadID string,
) models.NotificationRequest {
	body := strings.TrimSpace(req.Message)
	if body == "" {
		body = strings.TrimSpace(fallbackBody)
	}
	if body == "" {
		body = strings.TrimSpace(title)
	}

	localReq := models.LocalNotificationRequest{
		DeviceID: resolveLocalNotificationDevice(req, elevationReq),
		Title:    strings.TrimSpace(title),
		Body:     body,
		ThreadID: strings.TrimSpace(threadID),
	}
	if localReq.Title == "" {
		localReq.Title = "Thand notification"
	}

	var notificationPayload models.NotificationRequest
	if err := common.ConvertInterfaceToInterface(localReq, &notificationPayload); err != nil {
		logrus.WithError(err).Error("Failed to convert local notification request")
		return models.NotificationRequest{}
	}
	return notificationPayload
}

func resolveLocalNotificationDevice(
	req thandFunction.NotifierRequest,
	elevationReq *models.ElevateRequestInternal,
) string {
	if deviceID := strings.TrimSpace(req.Device); deviceID != "" {
		return deviceID
	}
	if elevationReq != nil {
		if deviceID := strings.TrimSpace(elevationReq.Device); deviceID != "" {
			return deviceID
		}
		if elevationReq.Metadata != nil {
			if raw, exists := elevationReq.Metadata["device_id"]; exists {
				if deviceID, ok := raw.(string); ok {
					return strings.TrimSpace(deviceID)
				}
			}
		}
	}
	return ""
}

func notificationPayloadDeviceID(payload models.NotificationRequest) string {
	if payload == nil {
		return ""
	}
	if raw, exists := payload["device_id"]; exists {
		if deviceID, ok := raw.(string); ok {
			return strings.TrimSpace(deviceID)
		}
	}
	return ""
}

func localNotificationTitleForRole(prefix string, elevationReq *models.ElevateRequestInternal) string {
	roleName := localPresenceRoleName(elevationReq)
	if roleName == "" {
		return prefix
	}
	return fmt.Sprintf("%s: %s", prefix, roleName)
}
