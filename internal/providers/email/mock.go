package email

import (
	"context"
	"sync"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/thand-io/agent/internal/common"
	"github.com/thand-io/agent/internal/models"
)

// EmailInterceptor is a global interceptor for capturing emails during tests
var EmailInterceptor = NewMockEmailInterceptor()

// MockEmailMessage represents an intercepted email message
type MockEmailMessage struct {
	From      string
	To        []string
	Subject   string
	Body      string
	HTML      string
	Timestamp time.Time
	Request   models.NotificationRequest
}

// MockEmailInterceptor captures and stores emails for testing
type MockEmailInterceptor struct {
	mu       sync.RWMutex
	messages []MockEmailMessage
	waitCh   chan struct{}
}

// NewMockEmailInterceptor creates a new email interceptor
func NewMockEmailInterceptor() *MockEmailInterceptor {
	return &MockEmailInterceptor{
		messages: make([]MockEmailMessage, 0),
		waitCh:   make(chan struct{}, 100),
	}
}

// AddMessage adds an intercepted email message
func (e *MockEmailInterceptor) AddMessage(msg MockEmailMessage) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.messages = append(e.messages, msg)

	logrus.WithFields(logrus.Fields{
		"to":      msg.To,
		"subject": msg.Subject,
	}).Debug("Mock email interceptor captured message")

	// Signal that a new message arrived
	select {
	case e.waitCh <- struct{}{}:
	default:
	}
}

// GetMessages returns all intercepted messages
func (e *MockEmailInterceptor) GetMessages() []MockEmailMessage {
	e.mu.RLock()
	defer e.mu.RUnlock()
	result := make([]MockEmailMessage, len(e.messages))
	copy(result, e.messages)
	return result
}

// GetMessageCount returns the number of intercepted messages
func (e *MockEmailInterceptor) GetMessageCount() int {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return len(e.messages)
}

// Clear clears all intercepted messages
func (e *MockEmailInterceptor) Clear() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.messages = make([]MockEmailMessage, 0)
	// Drain the wait channel
	for len(e.waitCh) > 0 {
		<-e.waitCh
	}
}

// WaitForMessages waits for at least n messages within the timeout
func (e *MockEmailInterceptor) WaitForMessages(ctx context.Context, n int) error {
	for {
		if e.GetMessageCount() >= n {
			return nil
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-e.waitCh:
			// Check again
		case <-time.After(100 * time.Millisecond):
			// Periodic check
		}
	}
}

// FindMessageBySubject finds a message by subject substring
func (e *MockEmailInterceptor) FindMessageBySubject(substring string) *MockEmailMessage {
	e.mu.RLock()
	defer e.mu.RUnlock()
	for i := range e.messages {
		if contains(e.messages[i].Subject, substring) {
			msg := e.messages[i]
			return &msg
		}
	}
	return nil
}

// FindMessagesByRecipient finds all messages sent to a specific recipient
func (e *MockEmailInterceptor) FindMessagesByRecipient(recipient string) []MockEmailMessage {
	e.mu.RLock()
	defer e.mu.RUnlock()
	var result []MockEmailMessage
	for _, msg := range e.messages {
		for _, to := range msg.To {
			if to == recipient {
				result = append(result, msg)
				break
			}
		}
	}
	return result
}

// contains checks if s contains substr (case-insensitive would need strings package)
func contains(s, substr string) bool {
	return len(s) >= len(substr) && findSubstring(s, substr) >= 0
}

func findSubstring(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// emailProviderMock is a mock implementation of emailProvider for testing
type emailProviderMock struct {
	*models.BaseProvider
	interceptor        *MockEmailInterceptor
	defaultFromAddress string
}

// NewMockEmailProvider creates a new mock email provider for testing
func NewMockEmailProvider() models.Provider {
	return &emailProviderMock{
		interceptor: EmailInterceptor,
	}
}

// NewMockEmailProviderWithInterceptor creates a mock with a custom interceptor
func NewMockEmailProviderWithInterceptor(interceptor *MockEmailInterceptor) models.Provider {
	return &emailProviderMock{
		interceptor: interceptor,
	}
}

// Initialize sets up the mock email provider
func (p *emailProviderMock) Initialize(identifier string, provider models.ProviderConfig) error {
	p.BaseProvider = models.NewBaseProvider(
		identifier,
		provider,
		EmailCapabilities,
	)

	if provider.Config != nil {
		p.defaultFromAddress = provider.Config.GetStringWithDefault("from", "test@mock.thand.io")
	} else {
		p.defaultFromAddress = "test@mock.thand.io"
	}

	logrus.WithField("from", p.defaultFromAddress).Info("Mock email provider initialized")

	return nil
}

// SendNotification captures the email instead of sending it
func (p *emailProviderMock) SendNotification(
	ctx context.Context, notification models.NotificationRequest,
) error {
	// Convert NotificationRequest to EmailNotificationRequest
	emailRequest := &models.EmailNotificationRequest{}
	common.ConvertMapToInterface(notification, emailRequest)

	// Determine from address
	fromAddress := p.defaultFromAddress
	if len(emailRequest.From) > 0 {
		fromAddress = emailRequest.From
	}

	// Create the mock message
	msg := MockEmailMessage{
		From:      fromAddress,
		To:        emailRequest.To,
		Subject:   emailRequest.Subject,
		Body:      emailRequest.Body.Text,
		HTML:      emailRequest.Body.HTML,
		Timestamp: time.Now(),
		Request:   notification,
	}

	// Add to interceptor
	p.interceptor.AddMessage(msg)

	logrus.WithFields(logrus.Fields{
		"from":    msg.From,
		"to":      msg.To,
		"subject": msg.Subject,
	}).Info("Mock email provider captured notification")

	return nil
}

// GetInterceptor returns the email interceptor for test assertions
func (p *emailProviderMock) GetInterceptor() *MockEmailInterceptor {
	return p.interceptor
}
