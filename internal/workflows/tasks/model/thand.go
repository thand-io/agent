package model

import (
	"fmt"

	"github.com/go-playground/validator/v10"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

const ThandTaskName = "thand"

// Thand task type constants
const (
	ThandTypeApprovals = "approvals"
	ThandTypeValidate  = "validate"
	ThandTypeAuthorize = "authorize"
	ThandTypeNotify    = "notify"
	ThandTypeRevoke    = "revoke"
	ThandTypeMonitor   = "monitor"
	ThandTypeForm      = "form"
	ThandTypeAgent     = "agent"
)

// ValidThandTypes contains all valid thand task types
var ValidThandTypes = []string{
	ThandTypeApprovals,
	ThandTypeValidate,
	ThandTypeAuthorize,
	ThandTypeNotify,
	ThandTypeRevoke,
	ThandTypeMonitor,
	ThandTypeForm,
	ThandTypeAgent,
}

// ThandTask defines a custom Thand task
type ThandTask struct {
	model.TaskBase `json:",inline"`    // Inline TaskBase fields
	Thand          string              `json:"thand" validate:"required,thand_type"`
	On             *models.BasicConfig `json:"on,omitempty"`
	With           *models.BasicConfig `json:"with,omitempty"`
	Do             *model.TaskList     `json:"do,omitempty"`
}

func (f *ThandTask) GetBase() *model.TaskBase {
	return &f.TaskBase
}

// Validate performs type-specific validation of the ThandTask based on the Thand type.
// This validates that On/With fields contain the required configuration for each task type.
func (t *ThandTask) Validate() error {
	switch t.Thand {
	case ThandTypeApprovals:
		return t.validateApprovalsTask()
	case ThandTypeNotify:
		return t.validateNotifyTask()
	case ThandTypeAuthorize:
		return t.validateAuthorizeTask()
	case ThandTypeForm:
		return t.validateFormTask()
	case ThandTypeValidate, ThandTypeRevoke, ThandTypeMonitor:
		// These task types have minimal or no required With configuration
		return nil
	case ThandTypeAgent:
		return t.validateAgentTask()
	default:
		return fmt.Errorf("unknown thand task type: %s", t.Thand)
	}
}

// validateApprovalsTask validates the approvals task configuration
func (t *ThandTask) validateApprovalsTask() error {
	// With field is optional for approvals (has defaults)
	// If On is provided, it should have approved/denied keys for routing
	if t.On != nil {
		if !t.On.HasString("approved") && !t.On.HasString("denied") {
			return fmt.Errorf("approvals task 'on' field should contain 'approved' and/or 'denied' routing")
		}
	}
	hasTimeout := false
	if t.With != nil {
		hasTimeout = t.With.HasString("timeout")
	}
	hasTimeoutBranch := t.On != nil && t.On.HasString("timeout")
	if hasTimeout != hasTimeoutBranch {
		return fmt.Errorf("approvals task requires both 'with.timeout' and 'on.timeout' when either is configured")
	}
	return nil
}

// validateNotifyTask validates the notify task configuration
func (t *ThandTask) validateNotifyTask() error {
	if t.With == nil {
		return fmt.Errorf("notify task requires 'with' field")
	}
	if !t.With.HasString("provider") {
		return fmt.Errorf("notify task requires 'with.provider' field")
	}
	// 'to' is required but can be string or array, just check it exists
	if _, hasTo := (*t.With)["to"]; !hasTo {
		return fmt.Errorf("notify task requires 'with.to' field")
	}
	return nil
}

// validateAuthorizeTask validates the authorize task configuration
func (t *ThandTask) validateAuthorizeTask() error {
	// Authorize task doesn't require With, but if present can have revocation and notifiers
	return nil
}

// validateFormTask validates the form task configuration
func (t *ThandTask) validateFormTask() error {
	if t.With == nil {
		return fmt.Errorf("form task requires 'with' field")
	}
	// Form requires notifiers for where to send the form
	if _, hasNotifiers := (*t.With)["notifiers"]; !hasNotifiers {
		return fmt.Errorf("form task requires 'with.notifiers' field")
	}
	return nil
}

// validateAgentTask validates the agent task configuration
func (t *ThandTask) validateAgentTask() error {
	if t.With == nil {
		return fmt.Errorf("agent task requires 'with' field")
	}
	if _, hasIdentities := (*t.With)["identities"]; !hasIdentities {
		return fmt.Errorf("agent task requires 'with.identities' field")
	}
	if t.Do == nil || len(*t.Do) == 0 {
		return fmt.Errorf("agent task requires 'do' field with at least one sub-task")
	}
	return nil
}

// registerThandValidators registers custom validators for ThandTask with the given validator instance.
func registerThandValidators(v *validator.Validate) error {
	// Register field-level validator for thand type
	if err := v.RegisterValidation("thand_type", validateThandType); err != nil {
		return fmt.Errorf("failed to register thand_type validator: %w", err)
	}

	// Register struct-level validator for ThandTask
	v.RegisterStructValidation(validateThandTaskStruct, ThandTask{})

	return nil
}

// validateThandType is a field-level validator that checks if the thand type is valid
func validateThandType(fl validator.FieldLevel) bool {
	value := fl.Field().String()
	for _, validType := range ValidThandTypes {
		if value == validType {
			return true
		}
	}
	return false
}

// validateThandTaskStruct is a struct-level validator for ThandTask
// that performs type-specific validation of On/With fields
func validateThandTaskStruct(sl validator.StructLevel) {
	task := sl.Current().Interface().(ThandTask)

	if err := task.Validate(); err != nil {
		sl.ReportError(task.With, "With", "with", "thand_config", err.Error())
	}
}

func init() {
	// Register ThandTask validators with the common validator singleton
	common.RegisterCustomValidator(registerThandValidators)

	// Register with serverlessworkflows SDK
	err := model.RegisterTask(ThandTaskName, model.TaskConstructor(func() model.Task {
		return &ThandTask{} // Create a new instance for each task
	}))

	if err != nil {
		panic("failed to register task type with serverlessworkflow SDK: " + err.Error())
	}
}
