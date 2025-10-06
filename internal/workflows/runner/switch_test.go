package runner

import (
	"testing"

	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/thand-io/agent/internal/config"
	"github.com/thand-io/agent/internal/models"
	"github.com/thand-io/agent/internal/workflows/functions"
)

func TestEvaluateSwitchTask_StringMatching(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]any
		expressions []struct {
			caseName string
			when     string
			then     string
		}
		expectedResult string
	}{
		{
			name: "Match electronic order",
			input: map[string]any{
				"orderType": "electronic",
			},
			expressions: []struct {
				caseName string
				when     string
				then     string
			}{
				{"electronic", `.orderType == "electronic"`, "processElectronic"},
				{"physical", `.orderType == "physical"`, "processPhysical"},
			},
			expectedResult: "processElectronic",
		},
		{
			name: "Match physical order",
			input: map[string]any{
				"orderType": "physical",
			},
			expressions: []struct {
				caseName string
				when     string
				then     string
			}{
				{"electronic", `.orderType == "electronic"`, "processElectronic"},
				{"physical", `.orderType == "physical"`, "processPhysical"},
			},
			expectedResult: "processPhysical",
		},
		{
			name: "Match color red",
			input: map[string]any{
				"color": "red",
			},
			expressions: []struct {
				caseName string
				when     string
				then     string
			}{
				{"red", `.color == "red"`, "setRed"},
				{"blue", `.color == "blue"`, "setBlue"},
				{"green", `.color == "green"`, "setGreen"},
			},
			expectedResult: "setRed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test runner
			cfg := &config.Config{}
			functionRegistry := functions.NewFunctionRegistry(cfg)
			workflowTask := &models.WorkflowTask{
				WorkflowID: "test-workflow",
			}

			runner := &ResumableWorkflowRunner{
				config:       cfg,
				functions:    functionRegistry,
				workflowTask: workflowTask,
			}

			// Build switch cases from test data
			switchItems := make([]model.SwitchItem, len(tt.expressions))
			for i, expr := range tt.expressions {
				switchItems[i] = model.SwitchItem{
					expr.caseName: model.SwitchCase{
						When: &model.RuntimeExpression{
							Value: expr.when,
						},
						Then: &model.FlowDirective{
							Value: expr.then,
						},
					},
				}
			}

			switchTask := &model.SwitchTask{
				Switch: switchItems,
			}

			// Execute the switch task
			result, err := runner.evaluateSwitchTask(tt.input, "testSwitch", switchTask)

			// Verify the result
			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedResult, result.Value)
		})
	}
}

func TestEvaluateSwitchTask_NumericComparison(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]any
		expressions []struct {
			caseName string
			when     string
			then     string
		}
		expectedResult string
	}{
		{
			name: "Small amount",
			input: map[string]any{
				"amount": 50,
			},
			expressions: []struct {
				caseName string
				when     string
				then     string
			}{
				{"small", `.amount < 100`, "processSmall"},
				{"medium", `.amount >= 100 and .amount < 500`, "processMedium"},
				{"large", `.amount >= 500`, "processLarge"},
			},
			expectedResult: "processSmall",
		},
		{
			name: "Medium amount",
			input: map[string]any{
				"amount": 250,
			},
			expressions: []struct {
				caseName string
				when     string
				then     string
			}{
				{"small", `.amount < 100`, "processSmall"},
				{"medium", `.amount >= 100 and .amount < 500`, "processMedium"},
				{"large", `.amount >= 500`, "processLarge"},
			},
			expectedResult: "processMedium",
		},
		{
			name: "Large amount",
			input: map[string]any{
				"amount": 750,
			},
			expressions: []struct {
				caseName string
				when     string
				then     string
			}{
				{"small", `.amount < 100`, "processSmall"},
				{"medium", `.amount >= 100 and .amount < 500`, "processMedium"},
				{"large", `.amount >= 500`, "processLarge"},
			},
			expectedResult: "processLarge",
		},
		{
			name: "Boundary case - exactly 100",
			input: map[string]any{
				"amount": 100,
			},
			expressions: []struct {
				caseName string
				when     string
				then     string
			}{
				{"small", `.amount < 100`, "processSmall"},
				{"medium", `.amount >= 100`, "processMedium"},
			},
			expectedResult: "processMedium",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test runner
			cfg := &config.Config{}
			functionRegistry := functions.NewFunctionRegistry(cfg)
			workflowTask := &models.WorkflowTask{
				WorkflowID: "test-workflow",
			}

			runner := &ResumableWorkflowRunner{
				config:       cfg,
				functions:    functionRegistry,
				workflowTask: workflowTask,
			}

			// Build switch cases from test data
			switchItems := make([]model.SwitchItem, len(tt.expressions))
			for i, expr := range tt.expressions {
				switchItems[i] = model.SwitchItem{
					expr.caseName: model.SwitchCase{
						When: &model.RuntimeExpression{
							Value: expr.when,
						},
						Then: &model.FlowDirective{
							Value: expr.then,
						},
					},
				}
			}

			switchTask := &model.SwitchTask{
				Switch: switchItems,
			}

			// Execute the switch task
			result, err := runner.evaluateSwitchTask(tt.input, "testSwitch", switchTask)

			// Verify the result
			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedResult, result.Value)
		})
	}
}

