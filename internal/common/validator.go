package common

import (
	"net/mail"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/go-playground/validator/v10"
)

var (
	// validatorInstance is the singleton validator instance
	validatorInstance *validator.Validate
	validatorOnce     sync.Once

	// validatorRegistrations holds custom validator registration functions
	// These are called when the validator is first initialized
	validatorRegistrations   []func(*validator.Validate) error
	validatorRegistrationsMu sync.Mutex

	// semanticVersionPattern for semver validation
	semanticVersionPattern = regexp.MustCompile(`^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)(?:-((?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*)(?:\.(?:0|[1-9]\d*|\d*[a-zA-Z-][0-9a-zA-Z-]*))*))?(?:\+([0-9a-zA-Z-]+(?:\.[0-9a-zA-Z-]+)*))?$`)
)

// RegisterCustomValidator registers a custom validator registration function.
// The function will be called when GetValidator() initializes the validator instance.
// This allows packages to register their validators without creating circular imports.
// Must be called before GetValidator() is first invoked (typically in init()).
func RegisterCustomValidator(fn func(*validator.Validate) error) {
	validatorRegistrationsMu.Lock()
	defer validatorRegistrationsMu.Unlock()
	validatorRegistrations = append(validatorRegistrations, fn)
}

// GetValidator returns a singleton validator instance with all custom validators registered
func GetValidator() *validator.Validate {
	validatorOnce.Do(func() {
		validatorInstance = validator.New()

		// Register custom validator for semantic versioning
		if err := validatorInstance.RegisterValidation("semver_pattern", func(fl validator.FieldLevel) bool {
			value := fl.Field().String()
			return semanticVersionPattern.MatchString(value)
		}); err != nil {
			// Log error but don't panic - validation will just fail if this doesn't work
			// In a production system, you might want to panic here since validation is critical
		}

		// Register custom validator for alphanumeric with hyphens, underscores, and dots
		// Used by Provider field validation in models.Provider
		if err := validatorInstance.RegisterValidation("alphanum_hyphen", func(fl validator.FieldLevel) bool {
			value := fl.Field().String()
			// Allow alphanumeric, hyphens, underscores, and dots
			// Must start with alphanumeric character
			matched, _ := regexp.MatchString(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`, value)
			return matched
		}); err != nil {
			// Log error but don't panic
		}

		// Call all registered custom validator functions
		validatorRegistrationsMu.Lock()
		registrations := validatorRegistrations
		validatorRegistrationsMu.Unlock()

		for _, fn := range registrations {
			if err := fn(validatorInstance); err != nil {
				// Log error but don't panic
			}
		}
	})

	return validatorInstance
}

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

func IsValidURL(rawurl string) bool {
	_, err := url.ParseRequestURI(rawurl)
	return err == nil
}

func IsValidEmail(email string) bool {
	_, err := mail.ParseAddress(email)
	return err == nil
}

func IsValidNumber(value string, decimalAllowed bool) bool {
	_, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return false
	}
	if !decimalAllowed && strings.Contains(value, ".") {
		return false
	}
	return true
}

// ValidateNumberRange checks if a number value is within the specified min/max range.
// Returns an error message if validation fails, or an empty string if validation passes.
// Empty minValue or maxValue strings are treated as no constraint.
func ValidateNumberRange(value string, minValue string, maxValue string) string {
	num, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return "Please enter a valid number"
	}

	if len(minValue) != 0 {
		min, err := strconv.ParseFloat(minValue, 64)
		if err == nil && num < min {
			return "Value must be at least " + minValue
		}
	}

	if len(maxValue) != 0 {
		max, err := strconv.ParseFloat(maxValue, 64)
		if err == nil && num > max {
			return "Value must be at most " + maxValue
		}
	}

	return ""
}
