package worker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"notif-api/internal/domain/priority"
	"notif-api/internal/domain/status"
	"notif-api/internal/metrics"
	"notif-api/internal/models/notification"
	"notif-api/internal/provider"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/time/rate"
)

const (
	QueueKeyHigh   = "notifications:queue:high"
	QueueKeyMedium = "notifications:queue:medium"
	QueueKeyLow    = "notifications:queue:low"
)

type NotificationStore interface {
	GetNotificationByID(id int) (*notification.Notification, error)
	UpdateNotificationStatus(id int, status int) error
}

type notificationMessage struct {
	NotificationID int `json:"notificationId"`
	Priority       int `json:"priority"`
}

type RedisQueueWorker struct {
	store           NotificationStore
	redisClient     *redis.Client
	notifProvider   provider.NotificationProvider
	maxConcurrent   int
	maxRetry        int
	wg              sync.WaitGroup
	pollInterval    time.Duration
	queueMutex      sync.Mutex
	channelLimiters map[string]*rate.Limiter
	limiterMu       sync.Mutex
}

func NewRedisQueueWorker(redisAddr, redisPassword string, store NotificationStore, notifProvider provider.NotificationProvider, concurrency, maxRetry int) (*RedisQueueWorker, error) {
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

	return &RedisQueueWorker{
		store:         store,
		redisClient:   client,
		notifProvider: notifProvider,
		maxConcurrent: concurrency,
		maxRetry:      maxRetry,
		pollInterval:  100 * time.Millisecond,
		channelLimiters: map[string]*rate.Limiter{
			"email": rate.NewLimiter(rate.Limit(100), 100),
			"sms":   rate.NewLimiter(rate.Limit(100), 100),
			"push":  rate.NewLimiter(rate.Limit(100), 100),
		},
	}, nil
}

func (w *RedisQueueWorker) Run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			slog.Info("context cancelled, stopping queue worker")
			return nil
		default:
			switch {
			case w.processQueueCompletely(ctx, QueueKeyHigh, "HIGH"):
			case w.processQueueCompletely(ctx, QueueKeyMedium, "MEDIUM"):
			case w.processQueueCompletely(ctx, QueueKeyLow, "LOW"):
			default:
				time.Sleep(w.pollInterval)
			}
		}
	}
}

func (w *RedisQueueWorker) processQueueCompletely(ctx context.Context, queueKey string, queueName string) bool {
	w.queueMutex.Lock()

	size, err := w.redisClient.ZCard(ctx, queueKey).Result()
	if err != nil || size == 0 {
		w.queueMutex.Unlock()
		return false
	}

	slog.Info("processing queue",
		"queue", queueName,
		"size", size,
		"concurrency", w.maxConcurrent,
	)
	w.queueMutex.Unlock()

	sem := make(chan struct{}, w.maxConcurrent)
	var wg sync.WaitGroup
	processed := 0

	for {
		w.queueMutex.Lock()
		msg := w.popFromQueueUnlocked(ctx, queueKey, queueName)
		w.queueMutex.Unlock()

		if msg == nil {
			break
		}

		processed++
		wg.Add(1)
		sem <- struct{}{} // acq. semaphore

		go func(notifID int) {
			defer wg.Done()
			defer func() { <-sem }() // release semaphore

			if err := w.processWithRetry(notifID); err != nil {
				slog.Warn("failed to process notification", "notification_id", notifID, "error", err)
			}
		}(msg.NotificationID)
	}

	wg.Wait()
	slog.Info("completed queue processing", "queue", queueName, "processed", processed)
	return processed > 0
}

func zMemberToString(member interface{}) (string, error) {
	switch v := member.(type) {
	case string:
		return v, nil
	case []byte:
		return string(v), nil
	default:
		return "", fmt.Errorf("unsupported member type: %T", member)
	}
}

func (w *RedisQueueWorker) popFromQueueUnlocked(ctx context.Context, queueKey string, queueName string) *notificationMessage {
	result, err := w.redisClient.ZPopMin(ctx, queueKey, 1).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			slog.Warn("error popping from queue", "queue", queueName, "error", err)
		}
		return nil
	}

	if len(result) == 0 {
		return nil
	}

	var msg notificationMessage
	msgStr, err := zMemberToString(result[0].Member)
	if err != nil {
		slog.Warn("invalid message type in queue", "queue", queueName, "error", err)
		return nil
	}

	if err := json.Unmarshal([]byte(msgStr), &msg); err != nil {
		slog.Warn("failed to unmarshal message", "queue", queueName, "error", err)
		return nil
	}

	size, _ := w.redisClient.ZCard(ctx, queueKey).Result()
	metrics.QueueSize.Set(float64(size))

	return &msg
}

