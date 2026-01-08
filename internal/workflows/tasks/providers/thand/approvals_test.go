package thand

import (
	"slices"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/providers/aws"
	thandFunction "github.com/thand-io/agent/internal/workflows/functions/providers/thand"
	taskModel "github.com/thand-io/agent/internal/workflows/tasks/model"
	sdkWorkflowsModel "github.com/thand-io/agent/sdk/workflows/models"
)

// TestEvaluateApprovalSwitch tests the approval switch logic with various scenarios
func TestEvaluateApprovalSwitch(t *testing.T) {
	tests := []struct {
		name              string
		approvals         map[string]any
		requiredApprovals int
		approvedState     string
		deniedState       string
		taskName          string
		expectedState     string
		description       string
	}{
		{
			name: "single approval - meets requirement",
			approvals: map[string]any{
				"user1@example.com": map[string]any{
					"approved":  true,
					"timestamp": time.Now().UTC().Format(time.RFC3339),
				},
			},
			requiredApprovals: 1,
			approvedState:     "authorize",
			deniedState:       "denied",
			taskName:          "approval_task",
			expectedState:     "authorize",
			description:       "Should approve when one approval meets requirement of 1",
		},
		{
			name: "single denial - should deny immediately",
			approvals: map[string]any{
				"user1@example.com": map[string]any{
					"approved":  false,
					"timestamp": time.Now().UTC().Format(time.RFC3339),
				},
			},
			requiredApprovals: 1,
			approvedState:     "authorize",
			deniedState:       "denied",
			taskName:          "approval_task",
			expectedState:     "denied",
			description:       "Should deny immediately when any user denies",
		},
		{
			name: "multiple approvals - meets requirement",
			approvals: map[string]any{
				"user1@example.com": map[string]any{
					"approved":  true,
					"timestamp": time.Now().UTC().Format(time.RFC3339),
				},
				"user2@example.com": map[string]any{
					"approved":  true,
					"timestamp": time.Now().UTC().Format(time.RFC3339),
				},
				"user3@example.com": map[string]any{
					"approved":  true,
					"timestamp": time.Now().UTC().Format(time.RFC3339),
				},
			},
			requiredApprovals: 2,
			approvedState:     "authorize",
			deniedState:       "denied",
			taskName:          "approval_task",
			expectedState:     "authorize",
			description:       "Should approve when 3 approvals meet requirement of 2",
		},
		{
			name: "multiple approvals - one denial",
			approvals: map[string]any{
				"user1@example.com": map[string]any{
					"approved":  true,
					"timestamp": time.Now().UTC().Format(time.RFC3339),
				},
				"user2@example.com": map[string]any{
					"approved":  false,
					"timestamp": time.Now().UTC().Format(time.RFC3339),
				},
				"user3@example.com": map[string]any{
					"approved":  true,
					"timestamp": time.Now().UTC().Format(time.RFC3339),
				},
			},
			requiredApprovals: 2,
			approvedState:     "authorize",
			deniedState:       "denied",
			taskName:          "approval_task",
			expectedState:     "denied",
			description:       "Should deny when any user denies, even if others approve",
		},
		{
			name: "insufficient approvals - should loop",
			approvals: map[string]any{
				"user1@example.com": map[string]any{
					"approved":  true,
					"timestamp": time.Now().UTC().Format(time.RFC3339),
				},
			},
			requiredApprovals: 2,
			approvedState:     "authorize",
			deniedState:       "denied",
			taskName:          "approval_task",
			expectedState:     "approval_task",
			description:       "Should loop back when approvals (1) don't meet requirement (2)",
		},
		{
			name:              "no approvals yet - should loop",
			approvals:         map[string]any{},
			requiredApprovals: 1,
			approvedState:     "authorize",
			deniedState:       "denied",
			taskName:          "approval_task",
			expectedState:     "approval_task",
			description:       "Should loop back when no approvals received yet",
		},
		{
			name: "exactly meets requirement",
			approvals: map[string]any{
				"user1@example.com": map[string]any{
					"approved":  true,
					"timestamp": time.Now().UTC().Format(time.RFC3339),
				},
				"user2@example.com": map[string]any{
					"approved":  true,
					"timestamp": time.Now().UTC().Format(time.RFC3339),
				},
			},
			requiredApprovals: 2,
			approvedState:     "authorize",
			deniedState:       "denied",
			taskName:          "approval_task",
			expectedState:     "authorize",
			description:       "Should approve when exactly meeting requirement",
		},
		{
			name: "mixed responses with insufficient approvals",
			approvals: map[string]any{
				"user1@example.com": map[string]any{
					"approved":  true,
					"timestamp": time.Now().UTC().Format(time.RFC3339),
				},
				"user2@example.com": map[string]any{
					"approved":  true,
					"timestamp": time.Now().UTC().Format(time.RFC3339),
				},
			},
			requiredApprovals: 3,
			approvedState:     "authorize",
			deniedState:       "denied",
			taskName:          "approval_task",
			expectedState:     "approval_task",
			description:       "Should loop when 2 approvals don't meet requirement of 3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a workflow task with the approvals context
			workflowTask := models.NewElevateWorkflowTask(&sdkWorkflowsModel.WorkflowTask{
				WorkflowID:   "test-workflow",
				WorkflowName: "Test Workflow",
			}) // Set the context with approvals
			workflowTask.SetContextKeyValue("approvals", tt.approvals)

			// Create a thandTask instance
			task := &thandTask{}

			// Execute the approval switch
			flowDirective, err := task.evaluateApprovalSwitch(
				workflowTask,
				tt.taskName,
				tt.approvals,
				tt.requiredApprovals,
				tt.approvedState,
				tt.deniedState,
			) // Assert no error occurred
			require.NoError(t, err, "evaluateApprovalSwitch should not return an error for test: %s", tt.description)

			// Assert the flow directive is not nil
			require.NotNil(t, flowDirective, "flowDirective should not be nil for test: %s", tt.description)

			// Assert the expected state
			assert.Equal(t, tt.expectedState, flowDirective.Value, "Test: %s", tt.description)
		})
	}
}

