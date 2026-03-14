package models

import "time"

type OutboxEventType string

const (
	OutboxEventNotificationCreated OutboxEventType = "notification.created"
	OutboxEventNotificationSent    OutboxEventType = "notification.sent"
	OutboxEventNotificationFailed  OutboxEventType = "notification.failed"
)

type OutboxStatus string

const (
	OutboxStatusPending   OutboxStatus = "pending"
	OutboxStatusPublished OutboxStatus = "published"
	OutboxStatusFailed    OutboxStatus = "failed"
	OutboxStatusCancelled OutboxStatus = "cancelled"
)

type OutboxEvent struct {
	ID           int             `json:"id"`
	AggregateID  string          `json:"aggregate_id"`
	EventType    OutboxEventType `json:"event_type"`
	Status       OutboxStatus    `json:"status"`
	CreatedAt    time.Time       `json:"created_at"`
	PublishedAt  *time.Time      `json:"published_at,omitempty"`
	ErrorMessage *string         `json:"error_message,omitempty"`
}

type OutboxEventResponse struct {
	ID           int        `json:"id"`
	AggregateID  string     `json:"aggregate_id"`
	EventType    string     `json:"event_type"`
	Status       string     `json:"status"`
	CreatedAt    time.Time  `json:"created_at"`
	PublishedAt  *time.Time `json:"published_at,omitempty"`
	ErrorMessage *string    `json:"error_message,omitempty"`
}

func (e *OutboxEvent) ToResponse() OutboxEventResponse {
	return OutboxEventResponse{
		ID:           e.ID,
		AggregateID:  e.AggregateID,
		EventType:    string(e.EventType),
		Status:       string(e.Status),
		CreatedAt:    e.CreatedAt,
		PublishedAt:  e.PublishedAt,
		ErrorMessage: e.ErrorMessage,
	}
}