func (w *RedisQueueWorker) processWithRetry(notificationID int) error {
	var lastErr error

	notif, err := w.store.GetNotificationByID(notificationID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			slog.Warn("notification not found, skipping", "notification_id", notificationID)
			return nil
		}
		return fmt.Errorf("failed to fetch notification: %w", err)
	}

	if notif.Status != int(status.StatusPending) {
		statusName := status.Status(notif.Status).String()
		slog.Info("skipping non-pending notification",
			"notification_id", notificationID,
			"status", statusName,
		)
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	limiter := w.getChannelLimiter(notif.Channel)
	if err := limiter.Wait(ctx); err != nil {
		return fmt.Errorf("rate limiter wait failed for notification %d (channel=%s): %w", notificationID, notif.Channel, err)
	}

	if err := w.store.UpdateNotificationStatus(notificationID, int(status.StatusProcessing)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			slog.Warn("notification disappeared while setting processing, skipping", "notification_id", notificationID)
			return nil
		}
		return fmt.Errorf("failed to set processing status: %w", err)
	}

	priorityName := priority.PriorityIntToString(notif.Priority)
	slog.Info("processing notification",
		"notification_id", notificationID,
		"channel", notif.Channel,
		"recipient", notif.Recipient,
		"priority", priorityName,
	)

	for attempt := 0; attempt <= w.maxRetry; attempt++ {
		if attempt > 0 {
			metrics.NotificationRetries.Inc()
			backoff := time.Duration(1<<uint(attempt-1)) * time.Second
			slog.Warn("retrying notification",
				"notification_id", notificationID,
				"attempt", attempt+1,
				"max_attempts", w.maxRetry+1,
				"backoff", backoff.String(),
			)
			time.Sleep(backoff)
		}

		processed, err := w.processNotification(ctx, notif)
		if err != nil {
			lastErr = err
			continue
		}

		if processed {
			metrics.NotificationsProcessed.WithLabelValues("success").Inc()
		}

		return nil
	}

	metrics.NotificationsProcessed.WithLabelValues("failure").Inc()
	if err := w.store.UpdateNotificationStatus(notificationID, int(status.StatusFailure)); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("failed after %d attempts: %w (also failed to set failure status: %v)", w.maxRetry+1, lastErr, err)
	}

	return fmt.Errorf("failed after %d attempts: %w", w.maxRetry+1, lastErr)
}

func (w *RedisQueueWorker) processNotification(ctx context.Context, notif *notification.Notification) (bool, error) {
	start := time.Now()
	defer func() {
		duration := time.Since(start).Seconds()
		metrics.NotificationProcessingDuration.Observe(duration)
	}()

	metrics.ActiveWorkers.Inc()
	defer metrics.ActiveWorkers.Dec()

	provReq := provider.ProviderRequest{
		To:      notif.Recipient,
		Channel: notif.Channel,
		Content: notif.Content,
	}


	if _, err := w.notifProvider.Send(ctx, provReq); err != nil {
		return false, fmt.Errorf("external provider call failed for notification %d: %w", notif.ID, err)
	}

	if err := w.store.UpdateNotificationStatus(notif.ID, int(status.StatusSuccess)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			slog.Warn("notification disappeared while setting success/failure, skipping", "notification_id", notif.ID)
			return false, nil
		}
		return false, fmt.Errorf("failed to set success status: %w", err)
	}

	slog.Info("notification processed successfully", "notification_id", notif.ID)
	return true, nil
}

func (w *RedisQueueWorker) Close() error {
	slog.Info("shutting down queue worker")
	w.wg.Wait()
	return w.redisClient.Close()
}

func (w *RedisQueueWorker) getChannelLimiter(channel string) *rate.Limiter {
	w.limiterMu.Lock()
	defer w.limiterMu.Unlock()
	if w.channelLimiters == nil {
		w.channelLimiters = make(map[string]*rate.Limiter)
	}

	if l, ok := w.channelLimiters[channel]; ok {
		return l
	}

	l := rate.NewLimiter(rate.Limit(100), 100)
	w.channelLimiters[channel] = l
	return l
}
