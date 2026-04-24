package thand

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/sdk/workflows/functions"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
)

const ThandNotifyFunction = "thand.notify"

// NotifyFunction implements access request notification using LLM or user input
type notifyFunction struct {
	config models.ConfigImpl
	*functions.BaseFunction
}

// NewValidateFunction creates a new validation Function
func NewNotifyFunction(config models.ConfigImpl) *notifyFunction {
	return &notifyFunction{
		config: config,
		BaseFunction: functions.NewBaseFunction(
			ThandNotifyFunction,
			"This provides notification capabilities",
			"1.0.0",
		),
	}
}

// GetRequiredParameters returns the required parameters for validation
func (t *notifyFunction) GetRequiredParameters() []string {
	return []string{
		"provider",
	} // No strictly required parameters
}

// GetOptionalParameters returns optional parameters with defaults
func (t *notifyFunction) GetOptionalParameters() map[string]any {
	return map[string]any{
		"provider": "email",
	}
}

// ValidateRequest validates the input parameters
func (t *notifyFunction) ValidateRequest(
	workflowTask sdkWorkflowsModel.WorkflowTaskSupport,
	call *model.CallFunction,
	input any,
) error {

	req := workflowTask.GetContextAsMap()

	if req == nil {
		return errors.New("request cannot be nil")
	}

	var notificationReq NotifierRequest
	common.ConvertMapToInterface(call.With, &notificationReq)

	// Get requesting user info
	requestingUser := models.NewElevateWorkflowTask(workflowTask).GetUser()

	if requestingUser == nil {
		return errors.New("requesting user cannot be nil")
	}

	logrus.WithFields(logrus.Fields{
		"provider": notificationReq.Provider,
		"user":     requestingUser.Name,
	}).Info("Validating notifier")

	notifierProviders := t.config.GetProvidersByCapabilityWithUser(
		requestingUser, models.ProviderCapabilityNotifier)

	// filter out providers to see if the name matches
	for _, provider := range notifierProviders {
		if strings.Compare(provider.GetName(), notificationReq.Provider) == 0 {
			return nil
		} else if strings.Compare(provider.GetProvider(), notificationReq.Provider) == 0 {
			return nil
		}
	}

	return errors.New("invalid notifier")
}

/*
provider: slack # or slack, email
to: "#access-requests"  # Can be a string or array of strings
message: "Workflow validation passed for user ${ $.user.name }"
approvals: true
*/
type NotifierRequest struct {
	Provider string   `json:"provider"`
	To       []string `json:"-"`       // Email, channel Id, username etc. - handled by custom marshal/unmarshal
	Message  string   `json:"message"` // Message body
	Device   string   `json:"device,omitempty"`
}

// UnmarshalJSON implements custom JSON unmarshaling to handle both string and []string for To field
func (r *NotifierRequest) UnmarshalJSON(data []byte) error {
	// Create a temporary struct with the same fields but To as any
	type Alias struct {
		Provider string `json:"provider"`
		To       any    `json:"to"`
		Message  string `json:"message"`
		Device   string `json:"device"`
	}

	var temp Alias
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}

	r.Provider = temp.Provider
	r.Message = temp.Message
	r.Device = temp.Device

	// Handle To field - can be string or []string
	switch v := temp.To.(type) {
	case nil:
		return fmt.Errorf("a contact 'to' must be provided for notifiers")
	case string:
		// Split by comma if it contains commas
		if strings.Contains(v, ",") {
			for _, recipient := range strings.Split(v, ",") {
				recipient = strings.TrimSpace(recipient)
				if len(recipient) > 0 {
					r.To = append(r.To, recipient)
				}
			}
		} else {
			r.To = []string{v}
		}
	case []any:
		for _, item := range v {
			if str, ok := item.(string); ok {
				r.To = append(r.To, str)
			}
		}
	case []string:
		r.To = v
	default:
		return fmt.Errorf("invalid type for 'to' field: %T", v)
	}

	return nil
}

func (r *NotifierRequest) IsValid() bool {
	return len(r.Provider) > 0 && len(r.To) > 0
}

func (r *NotifierRequest) AsMap() map[string]any {
	// Return 'to' as array for consistency
	return map[string]any{
		"provider": r.Provider,
		"to":       r.To,
		"message":  r.Message,
		"device":   r.Device,
	}
}

// Execute performs the validation logic
func (t *notifyFunction) Execute(
	workflowTask sdkWorkflowsModel.WorkflowTaskSupport,
	call *model.CallFunction,
	input any,
) (any, error) {

	req := workflowTask.GetContextAsMap()

	if req == nil {
		return nil, errors.New("request cannot be nil")
	}

	var notificationReq NotifierRequest
	err := common.ConvertMapToInterface(call.With, &notificationReq)

	if err != nil {
		return nil, fmt.Errorf("failed to convert request: %w", err)
	}

	if !notificationReq.IsValid() {
		return nil, errors.New("elevation request is not valid")
	}

	elevationReq, err := models.NewElevateWorkflowTask(workflowTask).GetContextAsElevationRequest()

	if err != nil {
		return nil, fmt.Errorf("failed to get elevation request from input: %w", err)
	}

	if !elevationReq.IsValid() {
		return nil, errors.New("elevation request is not valid")
	}

	foundProvider := notificationReq.Provider

	if len(foundProvider) == 0 {
		return nil, errors.New("provider must be specified in the with block")
	}

	// Get server config to fetch provider
	provider, err := t.config.GetProviderByName(foundProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to get provider config: %w", err)
	}

	logrus.WithFields(logrus.Fields{
		"provider": provider.GetName(),
	}).Info("Executing notification")

	// Overwrite the notification request with the converted input
	err = common.ConvertMapToInterface(call.With, &notificationReq)

	if err != nil {
		logrus.Warn("Failed to convert notification input")
		return nil, errors.New("failed to convert notification input")
	}

	var notificationPayload models.NotificationRequest
	err = common.ConvertInterfaceToInterface(input, &notificationPayload)

	if err != nil {
		return nil, fmt.Errorf("failed to convert notification payload: %w", err)
	}

	err = provider.SendNotification(
		workflowTask.GetContext(), notificationPayload)

	if err != nil {
		return nil, fmt.Errorf("failed to send notification: %w", err)
	}

	return nil, err
}
