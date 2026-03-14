package notification

import (
	"notif-api/internal/domain/priority"
	"notif-api/internal/domain/status"
	"time"
)

type Notification struct {
	ID          int        `json:"id" db:"id"`
	Recipient   string     `json:"recipient" db:"recipient"`
	Channel     string     `json:"channel" db:"channel"`
	Content     string     `json:"content" db:"content"`
	Priority    int        `json:"priority" db:"priority"`
	Status      int        `json:"status" db:"status"`
	BatchID     string     `json:"batchId" db:"batchId"`
	CreatedAt   time.Time  `json:"createdAt" db:"date"`
	CancelledAt *time.Time `json:"cancelledAt,omitempty" db:"cancelled_at"`
}

func (n *Notification) ToResponse() NotificationResponse {
	return NotificationResponse{
		ID:          n.ID,
		Recipient:   n.Recipient,
		Channel:     n.Channel,
		Content:     n.Content,
		Priority:    priority.PriorityIntToString(n.Priority),
		Status:      status.StatusIntToString(n.Status),
		BatchID:     n.BatchID,
		CreatedAt:   n.CreatedAt,
		CancelledAt: n.CancelledAt,
	}
}

type ListFilter struct {
	Status   *int
	Channel  *string
	DateFrom *time.Time
	DateTo   *time.Time
	Page     int
	PageSize int
}
