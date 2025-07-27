// file: internal/models/validation.go
package models

import (
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"go.uber.org/zap"
)

// ===============================
// VALIDATION ERRORS
// ===============================

// ValidationError represents a validation error
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
	Code    string `json:"code"`
	Value   interface{} `json:"value,omitempty"`
}

// Error implements the error interface
func (e ValidationError) Error() string {
	return fmt.Sprintf("validation failed for field '%s': %s", e.Field, e.Message)
}

// ValidationErrors represents multiple validation errors
type ValidationErrors []ValidationError

// Error implements the error interface
func (e ValidationErrors) Error() string {
	if len(e) == 0 {
		return "no validation errors"
	}
	if len(e) == 1 {
		return e[0].Error()
	}
	return fmt.Sprintf("validation failed with %d errors", len(e))
}

// Add adds a validation error
func (e *ValidationErrors) Add(field, message, code string, value interface{}) {
	*e = append(*e, ValidationError{
		Field:   field,
		Message: message,
		Code:    code,
		Value:   value,
	})
}

// HasErrors returns true if there are validation errors
func (e ValidationErrors) HasErrors() bool {
	return len(e) > 0
}

// GetField returns all errors for a specific field
func (e ValidationErrors) GetField(field string) []ValidationError {
	var fieldErrors []ValidationError
	for _, err := range e {
		if err.Field == field {
			fieldErrors = append(fieldErrors, err)
		}
	}
	return fieldErrors
}

// ===============================
// VALIDATOR INTERFACE
// ===============================

// Validator defines the validation interface
type Validator interface {
	Validate() ValidationErrors
}

// ValidatorWithContext defines validation with context
type ValidatorWithContext interface {
	ValidateWithContext(ctx ValidationContext) ValidationErrors
}

// ValidationContext provides context for validation
type ValidationContext struct {
	UserID    *int64
	IsUpdate  bool
	FieldMask []string
	Logger    *zap.Logger
}

// ===============================
// VALIDATION RULES
// ===============================

// ValidationRule represents a single validation rule
type ValidationRule func(value interface{}) *ValidationError

// ValidationRules holds multiple validation rules for a field
type ValidationRules map[string][]ValidationRule

// ===============================
// CORE VALIDATORS
// ===============================

// EmailValidator validates email addresses
func EmailValidator(field string, value string) *ValidationError {
	if value == "" {
		return &ValidationError{
			Field:   field,
			Message: "email is required",
			Code:    "required",
			Value:   value,
		}
	}

	// Basic email regex pattern
	emailRegex := regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	if !emailRegex.MatchString(value) {
		return &ValidationError{
			Field:   field,
			Message: "invalid email format",
			Code:    "invalid_format",
			Value:   value,
		}
	}

	// Check email length (RFC 5321 limit)
	if len(value) > 320 {
		return &ValidationError{
			Field:   field,
			Message: "email too long (max 320 characters)",
			Code:    "too_long",
			Value:   value,
		}
	}

	return nil
}

// UsernameValidator validates usernames
func UsernameValidator(field string, value string) *ValidationError {
	if value == "" {
		return &ValidationError{
			Field:   field,
			Message: "username is required",
			Code:    "required",
			Value:   value,
		}
	}

	// Check length
	if len(value) < 3 {
		return &ValidationError{
			Field:   field,
			Message: "username must be at least 3 characters",
			Code:    "too_short",
			Value:   value,
		}
	}

	if len(value) > 50 {
		return &ValidationError{
			Field:   field,
			Message: "username must be 50 characters or less",
			Code:    "too_long",
			Value:   value,
		}
	}

	// Check allowed characters
	usernameRegex := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !usernameRegex.MatchString(value) {
		return &ValidationError{
			Field:   field,
			Message: "username can only contain letters, numbers, underscores, and hyphens",
			Code:    "invalid_characters",
			Value:   value,
		}
	}

	// Check for reserved usernames
	reserved := []string{
		"admin", "administrator", "root", "system", "api", "app",
		"www", "ftp", "mail", "email", "support", "help", "info",
		"test", "demo", "example", "null", "undefined", "anonymous",
	}
	
	lowerValue := strings.ToLower(value)
	for _, r := range reserved {
		if lowerValue == r {
			return &ValidationError{
				Field:   field,
				Message: "username is reserved",
				Code:    "reserved",
				Value:   value,
			}
		}
	}

	return nil
}

