package notification

import (
	"fmt"
	channel "notif-api/internal/domain/messageChannel"
	"notif-api/internal/domain/priority"
	"strings"
	"unicode/utf8"
)

const MaxContentLength = 1000
const MaxBatchSize = 1000

type NotificationRequest struct {
	Recipient string                 `json:"recipient" binding:"required"`
	Channel   channel.Channel        `json:"channel" binding:"required" swaggertype:"string" enums:"email,sms,push"`
	Content   string                 `json:"content" binding:"required"`
	Priority  priority.PriorityLabel `json:"priority" binding:"required" swaggertype:"string" enums:"high,medium,low"`
}

func (r NotificationRequest) Validate() error {
	if strings.TrimSpace(r.Recipient) == "" {
		return fmt.Errorf("recipient is required")
	}

	if strings.TrimSpace(r.Content) == "" {
		return fmt.Errorf("content is required")
	}

	if utf8.RuneCountInString(r.Content) > MaxContentLength {
		return fmt.Errorf("content must be at most %d characters", MaxContentLength)
	}

	if !r.Channel.IsValid() {
		return fmt.Errorf("channel must be one of: %s, %s, %s", channel.ChannelEmail, channel.ChannelSMS, channel.ChannelPush)
	}

	if !r.Priority.IsValid() {
		return fmt.Errorf("priority must be one of: %s, %s, %s", priority.PriorityHighLabel, priority.PriorityMediumLabel, priority.PriorityLowLabel)
	}

	return nil
}

type CreateNotificationRequest struct {
	Notifications []NotificationRequest `json:"notifications" binding:"required,min=1"`
}

func (r CreateNotificationRequest) Validate() error {
	if len(r.Notifications) == 0 {
		return fmt.Errorf("notifications must contain at least 1 item")
	}

	if len(r.Notifications) > MaxBatchSize {
		return fmt.Errorf("notifications must contain at most %d items", MaxBatchSize)
	}

	for i := range r.Notifications {
		if err := r.Notifications[i].Validate(); err != nil {
			return fmt.Errorf("notifications[%d]: %w", i, err)
		}
	}

	return nil
}
