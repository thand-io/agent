package common

import (
	"github.com/go-playground/validator/v10"
	internal "github.com/thand-io/agent/internal/common"
)

func GetValidator() *validator.Validate {
	return internal.GetValidator()
}
