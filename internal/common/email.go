package common

import "strings"

// ExtractNameFromEmail extracts a display name from an email address
func ExtractNameFromEmail(email string) string {
	// Try to extract name from email format (e.g., "john.doe@example.com" -> "John Doe")
	parts := strings.Split(email, "@")
	if len(parts) == 0 {
		return email
	}

	localPart := parts[0]
	// Replace dots, underscores, and hyphens with spaces
	name := strings.ReplaceAll(localPart, ".", " ")
	name = strings.ReplaceAll(name, "_", " ")
	name = strings.ReplaceAll(name, "-", " ")

	// Capitalize each word
	words := strings.Fields(name)
	for i, word := range words {
		if len(word) > 0 {
			words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
		}
	}

	return strings.Join(words, " ")
}

func ExtractUsernameFromEmail(email string) string {
	return ConvertToSnakeCase(ExtractNameFromEmail(email))
}
