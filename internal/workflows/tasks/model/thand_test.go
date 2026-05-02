package model

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

func TestThandTask_Validate(t *testing.T) {
	tests := []struct {
		name    string
		task    ThandTask
		wantErr bool
		errMsg  string
	}{
		{
			name: "valid approvals task without on field",
			task: ThandTask{
				Thand: ThandTypeApprovals,
				With: &models.BasicConfig{
					"approvals": 2,
				},
			},
			wantErr: false,
		},
		{
			name: "valid approvals task with on field",
			task: ThandTask{
				Thand: ThandTypeApprovals,
				On: &models.BasicConfig{
					"approved": "authorize",
					"denied":   "notify_denied",
				},
				With: &models.BasicConfig{
					"approvals": 2,
				},
			},
			wantErr: false,
		},
		{
			name: "valid approvals task with timeout",
			task: ThandTask{
				Thand: ThandTypeApprovals,
				On: &models.BasicConfig{
					"approved": "authorize",
					"denied":   "notify_denied",
					"timeout":  "timeout_handler",
				},
				With: &models.BasicConfig{
					"approvals": 2,
					"timeout":   "15m",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid approvals task with invalid on field",
			task: ThandTask{
				Thand: ThandTypeApprovals,
				On: &models.BasicConfig{
					"invalid_key": "some_value",
				},
			},
			wantErr: true,
			errMsg:  "approvals task 'on' field should contain 'approved' and/or 'denied' routing",
		},
		{
			name: "invalid approvals task with timeout but no timeout branch",
			task: ThandTask{
				Thand: ThandTypeApprovals,
				On: &models.BasicConfig{
					"approved": "authorize",
					"denied":   "notify_denied",
				},
				With: &models.BasicConfig{
					"approvals": 2,
					"timeout":   "15m",
				},
			},
			wantErr: true,
			errMsg:  "approvals task requires both 'with.timeout' and 'on.timeout' when either is configured",
		},
		{
			name: "invalid approvals task with timeout branch but no timeout",
			task: ThandTask{
				Thand: ThandTypeApprovals,
				On: &models.BasicConfig{
					"approved": "authorize",
					"denied":   "notify_denied",
					"timeout":  "timeout_handler",
				},
				With: &models.BasicConfig{
					"approvals": 2,
				},
			},
			wantErr: true,
			errMsg:  "approvals task requires both 'with.timeout' and 'on.timeout' when either is configured",
		},
		{
			name: "invalid approvals task with legacy local presence",
			task: ThandTask{
				Thand: ThandTypeApprovals,
				On: &models.BasicConfig{
					"approved": "authorize",
					"denied":   "notify_denied",
				},
				With: &models.BasicConfig{
					"approvals":      1,
					"local_presence": map[string]any{"provider": "local-presence"},
				},
			},
			wantErr: true,
			errMsg:  "with.local_presence",
		},
		{
			name: "valid notify task",
			task: ThandTask{
				Thand: ThandTypeNotify,
				With: &models.BasicConfig{
					"provider": "slack",
					"to":       "#general",
				},
			},
			wantErr: false,
		},
		{
			name: "invalid notify task missing with",
			task: ThandTask{
				Thand: ThandTypeNotify,
			},
			wantErr: true,
			errMsg:  "notify task requires 'with' field",
		},
		{
			name: "invalid notify task missing provider",
			task: ThandTask{
				Thand: ThandTypeNotify,
				With: &models.BasicConfig{
					"to": "#general",
				},
			},
			wantErr: true,
			errMsg:  "notify task requires 'with.provider' field",
		},
		{
			name: "invalid notify task missing to",
			task: ThandTask{
				Thand: ThandTypeNotify,
				With: &models.BasicConfig{
					"provider": "slack",
				},
			},
			wantErr: true,
			errMsg:  "notify task requires 'with.to' field",
		},
		{
			name: "valid form task",
			task: ThandTask{
				Thand: ThandTypeForm,
				With: &models.BasicConfig{
					"title": "Access Request",
					"notifiers": map[string]any{
						"slack": map[string]any{
							"provider": "slack",
							"to":       "#approvals",
						},
					},
				},
			},
			wantErr: false,
		},
		{
			name: "invalid form task missing with",
			task: ThandTask{
				Thand: ThandTypeForm,
			},
			wantErr: true,
			errMsg:  "form task requires 'with' field",
		},
		{
			name: "invalid form task missing notifiers",
			task: ThandTask{
				Thand: ThandTypeForm,
				With: &models.BasicConfig{
					"title": "Access Request",
				},
			},
			wantErr: true,
			errMsg:  "form task requires 'with.notifiers' field",
		},
		{
			name: "valid authorize task",
			task: ThandTask{
				Thand: ThandTypeAuthorize,
				With: &models.BasicConfig{
					"revocation": "revoke_access",
				},
			},
			wantErr: false,
		},
		{
			name: "valid authorize task without with",
			task: ThandTask{
				Thand: ThandTypeAuthorize,
			},
			wantErr: false,
		},
		{
			name: "valid validate task",
			task: ThandTask{
				Thand: ThandTypeValidate,
			},
			wantErr: false,
		},
		{
			name: "valid revoke task",
			task: ThandTask{
				Thand: ThandTypeRevoke,
			},
			wantErr: false,
		},
		{
			name: "valid monitor task",
			task: ThandTask{
				Thand: ThandTypeMonitor,
				With: &models.BasicConfig{
					"mode":      "alert",
					"threshold": 5,
				},
			},
			wantErr: false,
		},
		{
			name: "invalid unknown task type",
			task: ThandTask{
				Thand: "unknown_type",
			},
			wantErr: true,
			errMsg:  "unknown thand task type: unknown_type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.task.Validate()
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errMsg)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestValidateThandType(t *testing.T) {
	tests := []struct {
		name      string
		thandType string
		valid     bool
	}{
		{"approvals is valid", ThandTypeApprovals, true},
		{"validate is valid", ThandTypeValidate, true},
		{"authorize is valid", ThandTypeAuthorize, true},
		{"notify is valid", ThandTypeNotify, true},
		{"revoke is valid", ThandTypeRevoke, true},
		{"monitor is valid", ThandTypeMonitor, true},
		{"form is valid", ThandTypeForm, true},
		{"unknown is invalid", "unknown", false},
		{"empty is invalid", "", false},
		{"APPROVALS (uppercase) is invalid", "APPROVALS", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			valid := false
			for _, validType := range ValidThandTypes {
				if tt.thandType == validType {
					valid = true
					break
				}
			}
			assert.Equal(t, tt.valid, valid)
		})
	}
}

func TestThandTask_GoValidatorIntegration(t *testing.T) {
	v := common.GetValidator()

	tests := []struct {
		name    string
		task    ThandTask
		wantErr bool
	}{
		{
			name: "valid task passes struct validation",
			task: ThandTask{
				Thand: ThandTypeApprovals,
			},
			wantErr: false,
		},
		{
			name: "missing thand field fails validation",
			task: ThandTask{
				Thand: "",
			},
			wantErr: true,
		},
		{
			name: "invalid thand type fails validation",
			task: ThandTask{
				Thand: "invalid_type",
			},
			wantErr: true,
		},
		{
			name: "notify without required fields fails struct validation",
			task: ThandTask{
				Thand: ThandTypeNotify,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := v.Struct(&tt.task)
			if tt.wantErr {
				require.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}
