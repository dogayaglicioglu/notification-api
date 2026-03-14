package publisher

import (
	"context"
	"database/sql"
	"errors"
	"notif-api/internal/models"
	"notif-api/internal/models/notification"
	"sync"
	"testing"
)

type mockOutboxRepo struct {
	mu             sync.Mutex
	pending        []models.OutboxEvent
	pendingErr     error
	pendingLimit   int
	failedCalls    []failedCall
	publishedCalls []int
}

type failedCall struct {
	eventID int
	message string
}

func (f *mockOutboxRepo) CreateOutboxEvent(_ string, _ models.OutboxEventType) (*models.OutboxEvent, error) {
	return nil, errors.New("not implemented")
}

func (f *mockOutboxRepo) GetPendingEvents(limit int) ([]models.OutboxEvent, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.pendingLimit = limit
	if f.pendingErr != nil {
		return nil, f.pendingErr
	}
	return append([]models.OutboxEvent(nil), f.pending...), nil
}

func (f *mockOutboxRepo) MarkEventAsPublished(eventID int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.publishedCalls = append(f.publishedCalls, eventID)
	return nil
}

func (f *mockOutboxRepo) MarkEventAsFailed(eventID int, errorMessage string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.failedCalls = append(f.failedCalls, failedCall{eventID: eventID, message: errorMessage})
	return nil
}

func (f *mockOutboxRepo) GetEventByID(_ int) (*models.OutboxEvent, error) {
	return nil, errors.New("not implemented")
}

func (f *mockOutboxRepo) DeletePublishedEvents(_ int) error {
	return nil
}

type fakeNotificationReader struct {
	notification *notification.Notification
	getErr       error
}

func (f *fakeNotificationReader) GetNotificationByID(_ int) (*notification.Notification, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.notification == nil {
		return nil, sql.ErrNoRows
	}
	return f.notification, nil
}

func (f *fakeNotificationReader) UpdateNotificationStatus(_ int, _ int) error {
	return nil
}

func TestPublishEvent_InvalidAggregateID_MarksFailed(t *testing.T) {
	repo := &mockOutboxRepo{}
	op := &OutboxPublisher{
		outboxDB:          repo,
		notificationStore: &fakeNotificationReader{},
		maxRetries:        0,
	}

	err := op.publishEvent(context.Background(), models.OutboxEvent{
		ID:          7,
		AggregateID: "not-an-int",
		EventType:   models.OutboxEventNotificationCreated,
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if len(repo.failedCalls) != 1 {
		t.Fatalf("expected 1 failed mark call, got %d", len(repo.failedCalls))
	}
	if repo.failedCalls[0].eventID != 7 {
		t.Fatalf("expected failed event id 7, got %d", repo.failedCalls[0].eventID)
	}
	if repo.failedCalls[0].message == "" {
		t.Fatal("expected failure message to be set")
	}
	if len(repo.publishedCalls) != 0 {
		t.Fatalf("expected no publish marks, got %d", len(repo.publishedCalls))
	}
}

func TestPublishEvent_NotificationNotFound_MarksFailed(t *testing.T) {
	repo := &mockOutboxRepo{}
	op := &OutboxPublisher{
		outboxDB:          repo,
		notificationStore: &fakeNotificationReader{getErr: sql.ErrNoRows},
		maxRetries:        0,
	}

	err := op.publishEvent(context.Background(), models.OutboxEvent{
		ID:          11,
		AggregateID: "42",
		EventType:   models.OutboxEventNotificationCreated,
	})

	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if len(repo.failedCalls) != 1 {
		t.Fatalf("expected 1 failed mark call, got %d", len(repo.failedCalls))
	}
	if repo.failedCalls[0].eventID != 11 {
		t.Fatalf("expected failed event id 11, got %d", repo.failedCalls[0].eventID)
	}
	if len(repo.publishedCalls) != 0 {
		t.Fatalf("expected no publish marks, got %d", len(repo.publishedCalls))
	}
}

func TestPublishBatch_UsesConfiguredLimit(t *testing.T) {
	repo := &mockOutboxRepo{pendingErr: errors.New("db unavailable")}
	op := &OutboxPublisher{
		outboxDB:          repo,
		notificationStore: &fakeNotificationReader{},
		batchSize:         25,
		maxConcurrent:     2,
	}

	op.publishBatch(context.Background())

	if repo.pendingLimit != 25 {
		t.Fatalf("expected pending limit 25, got %d", repo.pendingLimit)
	}
}

func TestPublishBatch_InvalidEvents_AreMarkedFailed(t *testing.T) {
	repo := &mockOutboxRepo{
		pending: []models.OutboxEvent{
			{ID: 1, AggregateID: "x", EventType: models.OutboxEventNotificationCreated},
			{ID: 2, AggregateID: "y", EventType: models.OutboxEventNotificationCreated},
		},
	}
	op := &OutboxPublisher{
		outboxDB:          repo,
		notificationStore: &fakeNotificationReader{},
		batchSize:         10,
		maxConcurrent:     1,
	}

	op.publishBatch(context.Background())

	if len(repo.failedCalls) != 2 {
		t.Fatalf("expected 2 failed marks, got %d", len(repo.failedCalls))
	}
	if len(repo.publishedCalls) != 0 {
		t.Fatalf("expected 0 publish marks, got %d", len(repo.publishedCalls))
	}
}