func TestEvaluateSwitchTask_BooleanConditions(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]any
		expressions []struct {
			caseName string
			when     string
			then     string
		}
		expectedResult string
	}{
		{
			name: "VIP user with discount",
			input: map[string]any{
				"isVip":       true,
				"hasDiscount": true,
			},
			expressions: []struct {
				caseName string
				when     string
				then     string
			}{
				{"vipWithDiscount", `.isVip == true and .hasDiscount == true`, "applyVipDiscount"},
				{"vipNoDiscount", `.isVip == true and .hasDiscount == false`, "applyVipPricing"},
				{"regular", `.isVip == false`, "applyRegularPricing"},
			},
			expectedResult: "applyVipDiscount",
		},
		{
			name: "VIP user without discount",
			input: map[string]any{
				"isVip":       true,
				"hasDiscount": false,
			},
			expressions: []struct {
				caseName string
				when     string
				then     string
			}{
				{"vipWithDiscount", `.isVip == true and .hasDiscount == true`, "applyVipDiscount"},
				{"vipNoDiscount", `.isVip == true and .hasDiscount == false`, "applyVipPricing"},
				{"regular", `.isVip == false`, "applyRegularPricing"},
			},
			expectedResult: "applyVipPricing",
		},
		{
			name: "Regular user",
			input: map[string]any{
				"isVip":       false,
				"hasDiscount": true,
			},
			expressions: []struct {
				caseName string
				when     string
				then     string
			}{
				{"vipWithDiscount", `.isVip == true and .hasDiscount == true`, "applyVipDiscount"},
				{"vipNoDiscount", `.isVip == true and .hasDiscount == false`, "applyVipPricing"},
				{"regular", `.isVip == false`, "applyRegularPricing"},
			},
			expectedResult: "applyRegularPricing",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test runner
			cfg := &config.Config{}
			functionRegistry := functions.NewFunctionRegistry(cfg)
			workflowTask := &models.WorkflowTask{
				WorkflowID: "test-workflow",
			}

			runner := &ResumableWorkflowRunner{
				config:       cfg,
				functions:    functionRegistry,
				workflowTask: workflowTask,
			}

			// Build switch cases from test data
			switchItems := make([]model.SwitchItem, len(tt.expressions))
			for i, expr := range tt.expressions {
				switchItems[i] = model.SwitchItem{
					expr.caseName: model.SwitchCase{
						When: &model.RuntimeExpression{
							Value: expr.when,
						},
						Then: &model.FlowDirective{
							Value: expr.then,
						},
					},
				}
			}

			switchTask := &model.SwitchTask{
				Switch: switchItems,
			}

			// Execute the switch task
			result, err := runner.evaluateSwitchTask(tt.input, "testSwitch", switchTask)

			// Verify the result
			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedResult, result.Value)
		})
	}
}

func TestEvaluateSwitchTask_NestedObjectAccess(t *testing.T) {
	tests := []struct {
		name        string
		input       map[string]any
		expressions []struct {
			caseName string
			when     string
			then     string
		}
		expectedResult string
	}{
		{
			name: "Access nested user type",
			input: map[string]any{
				"user": map[string]any{
					"type":     "premium",
					"verified": true,
				},
			},
			expressions: []struct {
				caseName string
				when     string
				then     string
			}{
				{"premium", `.user.type == "premium"`, "processPremium"},
				{"standard", `.user.type == "standard"`, "processStandard"},
			},
			expectedResult: "processPremium",
		},
		{
			name: "Complex nested conditions",
			input: map[string]any{
				"order": map[string]any{
					"customer": map[string]any{
						"tier": "gold",
					},
					"amount": 500,
				},
			},
			expressions: []struct {
				caseName string
				when     string
				then     string
			}{
				{"goldLarge", `.order.customer.tier == "gold" and .order.amount > 300`, "processGoldLarge"},
				{"goldSmall", `.order.customer.tier == "gold" and .order.amount <= 300`, "processGoldSmall"},
				{"regular", `.order.customer.tier != "gold"`, "processRegular"},
			},
			expectedResult: "processGoldLarge",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a test runner
			cfg := &config.Config{}
			functionRegistry := functions.NewFunctionRegistry(cfg)
			workflowTask := &models.WorkflowTask{
				WorkflowID: "test-workflow",
			}

			runner := &ResumableWorkflowRunner{
				config:       cfg,
				functions:    functionRegistry,
				workflowTask: workflowTask,
			}

			// Build switch cases from test data
			switchItems := make([]model.SwitchItem, len(tt.expressions))
			for i, expr := range tt.expressions {
				switchItems[i] = model.SwitchItem{
					expr.caseName: model.SwitchCase{
						When: &model.RuntimeExpression{
							Value: expr.when,
						},
						Then: &model.FlowDirective{
							Value: expr.then,
						},
					},
				}
			}

			switchTask := &model.SwitchTask{
				Switch: switchItems,
			}

			// Execute the switch task
			result, err := runner.evaluateSwitchTask(tt.input, "testSwitch", switchTask)

			// Verify the result
			require.NoError(t, err)
			assert.NotNil(t, result)
			assert.Equal(t, tt.expectedResult, result.Value)
		})
	}
}

