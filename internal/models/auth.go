package models

import (
	"crypto/rand"
	"fmt"
	"strings"
)

type AuthWrapper struct {
	Callback string `json:"callback"`
	Client   string `json:"client"`
	Provider string `json:"provider"`
	Code     string `json:"code"`
}

func NewAuthWrapper(callback string, client string, provider string) AuthWrapper {
	return AuthWrapper{
		Callback: callback,
		Client:   client,
		Provider: provider,
		Code:     createOneTimeCode(12),
	}
}

// Function to generate random hex code based on a provided length of hex
// returned as an uppercase string
func createOneTimeCode(length int) string {
	code := make([]byte, (length+1)/2)
	if _, err := rand.Read(code); err != nil {
		return ""
	}
	hexString := strings.ToUpper(fmt.Sprintf("%x", code))
	return fmt.Sprintf("%0*s", length, hexString)
}
