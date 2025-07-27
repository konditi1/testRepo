package validation

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/go-playground/validator/v10"
)

var validate = validator.New()

// ValidateStruct validates a struct using go-playground/validator
func ValidateStruct(s interface{}) error {
	if s == nil {
		return nil
	}

	// Check if it's a pointer to a struct
	val := reflect.ValueOf(s)
	if val.Kind() == reflect.Ptr {
		val = val.Elem()
	}

	if val.Kind() != reflect.Struct {
		return fmt.Errorf("validator: expected a struct, got %T", s)
	}

	err := validate.Struct(s)
	if err != nil {
		// Convert validation errors to a more user-friendly format
		if ve, ok := err.(validator.ValidationErrors); ok {
			errMsgs := make([]string, 0, len(ve))
			for _, e := range ve {
				errMsgs = append(errMsgs, fmt.Sprintf("field '%s' failed validation: %s", e.Field(), e.Tag()))
			}
			return fmt.Errorf(strings.Join(errMsgs, "; "))
		}
		return fmt.Errorf("validation failed: %w", err)
	}

	return nil
}