// PasswordValidator validates passwords
func PasswordValidator(field string, value string) *ValidationError {
	if value == "" {
		return &ValidationError{
			Field:   field,
			Message: "password is required",
			Code:    "required",
			Value:   value,
		}
	}

	// Check minimum length
	if len(value) < 8 {
		return &ValidationError{
			Field:   field,
			Message: "password must be at least 8 characters",
			Code:    "too_short",
			Value:   value,
		}
	}

	// Check maximum length
	if len(value) > 128 {
		return &ValidationError{
			Field:   field,
			Message: "password must be 128 characters or less",
			Code:    "too_long",
			Value:   value,
		}
	}

	// Check for required character types
	var hasLower, hasUpper, hasDigit, hasSpecial bool
	
	for _, char := range value {
		switch {
		case unicode.IsLower(char):
			hasLower = true
		case unicode.IsUpper(char):
			hasUpper = true
		case unicode.IsDigit(char):
			hasDigit = true
		case unicode.IsPunct(char) || unicode.IsSymbol(char):
			hasSpecial = true
		}
	}

	var missing []string
	if !hasLower {
		missing = append(missing, "lowercase letter")
	}
	if !hasUpper {
		missing = append(missing, "uppercase letter")
	}
	if !hasDigit {
		missing = append(missing, "number")
	}
	if !hasSpecial {
		missing = append(missing, "special character")
	}

	if len(missing) > 0 {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("password must contain at least one: %s", strings.Join(missing, ", ")),
			Code:    "weak_password",
			Value:   value,
		}
	}

	return nil
}

// URLValidator validates URLs
func URLValidator(field string, value string) *ValidationError {
	if value == "" {
		return nil // Optional field
	}

	parsedURL, err := url.Parse(value)
	if err != nil {
		return &ValidationError{
			Field:   field,
			Message: "invalid URL format",
			Code:    "invalid_format",
			Value:   value,
		}
	}

	// Check scheme
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return &ValidationError{
			Field:   field,
			Message: "URL must use http or https scheme",
			Code:    "invalid_scheme",
			Value:   value,
		}
	}

	// Check host
	if parsedURL.Host == "" {
		return &ValidationError{
			Field:   field,
			Message: "URL must have a valid host",
			Code:    "missing_host",
			Value:   value,
		}
	}

	return nil
}

// ContentValidator validates content fields
func ContentValidator(field string, value string, minLength, maxLength int) *ValidationError {
	if value == "" {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("%s is required", field),
			Code:    "required",
			Value:   value,
		}
	}

	// Trim whitespace for length check
	trimmed := strings.TrimSpace(value)
	if len(trimmed) < minLength {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("%s must be at least %d characters", field, minLength),
			Code:    "too_short",
			Value:   value,
		}
	}

	if len(value) > maxLength {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("%s must be %d characters or less", field, maxLength),
			Code:    "too_long",
			Value:   value,
		}
	}

	// Check for suspicious content
	if err := validateContentSafety(value); err != nil {
		return &ValidationError{
			Field:   field,
			Message: err.Error(),
			Code:    "unsafe_content",
			Value:   value,
		}
	}

	return nil
}

// EnumValidator validates enum values
func EnumValidator(field string, value string, allowedValues []string) *ValidationError {
	if value == "" {
		return &ValidationError{
			Field:   field,
			Message: fmt.Sprintf("%s is required", field),
			Code:    "required",
			Value:   value,
		}
	}

	for _, allowed := range allowedValues {
		if value == allowed {
			return nil
		}
	}

	return &ValidationError{
		Field:   field,
		Message: fmt.Sprintf("%s must be one of: %s", field, strings.Join(allowedValues, ", ")),
		Code:    "invalid_value",
		Value:   value,
	}
}

// ===============================
// MODEL VALIDATORS
// ===============================

