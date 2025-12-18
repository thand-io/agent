package thand

import (
	"fmt"
	"strings"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	emailProvider "github.com/thand-io/agent/internal/providers/email"
	slackProvider "github.com/thand-io/agent/internal/providers/slack"
	thandFunction "github.com/thand-io/agent/internal/workflows/functions/providers/thand"
)

// FormNotifierConfig holds the configuration for form notifications
type FormNotifierConfig struct {
	Title       string                        `json:"title,omitempty"`
	Description string                        `json:"description,omitempty"`
	SubmitLabel string                        `json:"submit_label,omitempty"`
	Blocks      []slack.Block                 `json:"blocks,omitempty"`
	Notifier    thandFunction.NotifierRequest `json:"notifier"`
	Entrypoint  string                        `json:"entrypoint"`
}

type formNotifier struct {
	config       *config.Config
	workflowTask *models.WorkflowTask
	elevationReq *models.ElevateRequestInternal
	req          *FormNotifierConfig
}

// NewFormNotifier creates a new form notifier
func NewFormNotifier(
	config *config.Config,
	workflowTask *models.WorkflowTask,
	elevationReq *models.ElevateRequestInternal,
	req *FormNotifierConfig,
) NotifierImpl {
	return &formNotifier{
		config:       config,
		workflowTask: workflowTask,
		elevationReq: elevationReq,
		req:          req,
	}
}

func (f *formNotifier) GetRecipients() []string {
	return f.req.Notifier.To
}

func (f *formNotifier) GetCallFunction(toIdentity *models.Identity) model.CallFunction {
	callMap := (&thandFunction.NotifierRequest{
		Provider: f.req.Notifier.Provider,
		To:       []string{toIdentity.GetEmail()},
	}).AsMap()

	return model.CallFunction{
		Call: thandFunction.ThandNotifyFunction,
		With: callMap,
	}
}

func (f *formNotifier) GetProviderName() string {
	return f.req.Notifier.Provider
}

func (f *formNotifier) GetPayload(toIdentity *models.Identity) models.NotificationRequest {
	var notificationPayload models.NotificationRequest

	if strings.Compare(f.GetProviderName(), slackProvider.SlackProviderName) == 0 {
		// For Slack: Send blocks directly as an interactive message
		blocks := f.createFormSlackBlocks()
		slackReq := slackProvider.SlackNotificationRequest{
			To: toIdentity.GetEmail(),
			Text: fmt.Sprintf("Form: %s", func() string {
				if len(f.req.Title) > 0 {
					return f.req.Title
				}
				return "Please fill out this form"
			}()),
			Blocks: slack.Blocks{
				BlockSet: blocks,
			},
		}
		err := common.ConvertInterfaceToInterface(slackReq, &notificationPayload)
		if err != nil {
			logrus.WithError(err).Error("Failed to convert slack request")
			return models.NotificationRequest{}
		}
	} else if strings.HasPrefix(f.GetProviderName(), emailProvider.EmailProviderName) {
		// For Email: Send a link to the HTML form page
		plainText, html := f.createFormEmailBody(toIdentity)
		emailReq := models.EmailNotificationRequest{
			To:      []string{toIdentity.GetEmail()},
			Subject: f.getEmailSubject(),
			Body: models.EmailNotificationBody{
				Text: plainText,
				HTML: html,
			},
		}
		err := common.ConvertInterfaceToInterface(emailReq, &notificationPayload)
		if err != nil {
			logrus.WithError(err).Error("Failed to convert email request")
			return models.NotificationRequest{}
		}
	} else {
		logrus.WithField("provider", f.GetProviderName()).Error("Unsupported provider type for form notification")
		return models.NotificationRequest{}
	}

	return notificationPayload
}

func (f *formNotifier) getEmailSubject() string {
	if len(f.req.Title) > 0 {
		return fmt.Sprintf("Form Required: %s", f.req.Title)
	}
	return "Form Required - Please Complete"
}
