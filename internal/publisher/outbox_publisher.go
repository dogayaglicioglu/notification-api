package publisher

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"notif-api/internal/db"
	priorityConst "notif-api/internal/domain/priority"
	"notif-api/internal/domain/priorityQueues"
	"notif-api/internal/models"
	"notif-api/internal/models/notification"
	"strconv"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

type NotificationReader interface {
	GetNotificationByID(id int) (*notification.Notification, error)
	UpdateNotificationStatus(id int, status int) error
}

type OutboxPublisher struct {
	redisClient       *redis.Client
	outboxDB          db.OutboxRepository
	notificationStore NotificationReader
	pollInterval      time.Duration
	maxRetries        int
	batchSize         int
	maxConcurrent     int
}

func NewOutboxPublisher(redisAddr, redisPassword string, outboxDB db.OutboxRepository, notificationStore NotificationReader, maxRetries, batchSize, maxConcurrent int) (*OutboxPublisher, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     redisAddr,
		Password: redisPassword,
		DB:       0,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &OutboxPublisher{
		redisClient:       client,
		outboxDB:          outboxDB,
		notificationStore: notificationStore,
		pollInterval:      500 * time.Millisecond,
		maxRetries:        maxRetries,
		batchSize:         batchSize,
		maxConcurrent:     maxConcurrent,
	}, nil
}

func (op *OutboxPublisher) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			slog.Info("outbox publisher context cancelled, stopping")
			return nil
		default:
			op.publishBatch(ctx)
			time.Sleep(op.pollInterval)
		}
	}
}

func (op *OutboxPublisher) publishBatch(ctx context.Context) {
	events, err := op.outboxDB.GetPendingEvents(op.batchSize)
	if err != nil {
		slog.Error("failed to get pending outbox events", "error", err)
		return
	}

	if len(events) == 0 {
		return
	}

	slog.Info("publishing outbox events", "count", len(events))

	sem := make(chan struct{}, op.maxConcurrent)
	var wg sync.WaitGroup

	for i := range events {
		wg.Add(1)
		sem <- struct{}{}

		go func(event models.OutboxEvent) {
			defer wg.Done()
			defer func() { <-sem }()

			op.publishEvent(ctx, event)
		}(events[i])
	}

	wg.Wait()
}

func (op *OutboxPublisher) publishEvent(ctx context.Context, event models.OutboxEvent) error {
	slog.Info("publishing outbox event",
		"event_id", event.ID,
		"event_type", event.EventType,
		"aggregate_id", event.AggregateID,
	)

	notificationID, err := strconv.Atoi(event.AggregateID)
	if err != nil {
		errorMsg := fmt.Sprintf("invalid aggregate_id (notification id): %s", event.AggregateID)
		if markErr := op.outboxDB.MarkEventAsFailed(event.ID, errorMsg); markErr != nil {
			slog.Error("failed to mark event as failed", "event_id", event.ID, "error", markErr)
		}
		return fmt.Errorf("event %d invalid aggregate_id", event.ID)
	}

	notification, err := op.notificationStore.GetNotificationByID(notificationID)
	if err != nil {
		errorMsg := fmt.Sprintf("failed to load notification %d: %v", notificationID, err)
		if markErr := op.outboxDB.MarkEventAsFailed(event.ID, errorMsg); markErr != nil {
			slog.Error("failed to mark event as failed", "event_id", event.ID, "error", markErr)
		}
		if err == sql.ErrNoRows {
			return fmt.Errorf("event %d notification not found", event.ID)
		}
		return fmt.Errorf("event %d notification lookup failed: %w", event.ID, err)
	}

	priority := notification.Priority
	if priority < priorityConst.PriorityLow || priority > priorityConst.PriorityHigh {
		priority = priorityConst.PriorityMedium
	}

	msgBytes, err := json.Marshal(notificationCreatedMessage{
		NotificationID: notificationID,
		Priority:       priority,
	})

	if err != nil {
		errorMsg := fmt.Sprintf("failed to marshal queue payload: %v", err)
		if markErr := op.outboxDB.MarkEventAsFailed(event.ID, errorMsg); markErr != nil {
			slog.Error("failed to mark event as failed", "event_id", event.ID, "error", markErr)
		}
		return fmt.Errorf("event %d marshal failure: %w", event.ID, err)
	}

	queueKey := priorityQueues.MediumKey
	score := float64(-priority)

	switch priority {
	case priorityConst.PriorityHigh:
		queueKey = priorityQueues.HighKey
	case priorityConst.PriorityMedium:
		queueKey = priorityQueues.MediumKey
	case priorityConst.PriorityLow:
		queueKey = priorityQueues.LowKey
	}

	var lastErr error
	for attempt := 0; attempt <= op.maxRetries; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(1<<uint(attempt-1)) * 100 * time.Millisecond
			time.Sleep(backoff)
		}

		lastErr = op.redisClient.ZAdd(ctx, queueKey, redis.Z{
			Score:  score,
			Member: msgBytes,
		}).Err()
		if lastErr == nil {
			break
		}
	}

	if lastErr != nil {
		errorMsg := fmt.Sprintf("failed to enqueue notification %d to %s after %d attempts: %v", notificationID, queueKey, op.maxRetries+1, lastErr)
		if err := op.outboxDB.MarkEventAsFailed(event.ID, errorMsg); err != nil {
			slog.Error("failed to mark event as failed", "event_id", event.ID, "error", err)
		}
		return fmt.Errorf("event %d enqueue failure: %w", event.ID, lastErr)
	}

	if err := op.outboxDB.MarkEventAsPublished(event.ID); err != nil {
		return fmt.Errorf("failed to mark event %d as published: %w", event.ID, err)
	}

	slog.Info("published outbox event", "event_id", event.ID, "queue", queueKey)
	return nil
}

func (op *OutboxPublisher) Close() error {
	slog.Info("shutting down outbox publisher")
	return op.redisClient.Close()
}

type notificationCreatedMessage struct {
	NotificationID int `json:"notificationId"`
	Priority       int `json:"priority"`
}