// Validate validates a User model
func (u *User) Validate() ValidationErrors {
	var errors ValidationErrors

	// Validate email
	if err := EmailValidator("email", u.Email); err != nil {
		errors = append(errors, *err)
	}

	// Validate username
	if err := UsernameValidator("username", u.Username); err != nil {
		errors = append(errors, *err)
	}

	// Validate optional fields
	if u.FirstName != nil && len(*u.FirstName) > 100 {
		errors.Add("first_name", "first name must be 100 characters or less", "too_long", *u.FirstName)
	}

	if u.LastName != nil && len(*u.LastName) > 100 {
		errors.Add("last_name", "last name must be 100 characters or less", "too_long", *u.LastName)
	}

	if u.Bio != nil && len(*u.Bio) > 1000 {
		errors.Add("bio", "bio must be 1000 characters or less", "too_long", *u.Bio)
	}

	// Validate years of experience
	if u.YearsExperience < 0 || u.YearsExperience > 100 {
		errors.Add("years_experience", "years of experience must be between 0 and 100", "invalid_range", u.YearsExperience)
	}

	// Validate expertise level
	expertiseLevels := []string{"none", "beginner", "intermediate", "advanced", "expert"}
	if err := EnumValidator("expertise", u.Expertise, expertiseLevels); err != nil {
		errors = append(errors, *err)
	}

	// Validate role
	roles := []string{"user", "reviewer", "moderator", "admin"}
	if err := EnumValidator("role", u.Role, roles); err != nil {
		errors = append(errors, *err)
	}

	// Validate URLs
	if u.WebsiteURL != nil {
		if err := URLValidator("website_url", *u.WebsiteURL); err != nil {
			errors = append(errors, *err)
		}
	}

	// Validate Twitter handle
	if u.TwitterHandle != nil && *u.TwitterHandle != "" {
		if !regexp.MustCompile(`^[A-Za-z0-9_]{1,50}$`).MatchString(*u.TwitterHandle) {
			errors.Add("twitter_handle", "invalid Twitter handle format", "invalid_format", *u.TwitterHandle)
		}
	}

	return errors
}

// Validate validates a Post model
func (p *Post) Validate() ValidationErrors {
	var errors ValidationErrors

	// Validate title
	if err := ContentValidator("title", p.Title, 5, 255); err != nil {
		errors = append(errors, *err)
	}

	// Validate content
	if err := ContentValidator("content", p.Content, 10, 50000); err != nil {
		errors = append(errors, *err)
	}

	// Validate category
	if p.Category == "" {
		errors.Add("category", "category is required", "required", p.Category)
	} else if len(p.Category) > 100 {
		errors.Add("category", "category must be 100 characters or less", "too_long", p.Category)
	}

	// Validate status
	statuses := []string{"draft", "published", "archived", "deleted", "flagged", "approved", "rejected"}
	if err := EnumValidator("status", p.Status, statuses); err != nil {
		errors = append(errors, *err)
	}

	// Validate user ID
	if p.UserID <= 0 {
		errors.Add("user_id", "valid user ID is required", "invalid", p.UserID)
	}

	return errors
}

// Validate validates a Comment model
func (c *Comment) Validate() ValidationErrors {
	var errors ValidationErrors

	// Validate content
	if err := ContentValidator("content", c.Content, 1, 10000); err != nil {
		errors = append(errors, *err)
	}

	// Validate user ID
	if c.UserID <= 0 {
		errors.Add("user_id", "valid user ID is required", "invalid", c.UserID)
	}

	// Validate parent references (exactly one must be set)
	parentCount := 0
	if c.PostID != nil && *c.PostID > 0 {
		parentCount++
	}
	if c.QuestionID != nil && *c.QuestionID > 0 {
		parentCount++
	}
	if c.DocumentID != nil && *c.DocumentID > 0 {
		parentCount++
	}

	if parentCount != 1 {
		errors.Add("parent", "comment must belong to exactly one parent (post, question, or document)", "invalid_parent", nil)
	}

	return errors
}

