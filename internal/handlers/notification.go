package handlers

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"notif-api/internal/db"
	"notif-api/internal/domain/messageChannel"
	"notif-api/internal/domain/priority"
	"notif-api/internal/domain/status"
	"notif-api/internal/models/notification"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type NotificationPublisher interface {
	PublishNotificationCreated(notificationID int) error
	PublishNotificationCreatedWithPriority(notificationID int, priority int) error
}

type NotificationHandler struct {
	db        db.NotificationRepository
	publisher NotificationPublisher
	outboxDB  db.OutboxRepository
}

func NewNotificationHandlerWithOutbox(notificationDB db.NotificationRepository, publisher NotificationPublisher, outboxDB db.OutboxRepository) *NotificationHandler {
	return &NotificationHandler{db: notificationDB, publisher: publisher, outboxDB: outboxDB}
}

// CreateBatch godoc
// @Summary Create batch notifications
// @Description Creates up to 1000 notifications in a single request.
// @Tags notifications
// @Accept json
// @Produce json
// @Param X-Correlation-ID header string false "Correlation ID"
// @Param request body notification.CreateNotificationRequest true "Batch request"
// @Success 201 {object} notification.BatchCreateResponse
// @Failure 400 {object} notification.ErrorResponse
// @Failure 500 {object} notification.ErrorResponse
// @Router /notifications/batch [post]
func (nh *NotificationHandler) CreateBatch(c *gin.Context) {
	var request notification.CreateNotificationRequest

	if err := c.ShouldBindJSON(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := request.Validate(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	notifications := make([]notification.Notification, len(request.Notifications))
	for i, n := range request.Notifications {
		notifications[i] = notification.Notification{
			Recipient: n.Recipient,
			Channel:   string(n.Channel),
			Content:   n.Content,
			Priority:  priority.PriorityStringToInt(n.Priority),
			Status:    int(status.StatusPending),
		}
	}

	batchID, created, err := nh.db.CreateNotificationsBatch(notifications)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	response := notification.BatchCreateResponse{
		BatchID:   batchID,
		Count:     len(created),
		CreatedAt: time.Now().Format(time.RFC3339),
	}

	slog.Info("created notification batch",
		"correlation_id", correlationID(c),
		"batch_id", batchID,
		"count", len(created),
	)

	c.JSON(http.StatusCreated, response)
}

// GetByBatchID godoc
// @Summary Get notifications by batch ID
// @Tags notifications
// @Produce json
// @Param batchId path string true "Batch ID"
// @Success 200 {array} notification.NotificationResponse
// @Failure 400 {object} notification.ErrorResponse
// @Failure 404 {object} notification.ErrorResponse
// @Failure 500 {object} notification.ErrorResponse
// @Router /notifications/batch/{batchId} [get]
func (nh *NotificationHandler) GetByBatchID(c *gin.Context) {
	batchID := c.Param("batchId")

	if batchID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid batchId"})
		return
	}

	notifications, err := nh.db.GetNotificationsByBatchID(batchID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if len(notifications) == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "No notifications found for this batchId"})
		return
	}

	response := make([]notification.NotificationResponse, len(notifications))
	for i, notif := range notifications {
		response[i] = notif.ToResponse()
	}

	c.JSON(http.StatusOK, response)
}

// GetByID godoc
// @Summary Get notification by ID
// @Tags notifications
// @Produce json
// @Param id path int true "Notification ID"
// @Success 200 {object} notification.NotificationResponse
// @Failure 400 {object} notification.ErrorResponse
// @Failure 404 {object} notification.ErrorResponse
// @Failure 500 {object} notification.ErrorResponse
// @Router /notifications/{id} [get]
func (nh *NotificationHandler) GetByID(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid notification id"})
		return
	}

	notif, err := nh.db.GetNotificationByID(id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			c.JSON(http.StatusNotFound, gin.H{"error": "notification not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, notif.ToResponse())
}

// Cancel godoc
// @Summary Cancel all pending notifications
// @Description Cancels every notification currently in pending status.
// @Tags notifications
// @Produce json
// @Success 200 {object} notification.CancelPendingResponse
// @Failure 500 {object} notification.ErrorResponse
// @Router /notifications/cancel [patch]
func (nh *NotificationHandler) Cancel(c *gin.Context) {
	cancelledCount, err := nh.db.CancelAllPendingNotifications()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	slog.Info("cancelled pending notifications",
		"correlation_id", correlationID(c),
		"cancelled_count", cancelledCount,
	)
	c.JSON(http.StatusOK, notification.CancelPendingResponse{CancelledCount: cancelledCount})
}

// List godoc
// @Summary List notifications
// @Tags notifications
// @Produce json
// @Param status query string false "Status (pending|processing|success|failure|cancelled)"
// @Param channel query string false "Channel (email|sms|push)"
// @Param start query string false "RFC3339 start time"
// @Param end query string false "RFC3339 end time"
// @Param page query int false "Page number" default(1)
// @Param page_size query int false "Page size" default(20)
// @Success 200 {object} notification.ListResponse
// @Failure 400 {object} notification.ErrorResponse
// @Failure 500 {object} notification.ErrorResponse
// @Router /notifications [get]
func (nh *NotificationHandler) List(c *gin.Context) {
	filter := notification.ListFilter{
		Page:     1,
		PageSize: 20,
	}

	if pageStr := c.Query("page"); pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			filter.Page = p
		}
	}
	if psStr := c.Query("page_size"); psStr != "" {
		if ps, err := strconv.Atoi(psStr); err == nil && ps > 0 && ps <= 100 {
			filter.PageSize = ps
		}
	}
	if s := c.Query("status"); s != "" {
		parsed, ok := status.StatusFromString(s)
		if !ok {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid status, must be one of: pending, processing, success, failure, cancelled"})
			return
		}
		filter.Status = &parsed
	}
	if ch := c.Query("channel"); ch != "" {
		chVal := messageChannel.Channel(ch)
		if !chVal.IsValid() {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel"})
			return
		}
		chStr := string(chVal)
		filter.Channel = &chStr
	}
	if df := c.Query("start"); df != "" {
		t, err := time.Parse(time.RFC3339, df)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "start must be RFC3339 format"})
			return
		}
		filter.DateFrom = &t
	}
	if dt := c.Query("end"); dt != "" {
		t, err := time.Parse(time.RFC3339, dt)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "end must be RFC3339 format"})
			return
		}
		filter.DateTo = &t
	}

	notifications, total, err := nh.db.ListNotifications(filter)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	data := make([]notification.NotificationResponse, len(notifications))
	for i, n := range notifications {
		data[i] = n.ToResponse()
	}

	totalPages := (total + filter.PageSize - 1) / filter.PageSize

	c.JSON(http.StatusOK, notification.ListResponse{
		Data:       data,
		Total:      total,
		Page:       filter.Page,
		PageSize:   filter.PageSize,
		TotalPages: totalPages,
	})
}

func correlationID(c *gin.Context) string {
	if id, exists := c.Get("correlation_id"); exists {
		if s, ok := id.(string); ok {
			return s
		}
	}
	return "-"
}
