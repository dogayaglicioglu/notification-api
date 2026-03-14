package notification

import "time"

type NotificationResponse struct {
	ID          int        `json:"id"`
	Recipient   string     `json:"recipient"`
	Channel     string     `json:"channel"`
	Content     string     `json:"content"`
	Priority    string     `json:"priority"`
	Status      string     `json:"status"`
	BatchID     string     `json:"batchId"`
	CreatedAt   time.Time  `json:"createdAt"`
	CancelledAt *time.Time `json:"cancelledAt,omitempty"`
}

type BatchCreateResponse struct {
	BatchID   string `json:"batchId"`
	Count     int    `json:"count"`
	CreatedAt string `json:"createdAt"`
}

type CancelPendingResponse struct {
	CancelledCount int64 `json:"cancelledCount"`
}

type ListResponse struct {
	Data       []NotificationResponse `json:"data"`
	Total      int                    `json:"total"`
	Page       int                    `json:"page"`
	PageSize   int                    `json:"pageSize"`
	TotalPages int                    `json:"totalPages"`
}

// ErrorResponse is a standard API error payload.
type ErrorResponse struct {
	Error string `json:"error"`
}