// Validate validates a Session model
func (s *Session) Validate() ValidationErrors {
	var errors ValidationErrors

	// Validate user ID
	if s.UserID <= 0 {
		errors.Add("user_id", "valid user ID is required", "invalid", s.UserID)
	}

	// Validate session token
	if s.SessionToken == "" {
		errors.Add("session_token", "session token is required", "required", s.SessionToken)
	} else if len(s.SessionToken) < 32 {
		errors.Add("session_token", "session token too short (minimum 32 characters)", "too_short", s.SessionToken)
	}

	// Validate expiry
	if s.ExpiresAt.IsZero() {
		errors.Add("expires_at", "expiry time is required", "required", s.ExpiresAt)
	} else if s.ExpiresAt.Before(time.Now()) {
		errors.Add("expires_at", "session cannot be created with past expiry time", "invalid_time", s.ExpiresAt)
	}

	return errors
}

// ===============================
// CONTENT SAFETY VALIDATION
// ===============================

// validateContentSafety checks for potentially harmful content
func validateContentSafety(content string) error {
	lowerContent := strings.ToLower(content)
	
	// Check for excessive repeated characters (spam indicator)
	if hasExcessiveRepetition(content) {
		return fmt.Errorf("content contains excessive repeated characters")
	}

	// Check for suspicious patterns
	suspiciousPatterns := []string{
		"javascript:",
		"<script",
		"onclick=",
		"onerror=",
		"onload=",
	}

	for _, pattern := range suspiciousPatterns {
		if strings.Contains(lowerContent, pattern) {
			return fmt.Errorf("content contains potentially unsafe elements")
		}
	}

	return nil
}

// hasExcessiveRepetition checks for spam-like repeated characters
func hasExcessiveRepetition(content string) bool {
	if len(content) < 10 {
		return false
	}

	// Check for more than 10 consecutive identical characters
	for i := 0; i < len(content)-10; i++ {
		char := content[i]
		count := 1
		
		for j := i + 1; j < len(content) && content[j] == char; j++ {
			count++
			if count > 10 {
				return true
			}
		}
	}

	return false
}

// ===============================
// VALIDATION UTILITIES
// ===============================

// ValidateModel validates any model that implements the Validator interface
func ValidateModel(model Validator) error {
	if errors := model.Validate(); errors.HasErrors() {
		return errors
	}
	return nil
}

// ValidateFields validates specific fields of a model
func ValidateFields(model interface{}, fieldMask []string) ValidationErrors {
	// This would use reflection to validate only specific fields
	// Implementation depends on the specific use case
	var errors ValidationErrors
	
	// Placeholder implementation
	// In a real implementation, this would use reflection to validate only the specified fields
	
	return errors
}

// SanitizeString removes potentially harmful content from strings
func SanitizeString(input string) string {
	// Remove null bytes
	input = strings.ReplaceAll(input, "\x00", "")
	
	// Trim whitespace
	input = strings.TrimSpace(input)
	
	// Remove excessive whitespace
	input = regexp.MustCompile(`\s+`).ReplaceAllString(input, " ")
	
	return input
}

// NormalizeEmail normalizes email addresses
func NormalizeEmail(email string) string {
	email = strings.ToLower(strings.TrimSpace(email))
	
	// Handle Gmail-specific normalization
	if strings.HasSuffix(email, "@gmail.com") {
		parts := strings.Split(email, "@")
		if len(parts) == 2 {
			localPart := parts[0]
			// Remove dots and everything after +
			localPart = strings.ReplaceAll(localPart, ".", "")
			if plusIndex := strings.Index(localPart, "+"); plusIndex > 0 {
				localPart = localPart[:plusIndex]
			}
			email = localPart + "@gmail.com"
		}
	}
	
	return email
}

// ===============================
// VALIDATION MIDDLEWARE SUPPORT
// ===============================

// ValidationResult represents the result of a validation operation
type ValidationResult struct {
	IsValid bool              `json:"is_valid"`
	Errors  ValidationErrors  `json:"errors,omitempty"`
	Model   interface{}       `json:"model,omitempty"`
}

// ValidateAndSanitize validates and sanitizes a model
func ValidateAndSanitize(model Validator) ValidationResult {
	// Perform validation
	errors := model.Validate()
	
	return ValidationResult{
		IsValid: !errors.HasErrors(),
		Errors:  errors,
		Model:   model,
	}
}
