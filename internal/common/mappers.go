package common

import (
	"encoding/json"
	"strings"
	"unicode"
)

// Function convert map[string]any into a given interface
func ConvertMapToInterface(m map[string]any, i any) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, i)
}

func ConvertInterfaceToInterface(from any, to any) error {

	if from == nil {
		return nil
	}

	data, err := json.Marshal(from)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, to)
}

func ConvertInterfaceToMap(from any) (map[string]any, error) {
	if from == nil {
		return nil, nil
	}

	data, err := json.Marshal(from)
	if err != nil {
		return nil, err
	}
	var result map[string]any
	err = json.Unmarshal(data, &result)
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ConvertToSnakeCase transforms a string into a clean snake_case identifier.
// Only lowercase letters, digits, and underscores are kept.
// All other characters (spaces, hyphens, dots, special chars, etc.) are
// replaced with underscores, consecutive underscores are collapsed, and
// leading/trailing underscores are trimmed.
func ConvertToSnakeCase(name string) string {
	var builder strings.Builder
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(unicode.ToLower(r))
		} else {
			builder.WriteRune('_')
		}
	}
	result := builder.String()

	// Collapse consecutive underscores
	for strings.Contains(result, "__") {
		result = strings.ReplaceAll(result, "__", "_")
	}

	return strings.Trim(result, "_")
}
