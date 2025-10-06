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

/*
Convert everything to lowercase and only allow
these special characters: _+=,.@-
*/
func ConvertToSnakeCase(name string) string {
	var builder strings.Builder
	for i, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			builder.WriteRune(unicode.ToLower(r))
		} else if strings.ContainsRune("_+=,.@-", r) {
			builder.WriteRune(r)
		} else if unicode.IsSpace(r) {
			// Replace spaces with underscores
			if i > 0 && builder.Len() > 0 && builder.String()[builder.Len()-1] != '_' {
				builder.WriteRune('_')
			}
		}
	}
	return builder.String()
}
