package common

import "net/url"

func IsValidLoginServer(hostname string) bool {

	// paarse url
	_, err := url.Parse(hostname)

	return err == nil
}

// IsAllDigits checks if a string contains only digits (0-9)
// This is optimized for speed by checking each byte directly
func IsAllDigits(s string) bool {
	if len(s) == 0 {
		return false
	}

	for i := 0; i < len(s); i++ {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}

	return true
}
