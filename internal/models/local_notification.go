package models

type LocalNotificationRequest struct {
	NotificationID string `json:"notification_id,omitempty"`
	DeviceID       string `json:"device_id,omitempty"`
	Title          string `json:"title"`
	Subtitle       string `json:"subtitle,omitempty"`
	Body           string `json:"body"`
	ThreadID       string `json:"thread_id,omitempty"`
}
