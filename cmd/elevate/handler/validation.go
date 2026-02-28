package handler

import (
	"fmt"

	"github.com/thand-io/agent/cmd/elevate/identity"
)

func validateRequestUsername(username string) *responseError {
	switch {
	case username == "":
		return invalidRequestErr(fmt.Errorf("username is required"))
	case len(username) > identity.MaxAccountNameLength:
		return invalidRequestErr(fmt.Errorf("username exceeds maximum length"))
	case !identity.ValidAccountName(username):
		return invalidRequestErr(fmt.Errorf("username contains unsupported characters"))
	default:
		return nil
	}
}