func TestEvaluateSwitchTask_DefaultCase(t *testing.T) {
	// Create a test runner
	cfg := &config.Config{}
	functionRegistry := functions.NewFunctionRegistry(cfg)
	workflowTask := &models.WorkflowTask{
		WorkflowID: "test-workflow",
	}

	runner := &ResumableWorkflowRunner{
		config:       cfg,
		functions:    functionRegistry,
		workflowTask: workflowTask,
	}

	// Input that won't match any specific case
	input := map[string]any{
		"color": "purple",
	}

	// Switch with specific cases and a default
	switchTask := &model.SwitchTask{
		Switch: []model.SwitchItem{
			{
				"red": model.SwitchCase{
					When: &model.RuntimeExpression{
						Value: `.color == "red"`,
					},
					Then: &model.FlowDirective{
						Value: "processRed",
					},
				},
			},
			{
				"blue": model.SwitchCase{
					When: &model.RuntimeExpression{
						Value: `.color == "blue"`,
					},
					Then: &model.FlowDirective{
						Value: "processBlue",
					},
				},
			},
			{
				"default": model.SwitchCase{
					// No When condition = default case
					Then: &model.FlowDirective{
						Value: "processOther",
					},
				},
			},
		},
	}

	// Execute the switch task
	result, err := runner.evaluateSwitchTask(input, "testSwitch", switchTask)

	// Verify the result falls back to default
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "processOther", result.Value)
}

func TestEvaluateSwitchTask_FirstMatchWins(t *testing.T) {
	// Create a test runner
	cfg := &config.Config{}
	functionRegistry := functions.NewFunctionRegistry(cfg)
	workflowTask := &models.WorkflowTask{
		WorkflowID: "test-workflow",
	}

	runner := &ResumableWorkflowRunner{
		config:       cfg,
		functions:    functionRegistry,
		workflowTask: workflowTask,
	}

	// Input that would match multiple conditions
	input := map[string]any{
		"value": 50,
	}

	// Switch where multiple conditions would match
	switchTask := &model.SwitchTask{
		Switch: []model.SwitchItem{
			{
				"lessThan100": model.SwitchCase{
					When: &model.RuntimeExpression{
						Value: `.value < 100`,
					},
					Then: &model.FlowDirective{
						Value: "processSmall",
					},
				},
			},
			{
				"lessThan200": model.SwitchCase{
					When: &model.RuntimeExpression{
						Value: `.value < 200`,
					},
					Then: &model.FlowDirective{
						Value: "processMedium",
					},
				},
			},
		},
	}

	// Execute the switch task
	result, err := runner.evaluateSwitchTask(input, "testSwitch", switchTask)

	// Verify first match wins
	require.NoError(t, err)
	assert.NotNil(t, result)
	assert.Equal(t, "processSmall", result.Value) // First matching case
}

func TestEvaluateSwitchTask_ErrorCases(t *testing.T) {
	// Create a test runner
	cfg := &config.Config{}
	functionRegistry := functions.NewFunctionRegistry(cfg)
	workflowTask := &models.WorkflowTask{
		WorkflowID: "test-workflow",
	}

	runner := &ResumableWorkflowRunner{
		config:       cfg,
		functions:    functionRegistry,
		workflowTask: workflowTask,
	}

	t.Run("No matching case and no default", func(t *testing.T) {
		input := map[string]any{
			"color": "purple",
		}

		switchTask := &model.SwitchTask{
			Switch: []model.SwitchItem{
				{
					"red": model.SwitchCase{
						When: &model.RuntimeExpression{
							Value: `.color == "red"`,
						},
						Then: &model.FlowDirective{
							Value: "processRed",
						},
					},
				},
			},
		}

		result, err := runner.evaluateSwitchTask(input, "testSwitch", switchTask)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "no matching switch case")
	})

	t.Run("Empty switch cases", func(t *testing.T) {
		input := map[string]any{
			"value": "test",
		}

		switchTask := &model.SwitchTask{
			Switch: []model.SwitchItem{},
		}

		result, err := runner.evaluateSwitchTask(input, "testSwitch", switchTask)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "no switch cases defined")
	})

	t.Run("Nil switch task", func(t *testing.T) {
		input := map[string]any{
			"value": "test",
		}

		result, err := runner.evaluateSwitchTask(input, "testSwitch", nil)
		assert.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "no switch cases defined")
	})
}
