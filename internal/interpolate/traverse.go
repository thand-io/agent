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
	// Normalize input and variables to convert Go-specific types to JSON-compatible types
	normalizedInput := normalizeValue(input)
	normalizedVars := make(map[string]any, len(variables))
	for k, v := range variables {
		normalizedVars[k] = normalizeValue(v)
	}
	return traverseAndEvaluate(node, normalizedInput, normalizedVars)
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

	// Convert Go-specific integer types to int
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	case int8:
		return int(v), nil
	case int16:
		return int(v), nil
	case uint:
		return int(v), nil
	case uint8:
		return int(v), nil
	case uint16:
		return int(v), nil
	case uint32:
		return int(v), nil
	case uint64:
		return int(v), nil

	// Convert float32 to float64
	case float32:
		return float64(v), nil

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

// normalizeValue recursively converts Go-specific types to JSON-compatible types
func normalizeValue(v any) any {
	if v == nil {
		return nil
	}

	switch val := v.(type) {
	case int32, int64, int8, int16:
		// Convert to int using reflection-free type assertion
		switch typed := v.(type) {
		case int32:
			return int(typed)
		case int64:
			return int(typed)
		case int8:
			return int(typed)
		case int16:
			return int(typed)
		}
	case uint, uint8, uint16, uint32, uint64:
		switch typed := v.(type) {
		case uint:
			return int(typed)
		case uint8:
			return int(typed)
		case uint16:
			return int(typed)
		case uint32:
			return int(typed)
		case uint64:
			return int(typed)
		}
	case float32:
		return float64(val)
	case []any:
		// Recursively normalize array elements
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = normalizeValue(item)
		}
		return result
	case []int32:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = int(item)
		}
		return result
	case []int64:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = int(item)
		}
		return result
	case []float32:
		result := make([]any, len(val))
		for i, item := range val {
			result[i] = float64(item)
		}
		return result
	case map[string]any:
		// Recursively normalize map values
		result := make(map[string]any, len(val))
		for k, item := range val {
			result[k] = normalizeValue(item)
		}
		return result
	}

	return v
}
