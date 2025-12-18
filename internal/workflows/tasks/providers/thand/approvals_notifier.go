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

type ApprovalNotifier struct {
	Approvals   int                           `json:"approvals" default:"1"`
	SelfApprove bool                          `json:"selfApprove" default:"false"`
	Notifier    thandFunction.NotifierRequest `json:"notifier"`
	Entrypoint  string                        `json:"entrypoint"`
}

type approvalsNotifier struct {
	config       *config.Config
	workflowTask *models.WorkflowTask
	elevationReq *models.ElevateRequestInternal
	req          *ApprovalNotifier
}

func NewApprovalsNotifier(
	config *config.Config,
	workflowTask *models.WorkflowTask,
	elevationReq *models.ElevateRequestInternal,
	req *ApprovalNotifier,
) NotifierImpl {
	return &approvalsNotifier{
		config:       config,
		workflowTask: workflowTask,
		elevationReq: elevationReq,
		req:          req,
	}
}

func (a *approvalsNotifier) GetRecipients() []string {
	return a.req.Notifier.To
}

func (a *approvalsNotifier) GetCallFunction(toIdentity *models.Identity) model.CallFunction {
	callMap := (&thandFunction.NotifierRequest{
		Provider: a.req.Notifier.Provider,
		To:       []string{toIdentity.GetEmail()},
	}).AsMap()

	return model.CallFunction{
		Call: thandFunction.ThandNotifyFunction,
		With: callMap,
	}
}

func (a *approvalsNotifier) GetProviderName() string {
	return a.req.Notifier.Provider
}

func (a *approvalsNotifier) GetPayload(toIdentity *models.Identity) models.NotificationRequest {

	elevationReq := a.elevationReq
	var notificationPayload models.NotificationRequest

	if strings.Compare(a.GetProviderName(), slackProvider.SlackProviderName) == 0 {

		blocks := a.createApprovalSlackBlocks()
		slackReq := slackProvider.SlackNotificationRequest{
			To: toIdentity.GetEmail(),
			Text: fmt.Sprintf("Access request for role %s", func() string {
				if elevationReq.Role != nil {
					return elevationReq.Role.Name
				}
				return "unknown"
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
	} else if strings.HasPrefix(a.GetProviderName(), emailProvider.EmailProviderName) {
		plainText, html := a.createApprovalEmailBody()
		emailReq := models.EmailNotificationRequest{
			To:      []string{toIdentity.GetEmail()},
			Subject: "Access Request - Approval Required",
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
		logrus.WithField("provider", a.GetProviderName()).Error("Unsupported provider type")
		return models.NotificationRequest{}
	}

	return notificationPayload
}
