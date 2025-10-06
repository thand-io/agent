package slack

import (
	"context"
	"fmt"
	"strings"

	"github.com/slack-go/slack"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers"
)

// slackProvider implements the ProviderImpl interface for Slack
type slackProvider struct {
	*models.BaseProvider
	client *slack.Client
}

func (p *slackProvider) Initialize(provider models.Provider) error {
	p.BaseProvider = models.NewBaseProvider(
		provider,
		models.ProviderCapabilityNotifier,
	)

	slackConfig := p.GetConfig()

	token, foundToken := slackConfig.GetString("bot_token")
	if !foundToken {
		return fmt.Errorf("missing Slack bot_token configuration")
	}

	// Initialize Slack client
	p.client = slack.New(token)

	// Optional: Test the connection
	_, err := p.client.AuthTest()
	if err != nil {
		return fmt.Errorf("failed to authenticate with Slack: %w", err)
	}

	return nil
}

type SlackNotificationRequest struct {
	To     string       `json:"channel"`
	Text   string       `json:"text,omitempty"`
	Blocks slack.Blocks `json:"blocks,omitempty"`

	Attachments []slack.Attachment `json:"attachments,omitempty"`
}

func (p *slackProvider) SendNotification(ctx context.Context, notification models.NotificationRequest) error {
	// Convert NotificationRequest to SlackNotificationRequest
	slackRequest := &SlackNotificationRequest{}
	common.ConvertMapToInterface(notification, slackRequest)

	// Validate required fields
	if len(slackRequest.To) == 0 {
		return fmt.Errorf("to is required for Slack notification")
	}

	if strings.HasPrefix(slackRequest.To, "#") {
		// Lookup channel ID using the channel name via the API
		return fmt.Errorf("channel name lookup not implemented, please provide a Channel ID (C...)")
	} else if strings.HasPrefix(slackRequest.To, "@") {
		// Lookup user ID using the user name via the API
		username := strings.TrimPrefix(slackRequest.To, "@")
		userID, err := p.getUserIDByUsername(ctx, username)
		if err != nil {
			return fmt.Errorf("failed to get user ID for user %s: %w", slackRequest.To, err)
		}
		slackRequest.To = userID
	} else if strings.Contains(slackRequest.To, "@") {
		// Is an email address
		email := strings.TrimSpace(slackRequest.To)
		user, err := p.client.GetUserByEmail(email)
		if err != nil {
			return fmt.Errorf("failed to get user by email: %w", err)
		}
		slackRequest.To = user.ID
	}

	// Now lets double check we hav a valid Channel Id or User Id for our request
	if !strings.HasPrefix(slackRequest.To, "C") && !strings.HasPrefix(slackRequest.To, "U") {
		return fmt.Errorf("invalid to field for Slack notification: %s expects a Channel ID (C...) or User ID (U...)", slackRequest.To)
	}

	// Build message options
	options := []slack.MsgOption{
		slack.MsgOptionText(slackRequest.Text, false),
	}

	// Add optional parameters
	//options = append(options, slack.MsgOptionUsername("Thand"))
	//options = append(options, slack.MsgOptionIconURL("https://providers.thand.io/slack/icon.png"))
	//options = append(options, slack.MsgOptionIconEmoji("rocket"))

	if len(slackRequest.Attachments) > 0 {
		options = append(options, slack.MsgOptionAttachments(slackRequest.Attachments...))
	}

	if len(slackRequest.Blocks.BlockSet) > 0 {
		options = append(options, slack.MsgOptionBlocks(slackRequest.Blocks.BlockSet...))
	}

	// Send the message
	_, _, err := p.client.PostMessageContext(ctx, slackRequest.To, options...)
	if err != nil {
		return fmt.Errorf("failed to send Slack message: %w", err)
	}

	return nil
}

// getUserIDByUsername searches for a user by username and returns their ID
func (p *slackProvider) getUserIDByUsername(ctx context.Context, username string) (string, error) {
	// Get list of users
	users, err := p.client.GetUsersContext(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to list users: %w", err)
	}

	// Search for user by username
	for _, user := range users {
		if user.Name == username {
			return user.ID, nil
		}
	}

	return "", fmt.Errorf("user not found: %s", username)
}

func init() {
	providers.Register("slack", &slackProvider{})
}