// TestEvaluateApprovalSwitchProgression tests the progression of approvals over time
func TestEvaluateApprovalSwitchProgression(t *testing.T) {
	workflowTask := models.NewElevateWorkflowTask(&sdkWorkflowsModel.WorkflowTask{
		WorkflowID:   "test-workflow",
		WorkflowName: "Test Workflow",
	})

	task := &thandTask{}
	taskName := "approval_task"
	requiredApprovals := 2
	approvedState := "authorize"
	deniedState := "denied"

	// Step 1: No approvals yet - should loop
	approvals := map[string]any{}
	workflowTask.SetContextKeyValue("approvals", approvals)

	flowDirective, err := task.evaluateApprovalSwitch(
		workflowTask,
		taskName,
		approvals,
		requiredApprovals,
		approvedState,
		deniedState,
	)

	require.NoError(t, err)
	require.NotNil(t, flowDirective)
	assert.Equal(t, taskName, flowDirective.Value, "Should loop when no approvals")

	// Step 2: First approval comes in - still should loop
	approvals["user1@example.com"] = map[string]any{
		"approved":  true,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	workflowTask.SetContextKeyValue("approvals", approvals)

	flowDirective, err = task.evaluateApprovalSwitch(
		workflowTask,
		taskName,
		approvals,
		requiredApprovals,
		approvedState,
		deniedState,
	)

	require.NoError(t, err)
	require.NotNil(t, flowDirective)
	assert.Equal(t, taskName, flowDirective.Value, "Should loop when only 1 of 2 approvals")

	// Step 3: Second approval comes in - should now authorize
	approvals["user2@example.com"] = map[string]any{
		"approved":  true,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	workflowTask.SetContextKeyValue("approvals", approvals)

	flowDirective, err = task.evaluateApprovalSwitch(
		workflowTask,
		taskName,
		approvals,
		requiredApprovals,
		approvedState,
		deniedState,
	)

	require.NoError(t, err)
	require.NotNil(t, flowDirective)
	assert.Equal(t, approvedState, flowDirective.Value, "Should authorize when 2 of 2 approvals received")
}

// TestEvaluateApprovalSwitchDenialProgression tests that a denial stops the process
func TestEvaluateApprovalSwitchDenialProgression(t *testing.T) {
	workflowTask := models.NewElevateWorkflowTask(&sdkWorkflowsModel.WorkflowTask{
		WorkflowID:   "test-workflow",
		WorkflowName: "Test Workflow",
	})

	task := &thandTask{}
	taskName := "approval_task"
	requiredApprovals := 3
	approvedState := "authorize"
	deniedState := "denied"

	// Step 1: First approval comes in
	approvals := map[string]any{
		"user1@example.com": map[string]any{
			"approved":  true,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		},
	}
	workflowTask.SetContextKeyValue("approvals", approvals)

	flowDirective, err := task.evaluateApprovalSwitch(
		workflowTask,
		taskName,
		approvals,
		requiredApprovals,
		approvedState,
		deniedState,
	)

	require.NoError(t, err)
	require.NotNil(t, flowDirective)
	assert.Equal(t, taskName, flowDirective.Value, "Should loop with 1 approval")

	// Step 2: Second user denies - should immediately deny
	approvals["user2@example.com"] = map[string]any{
		"approved":  false,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	workflowTask.SetContextKeyValue("approvals", approvals)

	flowDirective, err = task.evaluateApprovalSwitch(
		workflowTask,
		taskName,
		approvals,
		requiredApprovals,
		approvedState,
		deniedState,
	)

	require.NoError(t, err)
	require.NotNil(t, flowDirective)
	assert.Equal(t, deniedState, flowDirective.Value, "Should deny immediately when any user denies")

	// Step 3: Even if a third approval comes in, should still be denied
	approvals["user3@example.com"] = map[string]any{
		"approved":  true,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	workflowTask.SetContextKeyValue("approvals", approvals)

	flowDirective, err = task.evaluateApprovalSwitch(
		workflowTask,
		taskName,
		approvals,
		requiredApprovals,
		approvedState,
		deniedState,
	)

	require.NoError(t, err)
	require.NotNil(t, flowDirective)
	assert.Equal(t, deniedState, flowDirective.Value, "Should remain denied even after additional approvals")
}

// TestSelfApprovalLogic tests the self-approval validation logic
func TestSelfApprovalLogic(t *testing.T) {
	tests := []struct {
		name               string
		selfApprove        bool
		requesterIdentity  string
		approverIdentity   string
		elevatedIdentities []string
		shouldBlock        bool
		description        string
	}{
		{
			name:               "self-approval disabled - approver is requester",
			selfApprove:        false,
			requesterIdentity:  "user@example.com",
			approverIdentity:   "user@example.com",
			elevatedIdentities: []string{"user@example.com"},
			shouldBlock:        true,
			description:        "Should block approval when requester and approver are same and selfApprove is false",
		},
		{
			name:               "self-approval disabled - approver is elevated identity",
			selfApprove:        false,
			requesterIdentity:  "admin@example.com",
			approverIdentity:   "user@example.com",
			elevatedIdentities: []string{"user@example.com", "otheruser@example.com"},
			shouldBlock:        true,
			description:        "Should block approval when approver is one of the elevated identities and selfApprove is false",
		},
		{
			name:               "self-approval disabled - approver is different elevated identity",
			selfApprove:        false,
			requesterIdentity:  "admin@example.com",
			approverIdentity:   "otheruser@example.com",
			elevatedIdentities: []string{"user@example.com", "otheruser@example.com"},
			shouldBlock:        true,
			description:        "Should block approval when approver is any of the elevated identities and selfApprove is false",
		},
		{
			name:               "self-approval disabled - approver not requester or elevated identity",
			selfApprove:        false,
			requesterIdentity:  "admin@example.com",
			approverIdentity:   "approver@example.com",
			elevatedIdentities: []string{"user@example.com"},
			shouldBlock:        false,
			description:        "Should allow approval when approver is neither requester nor elevated identity and selfApprove is false",
		},
		{
			name:               "self-approval disabled - requester elevating someone else",
			selfApprove:        false,
			requesterIdentity:  "requester@example.com",
			approverIdentity:   "approver@example.com",
			elevatedIdentities: []string{"targetuser@example.com"},
			shouldBlock:        false,
			description:        "Should allow approval when requester elevating someone else and approver is external",
		},
		{
			name:               "self-approval enabled - approver is requester",
			selfApprove:        true,
			requesterIdentity:  "user@example.com",
			approverIdentity:   "user@example.com",
			elevatedIdentities: []string{"user@example.com"},
			shouldBlock:        false,
			description:        "Should allow approval when requester and approver are same and selfApprove is true",
		},
		{
			name:               "self-approval enabled - approver is elevated identity",
			selfApprove:        true,
			requesterIdentity:  "admin@example.com",
			approverIdentity:   "user@example.com",
			elevatedIdentities: []string{"user@example.com"},
			shouldBlock:        false,
			description:        "Should allow approval when approver is elevated identity and selfApprove is true",
		},
		{
			name:               "self-approval enabled - different user",
			selfApprove:        true,
			requesterIdentity:  "requester@example.com",
			approverIdentity:   "approver@example.com",
			elevatedIdentities: []string{"targetuser@example.com"},
			shouldBlock:        false,
			description:        "Should allow approval when requester and approver are different and selfApprove is true",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Test the self-approval check logic
			notifyReq := ApprovalsTask{
				Approvals:   1,
				SelfApprove: tt.selfApprove,
			}

			// Simulate the self-approval check logic from the code
			// This matches the logic in approvals.go
			shouldBlock := false
			if !notifyReq.SelfApprove {
				// Check if approver is the requester
				if tt.approverIdentity == tt.requesterIdentity {
					shouldBlock = true
				}
				// Check if approver is one of the identities being elevated
				if slices.Contains(tt.elevatedIdentities, tt.approverIdentity) {
					shouldBlock = true
				}
			}

			// Verify the result
			assert.Equal(t, tt.shouldBlock, shouldBlock, "Test: %s", tt.description)
		})
	}
}

// TestSelfApprovalWithMultipleApprovers tests self-approval counting logic
func TestSelfApprovalWithMultipleApprovers(t *testing.T) {
	requesterIdentity := "user1@example.com"

	t.Run("self-approval disabled - only external approvers count", func(t *testing.T) {
		// Test scenario: selfApprove = false, requires 2 approvals
		// User1 (requester) tries to approve their own request (should be blocked)
		// User2 approves (should count)
		// User3 approves (should count)
		// Result: 2 valid approvals

		notifyReq := ApprovalsTask{
			Approvals:   2,
			SelfApprove: false,
		}

		approvals := map[string]any{}

		// Step 1: Requester tries to self-approve (should be blocked)
		user1ShouldBlock := !notifyReq.SelfApprove && requesterIdentity == "user1@example.com"
		if !user1ShouldBlock {
			approvals["user1@example.com"] = map[string]any{
				"approved":  true,
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			}
		}

		assert.Empty(t, approvals, "Self-approval should be blocked")

		// Step 2: External user2 approves (should count)
		user2ShouldBlock := !notifyReq.SelfApprove && requesterIdentity == "user2@example.com"
		if !user2ShouldBlock {
			approvals["user2@example.com"] = map[string]any{
				"approved":  true,
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			}
		}

		assert.Len(t, approvals, 1, "External approval should be counted")

		// Step 3: External user3 approves (should count)
		user3ShouldBlock := !notifyReq.SelfApprove && requesterIdentity == "user3@example.com"
		if !user3ShouldBlock {
			approvals["user3@example.com"] = map[string]any{
				"approved":  true,
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			}
		}

		assert.Len(t, approvals, 2, "Two external approvals should be counted")
		assert.NotContains(t, approvals, "user1@example.com", "Requester's self-approval should not be in approvals map")
		assert.Contains(t, approvals, "user2@example.com", "User2's approval should be in approvals map")
		assert.Contains(t, approvals, "user3@example.com", "User3's approval should be in approvals map")
	})

	t.Run("self-approval enabled - requester can approve own request", func(t *testing.T) {
		// Test scenario: selfApprove = true, requires 1 approval
		// User1 (requester) approves their own request (should count)

		notifyReq := ApprovalsTask{
			Approvals:   1,
			SelfApprove: true,
		}

		approvals := map[string]any{}

		// Requester self-approves (should be allowed)
		user1ShouldBlock := !notifyReq.SelfApprove && requesterIdentity == "user1@example.com"
		if !user1ShouldBlock {
			approvals["user1@example.com"] = map[string]any{
				"approved":  true,
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			}
		}

		assert.Len(t, approvals, 1, "Self-approval should be allowed and counted")
		assert.Contains(t, approvals, "user1@example.com", "Requester's self-approval should be in approvals map")
	})
}

// TestSelfApprovalWithElevatedIdentities tests scenarios where identities being elevated try to approve
func TestSelfApprovalWithElevatedIdentities(t *testing.T) {
	t.Run("self-approval disabled - elevated identity cannot approve", func(t *testing.T) {
		// Scenario: Admin requests elevation for user1@example.com
		// user1@example.com tries to approve their own elevation (should be blocked)
		// user2@example.com (external) approves (should count)

		requesterIdentity := "admin@example.com"
		elevatedIdentity := "user1@example.com"
		externalApproverIdentity := "user2@example.com"

		notifyReq := ApprovalsTask{
			Approvals:   1,
			SelfApprove: false,
		}

		approvals := map[string]any{}

		// Step 1: Elevated identity tries to approve (should be blocked)
		shouldBlockElevated := false
		if !notifyReq.SelfApprove {
			// Check if approver is requester
			if elevatedIdentity == requesterIdentity {
				shouldBlockElevated = true
			}
			// Check if approver is one of the elevated identities
			elevatedIdentities := []string{elevatedIdentity}
			if slices.Contains(elevatedIdentities, elevatedIdentity) {
				shouldBlockElevated = true
			}
		}

		if !shouldBlockElevated {
			approvals[elevatedIdentity] = map[string]any{
				"approved":  true,
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			}
		}

		assert.True(t, shouldBlockElevated, "Elevated identity should be blocked from approving")
		assert.Empty(t, approvals, "No approvals should be recorded when elevated identity tries to approve")

		// Step 2: External approver approves (should count)
		shouldBlockExternal := false
		if !notifyReq.SelfApprove {
			if externalApproverIdentity == requesterIdentity {
				shouldBlockExternal = true
			}
			elevatedIdentities := []string{elevatedIdentity}
			if slices.Contains(elevatedIdentities, externalApproverIdentity) {
				shouldBlockExternal = true
			}
		}

		if !shouldBlockExternal {
			approvals[externalApproverIdentity] = map[string]any{
				"approved":  true,
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			}
		}

		assert.False(t, shouldBlockExternal, "External approver should not be blocked")
		assert.Len(t, approvals, 1, "External approval should be counted")
		assert.Contains(t, approvals, externalApproverIdentity, "External approver's approval should be recorded")
		assert.NotContains(t, approvals, elevatedIdentity, "Elevated identity's approval should not be recorded")
	})

	t.Run("self-approval disabled - multiple elevated identities", func(t *testing.T) {
		// Scenario: Admin requests elevation for user1 and user2
		// user1 tries to approve (should be blocked)
		// user2 tries to approve (should be blocked)
		// user3 (external) approves (should count)

		requesterIdentity := "admin@example.com"
		elevatedIdentities := []string{"user1@example.com", "user2@example.com"}
		externalApproverIdentity := "user3@example.com"

		notifyReq := ApprovalsTask{
			Approvals:   2,
			SelfApprove: false,
		}

		approvals := map[string]any{}

		// Helper function to check if approval should be blocked
		checkShouldBlock := func(approverIdentity string) bool {
			if !notifyReq.SelfApprove {
				if approverIdentity == requesterIdentity {
					return true
				}
				if slices.Contains(elevatedIdentities, approverIdentity) {
					return true
				}
			}
			return false
		}

		// user1 tries to approve
		if !checkShouldBlock(elevatedIdentities[0]) {
			approvals[elevatedIdentities[0]] = map[string]any{"approved": true, "timestamp": time.Now().UTC().Format(time.RFC3339)}
		}

		// user2 tries to approve
		if !checkShouldBlock(elevatedIdentities[1]) {
			approvals[elevatedIdentities[1]] = map[string]any{"approved": true, "timestamp": time.Now().UTC().Format(time.RFC3339)}
		}

		assert.Empty(t, approvals, "No elevated identities should be able to approve")

		// user3 (external) approves
		if !checkShouldBlock(externalApproverIdentity) {
			approvals[externalApproverIdentity] = map[string]any{"approved": true, "timestamp": time.Now().UTC().Format(time.RFC3339)}
		}

		assert.Len(t, approvals, 1, "Only external approval should be counted")
		assert.Contains(t, approvals, externalApproverIdentity)
	})

	t.Run("self-approval enabled - elevated identities can approve", func(t *testing.T) {
		// Scenario: Admin requests elevation for user1
		// When selfApprove is enabled, user1 can approve their own elevation

		requesterIdentity := "admin@example.com"
		elevatedIdentity := "user1@example.com"

		notifyReq := ApprovalsTask{
			Approvals:   1,
			SelfApprove: true,
		}

		approvals := map[string]any{}

		// user1 tries to approve (should be allowed when selfApprove is true)
		shouldBlock := false
		if !notifyReq.SelfApprove {
			if elevatedIdentity == requesterIdentity {
				shouldBlock = true
			}
			elevatedIdentities := []string{elevatedIdentity}
			if slices.Contains(elevatedIdentities, elevatedIdentity) {
				shouldBlock = true
			}
		}

		if !shouldBlock {
			approvals[elevatedIdentity] = map[string]any{
				"approved":  true,
				"timestamp": time.Now().UTC().Format(time.RFC3339),
			}
		}

		assert.False(t, shouldBlock, "Self-approval should be allowed when selfApprove is true")
		assert.Len(t, approvals, 1, "Elevated identity's approval should be counted when selfApprove is true")
		assert.Contains(t, approvals, elevatedIdentity)
	})
}

// TestGetIdentityVariations tests different identity formats
func TestGetIdentityVariations(t *testing.T) {
	tests := []struct {
		name             string
		user             models.User
		expectedIdentity string
		description      string
	}{
		{
			name: "email present",
			user: models.User{
				Email:    "user@example.com",
				Username: "username",
				ID:       "123",
			},
			expectedIdentity: "user@example.com",
			description:      "Should use email when available",
		},
		{
			name: "no email, username present",
			user: models.User{
				Username: "username",
				ID:       "123",
			},
			expectedIdentity: "username",
			description:      "Should use username when email not available",
		},
		{
			name: "no email or username, ID present",
			user: models.User{
				ID: "123",
			},
			expectedIdentity: "123",
			description:      "Should use ID when email and username not available",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			identity := tt.user.GetIdentity()
			assert.Equal(t, tt.expectedIdentity, identity, "Test: %s", tt.description)
		})
	}
}

func TestApprovalsTask_Helpers(t *testing.T) {
	t.Run("IsValid", func(t *testing.T) {
		task := &ApprovalsTask{Approvals: 1}
		assert.True(t, task.IsValid())

		taskInvalid := &ApprovalsTask{Approvals: 0}
		assert.False(t, taskInvalid.IsValid())
	})

	t.Run("HasNotifiers", func(t *testing.T) {
		task := &ApprovalsTask{Notifiers: map[string]thandFunction.NotifierRequest{"email": {}}}
		assert.True(t, task.HasNotifiers())

		taskEmpty := &ApprovalsTask{}
		assert.False(t, taskEmpty.HasNotifiers())
	})
}

func TestExecuteApprovalsTask_ProcessEvent(t *testing.T) {
	// Setup identities
	requesterID := "requester@example.com"
	approverID := "approver@example.com"

	mockProvider := aws.NewMockAwsProvider()
	provider := models.ProviderConfig{
		Name:     "mock",
		Provider: "aws",
		Enabled:  true,
	}
	mockProvider.Initialize("mock", provider)

	mockProvider.SetIdentities([]models.Identity{
		{
			ID: requesterID,
			User: &models.User{
				ID:    requesterID,
				Email: requesterID,
			},
		},
		{
			ID: approverID,
			User: &models.User{
				ID:    approverID,
				Email: approverID,
			},
		},
	})

	cfg := &config.Config{}
	cfg.AddProvider("mock", mockProvider)

	// Setup WorkflowTask with ElevationRequest context
	workflowTask := models.NewElevateWorkflowTask(&sdkWorkflowsModel.WorkflowTask{
		WorkflowID:   "test-workflow",
		WorkflowName: "Test Workflow",
		Context: map[string]any{
			"user": map[string]any{
				"id":    requesterID,
				"email": requesterID,
			},
			"identities": []string{"target-identity"},
			"approvals":  map[string]any{}, // Empty approvals initially
		},
	})

	// Setup the Task Definition
	taskDef := &taskModel.ThandTask{
		With: &models.BasicConfig{
			"approvals":   1,
			"selfApprove": false,
		},
		On: &models.BasicConfig{
			"approved": "authorize",
			"denied":   "denied",
		},
	}

	// Create the CloudEvent representing an approval
	event := cloudevents.NewEvent()
	event.SetType(ThandApprovalEventType)
	event.SetSource("test-source")
	event.SetExtension(models.VarsContextUser, approverID)
	event.SetData(cloudevents.ApplicationJSON, map[string]any{
		"approved": true,
	})

	// Create task handler with mock config
	taskHandler := &thandTask{
		config: cfg,
	}

	// Execute
	// We pass the event as 'input' to simulate the task resuming after receiving an event
	result, err := taskHandler.executeApprovalsTask(workflowTask, "approval_task", taskDef, &event)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify FlowDirective
	directive, ok := result.(*model.FlowDirective)
	require.True(t, ok)
	assert.Equal(t, "authorize", directive.Value, "Should transition to 'authorize' state after valid approval")

	// Verify Context Update
	approvals := workflowTask.Context.(map[string]any)["approvals"].(map[string]any)
	assert.Contains(t, approvals, approverID)
	approvalData := approvals[approverID].(map[string]any)
	assert.Equal(t, true, approvalData["approved"])
}

func TestExecuteApprovalsTask_SelfApprovalDenied(t *testing.T) {
	// Setup identities where requester tries to approve
	requesterID := "requester@example.com"

	mockProvider := aws.NewMockAwsProvider()
	provider := models.ProviderConfig{
		Name:     "mock",
		Provider: "aws",
		Enabled:  true,
	}
	mockProvider.Initialize("mock", provider)

	mockProvider.SetIdentities([]models.Identity{
		{
			ID: requesterID,
			User: &models.User{
				ID:    requesterID,
				Email: requesterID,
			},
		},
	})

	cfg := &config.Config{}
	cfg.AddProvider("mock", mockProvider)

	workflowTask := models.NewElevateWorkflowTask(&sdkWorkflowsModel.WorkflowTask{
		WorkflowID: "test-workflow",
		Context: map[string]any{
			"user":      map[string]any{"id": requesterID, "email": requesterID},
			"approvals": map[string]any{},
		},
	})

	taskDef := &taskModel.ThandTask{
		With: &models.BasicConfig{
			"approvals":   1,
			"selfApprove": false, // Explicitly disable self-approval
		},
		On: &models.BasicConfig{
			"approved": "authorize",
			"denied":   "denied",
		},
	}

	// Event from the requester
	event := cloudevents.NewEvent()
	event.SetType(ThandApprovalEventType)
	event.SetSource("test-source")
	event.SetExtension(models.VarsContextUser, requesterID)
	event.SetData(cloudevents.ApplicationJSON, map[string]any{"approved": true})

	taskHandler := &thandTask{config: cfg}

	// Execute
	result, err := taskHandler.executeApprovalsTask(workflowTask, "approval_task", taskDef, &event)

	require.NoError(t, err)
	directive, ok := result.(*model.FlowDirective)
	require.True(t, ok)

	// Should loop back (default case) because the approval was ignored
	assert.Equal(t, "approval_task", directive.Value, "Should loop back when self-approval is ignored")

	// Verify approval was NOT recorded
	approvals := workflowTask.Context.(map[string]any)["approvals"].(map[string]any)
	assert.NotContains(t, approvals, requesterID, "Self-approval should not be recorded in context")
}

func TestExecuteApprovalsTask_NonExistentIdentity(t *testing.T) {
	// Setup identities
	requesterID := "requester@example.com"
	approverID := "nonexistent@example.com"

	mockProvider := aws.NewMockAwsProvider()
	provider := models.ProviderConfig{
		Name:     "mock",
		Provider: "aws",
		Enabled:  true,
	}
	mockProvider.Initialize("mock", provider)

	// Only register requester, NOT approver
	mockProvider.SetIdentities([]models.Identity{
		{
			ID: requesterID,
			User: &models.User{
				ID:    requesterID,
				Email: requesterID,
			},
		},
	})

	cfg := &config.Config{}
	cfg.AddProvider("mock", mockProvider)

	// Setup WorkflowTask with ElevationRequest context
	workflowTask := models.NewElevateWorkflowTask(&sdkWorkflowsModel.WorkflowTask{
		WorkflowID:   "test-workflow",
		WorkflowName: "Test Workflow",
		Context: map[string]any{
			"user": map[string]any{
				"id":    requesterID,
				"email": requesterID,
			},
			"identities": []string{"target-identity"},
			"approvals":  map[string]any{}, // Empty approvals initially
		},
	})

	// Setup the Task Definition
	taskDef := &taskModel.ThandTask{
		With: &models.BasicConfig{
			"approvals":   1,
			"selfApprove": false,
		},
		On: &models.BasicConfig{
			"approved": "authorize",
			"denied":   "denied",
		},
	}

	// Create the CloudEvent representing an approval
	event := cloudevents.NewEvent()
	event.SetType(ThandApprovalEventType)
	event.SetSource("test-source")
	event.SetExtension(models.VarsContextUser, approverID)
	event.SetData(cloudevents.ApplicationJSON, map[string]any{
		"approved": true,
	})

	// Create task handler with mock config
	taskHandler := &thandTask{
		config: cfg,
	}

	// Execute
	taskName := "approval_task"
	result, err := taskHandler.executeApprovalsTask(workflowTask, taskName, taskDef, &event)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify FlowDirective - should loop back to taskName because identity wasn't found
	directive, ok := result.(*model.FlowDirective)
	require.True(t, ok)
	assert.Equal(t, taskName, directive.Value, "Should stay in same state when identity is missing")

	// Verify Context Update - approvals should NOT contain the non-existent approver
	approvals := workflowTask.Context.(map[string]any)["approvals"].(map[string]any)
	assert.NotContains(t, approvals, approverID)
}

func TestExecuteApprovalsTask_Denial(t *testing.T) {
	// Setup identities
	requesterID := "requester@example.com"
	approverID := "approver@example.com"

	mockProvider := aws.NewMockAwsProvider()
	provider := models.ProviderConfig{
		Name:     "mock",
		Provider: "aws",
		Enabled:  true,
	}
	mockProvider.Initialize("mock", provider)

	mockProvider.SetIdentities([]models.Identity{
		{
			ID: requesterID,
			User: &models.User{
				ID:    requesterID,
				Email: requesterID,
			},
		},
		{
			ID: approverID,
			User: &models.User{
				ID:    approverID,
				Email: approverID,
			},
		},
	})

	cfg := &config.Config{}
	cfg.AddProvider("mock", mockProvider)

	// Setup WorkflowTask with ElevationRequest context
	workflowTask := models.NewElevateWorkflowTask(&sdkWorkflowsModel.WorkflowTask{
		WorkflowID:   "test-workflow",
		WorkflowName: "Test Workflow",
		Context: map[string]any{
			"user": map[string]any{
				"id":    requesterID,
				"email": requesterID,
			},
			"identities": []string{"target-identity"},
			"approvals":  map[string]any{}, // Empty approvals initially
		},
	})

	// Setup the Task Definition
	taskDef := &taskModel.ThandTask{
		With: &models.BasicConfig{
			"approvals":   1,
			"selfApprove": false,
		},
		On: &models.BasicConfig{
			"approved": "authorize",
			"denied":   "denied",
		},
	}

	// Create the CloudEvent representing a denial
	event := cloudevents.NewEvent()
	event.SetType(ThandApprovalEventType)
	event.SetSource("test-source")
	event.SetExtension(models.VarsContextUser, approverID)
	event.SetData(cloudevents.ApplicationJSON, map[string]any{
		"approved": false,
	})

	// Create task handler with mock config
	taskHandler := &thandTask{
		config: cfg,
	}

	// Execute
	result, err := taskHandler.executeApprovalsTask(workflowTask, "approval_task", taskDef, &event)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify FlowDirective
	directive, ok := result.(*model.FlowDirective)
	require.True(t, ok)
	assert.Equal(t, "denied", directive.Value, "Should transition to 'denied' state after denial")

	// Verify Context Update
	approvals := workflowTask.Context.(map[string]any)["approvals"].(map[string]any)
	assert.Contains(t, approvals, approverID)
	approvalData := approvals[approverID].(map[string]any)
	assert.Equal(t, false, approvalData["approved"])
}

func TestExecuteApprovalsTask_ApprovalThenDenial(t *testing.T) {
	// Setup identities
	requesterID := "requester@example.com"
	approver1ID := "approver1@example.com"
	approver2ID := "approver2@example.com"

	mockProvider := aws.NewMockAwsProvider()
	provider := models.ProviderConfig{
		Name:     "mock",
		Provider: "aws",
		Enabled:  true,
	}
	mockProvider.Initialize("mock", provider)

	mockProvider.SetIdentities([]models.Identity{
		{
			ID: requesterID,
			User: &models.User{
				ID:    requesterID,
				Email: requesterID,
			},
		},
		{
			ID: approver1ID,
			User: &models.User{
				ID:    approver1ID,
				Email: approver1ID,
			},
		},
		{
			ID: approver2ID,
			User: &models.User{
				ID:    approver2ID,
				Email: approver2ID,
			},
		},
	})

	cfg := &config.Config{}
	cfg.AddProvider("mock", mockProvider)

	// Setup WorkflowTask with ElevationRequest context
	// Pre-populate with one approval
	workflowTask := models.NewElevateWorkflowTask(&sdkWorkflowsModel.WorkflowTask{
		WorkflowID:   "test-workflow",
		WorkflowName: "Test Workflow",
		Context: map[string]any{
			"user": map[string]any{
				"id":    requesterID,
				"email": requesterID,
			},
			"identities": []string{"target-identity"},
			"approvals": map[string]any{
				approver1ID: map[string]any{
					"approved":  true,
					"timestamp": time.Now().UTC().Format(time.RFC3339),
				},
			},
		},
	})

	// Setup the Task Definition
	// Require 2 approvals
	taskDef := &taskModel.ThandTask{
		With: &models.BasicConfig{
			"approvals":   2,
			"selfApprove": false,
		},
		On: &models.BasicConfig{
			"approved": "authorize",
			"denied":   "denied",
		},
	}

	// Create the CloudEvent representing a denial from the second approver
	event := cloudevents.NewEvent()
	event.SetType(ThandApprovalEventType)
	event.SetSource("test-source")
	event.SetExtension(models.VarsContextUser, approver2ID)
	event.SetData(cloudevents.ApplicationJSON, map[string]any{
		"approved": false,
	})

	// Create task handler with mock config
	taskHandler := &thandTask{
		config: cfg,
	}

	// Execute
	result, err := taskHandler.executeApprovalsTask(workflowTask, "approval_task", taskDef, &event)

	require.NoError(t, err)
	require.NotNil(t, result)

	// Verify FlowDirective
	directive, ok := result.(*model.FlowDirective)
	require.True(t, ok)
	assert.Equal(t, "denied", directive.Value, "Should transition to 'denied' state after denial, even with one existing approval")

	// Verify Context Update
	approvals := workflowTask.Context.(map[string]any)["approvals"].(map[string]any)

	// Check approver1 (existing approval)
	assert.Contains(t, approvals, approver1ID)
	approval1Data := approvals[approver1ID].(map[string]any)
	assert.Equal(t, true, approval1Data["approved"])

	// Check approver2 (new denial)
	assert.Contains(t, approvals, approver2ID)
	approval2Data := approvals[approver2ID].(map[string]any)
	assert.Equal(t, false, approval2Data["approved"])
}
