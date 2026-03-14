package notification

import (
	"strings"
	"testing"

	channel "notif-api/internal/domain/messageChannel"
	"notif-api/internal/domain/priority"
)

func validNotificationRequest() NotificationRequest {
	return NotificationRequest{
		Recipient: "alice@example.com",
		Channel:   channel.ChannelEmail,
		Content:   "hello",
		Priority:  priority.PriorityHighLabel,
	}
}

func TestNotificationRequestValidate_Valid(t *testing.T) {
	req := validNotificationRequest()

	if err := req.Validate(); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestNotificationRequestValidate_WhitespaceRecipient(t *testing.T) {
	req := validNotificationRequest()
	req.Recipient = "   "

	err := req.Validate()
	if err == nil || !strings.Contains(err.Error(), "recipient is required") {
		t.Fatalf("expected recipient required error, got %v", err)
	}
}

func TestNotificationRequestValidate_WhitespaceContent(t *testing.T) {
	req := validNotificationRequest()
	req.Content = "\t\n"

	err := req.Validate()
	if err == nil || !strings.Contains(err.Error(), "content is required") {
		t.Fatalf("expected content required error, got %v", err)
	}
}
