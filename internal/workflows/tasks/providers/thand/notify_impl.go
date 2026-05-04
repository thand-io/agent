package thand

import (
	"strings"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	emailProvider "github.com/thand-io/agent/internal/providers/email"
	localNotificationProvider "github.com/thand-io/agent/internal/providers/local.notification"
	localPresenceProvider "github.com/thand-io/agent/internal/providers/local.presence"
	slackProvider "github.com/thand-io/agent/internal/providers/slack"
	thandFunction "github.com/thand-io/agent/internal/workflows/functions/providers/thand"
)

const defaultLocalNotificationTitle = "Workflow Notification"

type NotifierImpl interface {
	GetProviderName() string
	GetRecipients() []string
	GetCallFunction(toIdentity *models.Identity) model.CallFunction
	GetPayload(toIdentity *models.Identity) models.NotificationRequest
}

type defaultNotifierImpl struct {
	req thandFunction.NotifierRequest
}

func NewDefaultNotifierImpl(req thandFunction.NotifierRequest) NotifierImpl {
	return &defaultNotifierImpl{
		req: req,
	}
}

func (d *defaultNotifierImpl) GetRecipients() []string {
	return d.req.To
}

func (d *defaultNotifierImpl) GetCallFunction(toIdentity *models.Identity) model.CallFunction {

	callMap := (&thandFunction.NotifierRequest{
		Provider: d.req.Provider,
		To:       []string{toIdentity.GetEmail()},
	}).AsMap()

	return model.CallFunction{
		Call: thandFunction.ThandNotifyFunction,
		With: callMap,
	}
}

func (d *defaultNotifierImpl) GetProviderName() string {
	return d.req.Provider
}

func (d *defaultNotifierImpl) GetPayload(toIdentity *models.Identity) models.NotificationRequest {

	if strings.Compare(d.GetProviderName(), slackProvider.SlackProviderName) == 0 {
		return d.GetSlackPayload(toIdentity)
	} else if strings.HasPrefix(d.GetProviderName(), emailProvider.EmailProviderName) {
		return d.GetEmailPayload(toIdentity)
	} else if strings.Compare(d.GetProviderName(), localNotificationProvider.ProviderName) == 0 {
		return d.GetLocalNotificationPayload(toIdentity)
	} else if strings.Compare(d.GetProviderName(), localPresenceProvider.ProviderName) == 0 {
		return d.GetLocalPresencePayload(toIdentity)
	} else {
		return models.NotificationRequest{}
	}

}

func (d *defaultNotifierImpl) GetEmailPayload(toIdentity *models.Identity) models.NotificationRequest {

	notificationReq := d.req

	// Render HTML email using template
	html, err := RenderEmail("Workflow Notification", notificationReq.Message)
	if err != nil {
		logrus.WithError(err).Error("Failed to render email template")
		// Fallback to plain message if template fails
		// TODO: format markdown
		html = notificationReq.Message
	}

	emailReq := models.EmailNotificationRequest{
		To:      []string{toIdentity.GetEmail()},
		Subject: "Workflow Notification",
		Body: models.EmailNotificationBody{
			Text: notificationReq.Message,
			HTML: html,
		},
	}

	var notificationPayload models.NotificationRequest
	err = common.ConvertInterfaceToInterface(emailReq, &notificationPayload)

	if err != nil {
		logrus.WithError(err).Error("Failed to convert email request")
		return models.NotificationRequest{}
	}

	return notificationPayload
}

func (d *defaultNotifierImpl) GetSlackPayload(toIdentity *models.Identity) models.NotificationRequest {

	notificationReq := d.req

	slackReq := slackProvider.SlackNotificationRequest{
		To:   toIdentity.GetEmail(),
		Text: notificationReq.Message,
		Blocks: slack.Blocks{
			BlockSet: []slack.Block{
				slack.NewSectionBlock(
					slack.NewTextBlockObject("mrkdwn", notificationReq.Message, false, false),
					nil,
					nil,
				),
			},
		},
	}

	var notificationPayload models.NotificationRequest
	err := common.ConvertInterfaceToInterface(slackReq, &notificationPayload)

	if err != nil {
		logrus.WithError(err).Error("Failed to convert slack request")
		return models.NotificationRequest{}
	}

	return notificationPayload
}

// GetLocalNotificationPayload renders a NotifierRequest as a macOS local
// notification payload. The notifier message becomes the body and the
// recipient identity becomes the target device, mirroring the email/slack
// pattern. Title falls back to a generic workflow notification subject so the
// caller can issue a short alert without crafting a structured payload.
func (d *defaultNotifierImpl) GetLocalNotificationPayload(toIdentity *models.Identity) models.NotificationRequest {
	return BuildLocalNotificationPayload(toIdentity, defaultLocalNotificationTitle, "Open Thand for details")
}

// GetLocalPresencePayload renders a NotifierRequest as a local-presence
// challenge. The notifier message is surfaced as the Touch ID prompt and the
// recipient becomes the target device. Callers using local-presence as a
// notifier are issuing a fire-and-forget challenge; the approval result is
// logged by the provider but not returned through the notifier interface.
func (d *defaultNotifierImpl) GetLocalPresencePayload(toIdentity *models.Identity) models.NotificationRequest {
	return BuildLocalPresencePayload(toIdentity, "Approve request")
}

// BuildLocalNotificationPayload constructs a NotificationRequest for the
// local-notification provider from a title + body pair. The title is
// required by the provider so callers must supply a non-empty value;
// passing an empty title here causes the provider to reject the payload
// with a non-retryable application error.
func BuildLocalNotificationPayload(toIdentity *models.Identity, title, body string) models.NotificationRequest {
	if len(strings.TrimSpace(title)) == 0 {
		title = defaultLocalNotificationTitle
	}

	localReq := models.LocalNotificationRequest{
		DeviceID: toIdentity.GetEmail(),
		Title:    title,
		Body:     body,
	}

	var notificationPayload models.NotificationRequest
	if err := common.ConvertInterfaceToInterface(localReq, &notificationPayload); err != nil {
		logrus.WithError(err).Error("Failed to convert local notification request")
		return models.NotificationRequest{}
	}

	return notificationPayload
}

// BuildLocalPresencePayload constructs a NotificationRequest for the
// local-presence provider. The prompt is shown to the user during the
// Touch ID / passkey challenge and is the only user-visible string the
// notifier surface offers, so callers should pass a short human-readable
// summary of the action being authorized.
func BuildLocalPresencePayload(toIdentity *models.Identity, prompt string) models.NotificationRequest {
	presenceReq := models.LocalPresenceApprovalRequest{
		DeviceID: toIdentity.GetEmail(),
		TaskName: thandFunction.ThandNotifyFunction,
		Prompt:   prompt,
	}

	var notificationPayload models.NotificationRequest
	if err := common.ConvertInterfaceToInterface(presenceReq, &notificationPayload); err != nil {
		logrus.WithError(err).Error("Failed to convert local presence request")
		return models.NotificationRequest{}
	}

	return notificationPayload
}
