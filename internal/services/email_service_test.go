// file: internal/services/email_service_test.go
package services

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestSendVerificationEmail(t *testing.T) {
	// Create a test logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Create a new email service
	service := NewEmailService(logger)

	// Test data
	testEmail := "test@example.com"
	testToken := "test-verification-token"

	// Call the method
	err := service.SendVerificationEmail(context.Background(), testEmail, testToken)

	// Assert no error occurred
	assert.NoError(t, err, "SendVerificationEmail should not return an error")
}

func TestSendPasswordResetEmail(t *testing.T) {
	// Create a test logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Create a new email service
	service := NewEmailService(logger)

	// Test data
	testEmail := "test@example.com"
	testToken := "test-reset-token"

	// Call the method
	err := service.SendPasswordResetEmail(context.Background(), testEmail, testToken)

	// Assert no error occurred
	assert.NoError(t, err, "SendPasswordResetEmail should not return an error")
}
