package interpolate

import (
	"errors"
	"fmt"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/serverlessworkflow/sdk-go/v3/model"
	"github.com/sirupsen/logrus"
)

func NewTraverse(node any, input any, variables map[string]any) (any, error) {
	return traverseAndEvaluate(node, input, variables)
}

func traverseAndEvaluate(node any, input any, variables map[string]any) (any, error) {
	switch v := node.(type) {
	case map[string]any:
		// Traverse map
		for key, value := range v {
			evaluatedValue, err := traverseAndEvaluate(value, input, variables)
			if err != nil {

				logrus.WithFields(logrus.Fields{
					"key":       key,
					"input":     input,
					"variables": variables,
				}).WithError(err).Error("Failed to evaluate expression in map")

				return nil, err
			}
			v[key] = evaluatedValue
		}
		return v, nil

	case []any:
		// Traverse array
		for i, value := range v {
			evaluatedValue, err := traverseAndEvaluate(value, input, variables)
			if err != nil {
				return nil, err
			}
			v[i] = evaluatedValue
		}
		return v, nil

	case string:

		// Remove leading/trailing whitespace and newlines
		v = strings.TrimSpace(v)

		// Check if the string is a runtime expression (e.g., ${ .some.path })
		if model.IsStrictExpr(v) {
			return evaluateJQExpression(model.SanitizeExpr(v), input, variables)
		}
		return v, nil

	case *model.Duration:

		expr := v.AsExpression()

		if model.IsStrictExpr(expr) {
			return evaluateJQExpression(model.SanitizeExpr(expr), input, variables)
		}
		return v, nil

	default:
		// Return other types as-is
		return v, nil
	}
}

// evaluateJQExpression evaluates a jq expression against a given JSON input
func evaluateJQExpression(expression string, input any, variables map[string]any) (any, error) {
	query, err := gojq.Parse(expression)
	if err != nil {
		return nil, fmt.Errorf("failed to parse jq expression: %s, error: %w", expression, err)
	}

	// Get the variable names & values in a single pass:
	names, values := getVariableNamesAndValues(variables)

	code, err := gojq.Compile(query, gojq.WithVariables(names))
	if err != nil {
		return nil, fmt.Errorf("failed to compile jq expression: %s, error: %w", expression, err)
	}

	iter := code.Run(input, values...)
	result, ok := iter.Next()
	if !ok {
		return nil, errors.New("no result from jq evaluation")
	}

	// If there's an error from the jq engine, report it
	if errVal, isErr := result.(error); isErr {
		return nil, fmt.Errorf("jq evaluation error: %w", errVal)
	}

	return result, nil
}

// getVariableNamesAndValues constructs two slices, where 'names[i]' matches 'values[i]'.
func getVariableNamesAndValues(vars map[string]any) ([]string, []any) {
	names := make([]string, 0, len(vars))
	values := make([]any, 0, len(vars))

	for k, v := range vars {
		names = append(names, k)
		values = append(values, v)
	}
	return names, values
}
