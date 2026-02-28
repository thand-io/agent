package handler

import (
	"fmt"
	"regexp"
)

const maxUsernameLength = 32

var usernamePattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9._-]*[$]?$`)

func validateRequestUsername(username string) *responseError {
	switch {
	case username == "":
		return invalidRequestErr(fmt.Errorf("username is required"))
	case len(username) > maxUsernameLength:
		return invalidRequestErr(fmt.Errorf("username exceeds maximum length"))
	case !usernamePattern.MatchString(username):
		return invalidRequestErr(fmt.Errorf("username contains unsupported characters"))
	default:
		return nil
	}
}
