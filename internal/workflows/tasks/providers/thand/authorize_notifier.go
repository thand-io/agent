package thand

import (
	"fmt"
	"strings"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/slack-go/slack"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	emailProvider "github.com/thand-io/agent/internal/providers/email"
	localnotification "github.com/thand-io/agent/internal/providers/localnotification"
	slackProvider "github.com/thand-io/agent/internal/providers/slack"
	thandFunction "github.com/thand-io/agent/internal/workflows/functions/providers/thand"
)

// authorizerNotifier handles notifications sent to users after their access request has been approved
type authorizerNotifier struct {
	config        models.ConfigImpl
	workflowTask  *models.ElevateWorkflowTask
	elevationReq  *models.ElevateRequestInternal
	req           *thandFunction.NotifierRequest
	providerKey   string
	authRequests  map[string]*models.AuthorizeRoleRequest
	authResponses map[string]*models.AuthorizeRoleResponse
}

// NewAuthorizerNotifier creates a new notifier for sending approval confirmation notifications
func NewAuthorizerNotifier(
	config models.ConfigImpl,
	workflowTask *models.ElevateWorkflowTask,
	elevationReq *models.ElevateRequestInternal,
	req *thandFunction.NotifierRequest,
	providerKey string,
	requests map[string]*models.AuthorizeRoleRequest,
	authorizations map[string]*models.AuthorizeRoleResponse,
) NotifierImpl {
	return &authorizerNotifier{
		config:        config,
		workflowTask:  workflowTask,
		elevationReq:  elevationReq,
		req:           req,
		providerKey:   providerKey,
		authRequests:  requests,
		authResponses: authorizations,
	}
}

func (a *authorizerNotifier) GetRecipients() []string {
	return a.req.To
}

func (a *authorizerNotifier) GetCallFunction(toIdentity *models.Identity) model.CallFunction {

	callMap := (&thandFunction.NotifierRequest{
		Provider: a.req.Provider,
		To:       []string{toIdentity.GetEmail()},
	}).AsMap()

	return model.CallFunction{
		Call: thandFunction.ThandNotifyFunction,
		With: callMap,
	}
}

func (a *authorizerNotifier) GetProviderName() string {
	return a.req.Provider
}

func (a *authorizerNotifier) GetPayload(toIdentity *models.Identity) models.NotificationRequest {

	elevationReq := a.elevationReq
	var notificationPayload models.NotificationRequest

	if strings.Compare(a.GetProviderName(), slackProvider.SlackProviderName) == 0 {

		blocks := a.createAuthorizeSlackBlocks(toIdentity)

		slackReq := slackProvider.SlackNotificationRequest{
			To: toIdentity.GetEmail(),
			Text: fmt.Sprintf("Your access request for role %s has been approved", func() string {
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
		plainText, html := a.createAuthorizeEmailBody()
		emailReq := models.EmailNotificationRequest{
			To:      []string{toIdentity.GetEmail()},
			Subject: "Access Request Approved",
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
	} else if strings.Compare(a.GetProviderName(), localnotification.ProviderName) == 0 {
		return localNotificationPayload(
			*a.req,
			elevationReq,
			localNotificationTitleForRole("Access approved", elevationReq),
			fmt.Sprintf("Your access request for role %s has been approved", localPresenceRoleName(elevationReq)),
			a.workflowTask.GetWorkflowID(),
		)
	} else {
		logrus.WithField("provider", a.GetProviderName()).Error("Unsupported provider type")
		return models.NotificationRequest{}
	}

	return notificationPayload
}
