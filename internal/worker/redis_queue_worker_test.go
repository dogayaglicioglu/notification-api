package worker

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"notif-api/internal/domain/priority"
	"notif-api/internal/domain/status"
	"notif-api/internal/models/notification"
	"notif-api/internal/provider"
	"reflect"
	"testing"
)

type mockProvider struct {
	err error
}

func (m *mockProvider) Send(_ context.Context, _ provider.ProviderRequest) (*provider.ProviderResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &provider.ProviderResponse{MessageID: "test-id", Status: "accepted"}, nil
}

type fakeStore struct {
	notification *notification.Notification
	getErr       error
	updateErr    map[string]error
	updates      map[int][]int
}

func (f *fakeStore) GetNotificationByID(_ int) (*notification.Notification, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	if f.notification == nil {
		return nil, sql.ErrNoRows
	}
	return f.notification, nil
}

func (f *fakeStore) UpdateNotificationStatus(id int, status int) error {
	if f.updateErr != nil {
		if err, ok := f.updateErr[fmt.Sprintf("%d:%d", id, status)]; ok {
			return err
		}
	}
	if f.updates == nil {
		f.updates = make(map[int][]int)
	}
	f.updates[id] = append(f.updates[id], status)
	return nil
}

func TestProcessNotification_SetsProcessingThenSuccess(t *testing.T) {
	store := &fakeStore{
		notification: &notification.Notification{ID: 10, Channel: "email", Recipient: "user@example.com", Priority: priority.PriorityHigh, Status: int(status.StatusPending)},
	}
	worker := &RedisQueueWorker{store: store, notifProvider: &mockProvider{}}

	processed, err := worker.processNotification(10)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !processed {
		t.Fatal("expected notification to be processed")
	}

	want := []int{int(status.StatusProcessing), int(status.StatusSuccess)}
	if got := store.updates[10]; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected status flow %v, got %v", want, got)
	}
}

func TestProcessNotification_NotFoundWhileSettingProcessingIsIgnored(t *testing.T) {
	store := &fakeStore{updateErr: map[string]error{"99:150": sql.ErrNoRows}}
	worker := &RedisQueueWorker{store: store, notifProvider: &mockProvider{}}

	processed, err := worker.processNotification(99)
	if err != nil {
		t.Fatalf("expected nil error for missing notification, got %v", err)
	}
	if processed {
		t.Fatal("expected missing notification to be skipped")
	}
}

func TestProcessNotification_DBErrorMarksFailure(t *testing.T) {
	store := &fakeStore{
		getErr: errors.New("db down"),
	}
	worker := &RedisQueueWorker{store: store, notifProvider: &mockProvider{}}

	if _, err := worker.processNotification(55); err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestProcessNotification_SuccessUpdateFailureMarksFailure(t *testing.T) {
	store := &fakeStore{
		notification: &notification.Notification{ID: 77, Channel: "push", Recipient: "user-77", Priority: priority.PriorityMedium, Status: int(status.StatusPending)},
		updateErr:    map[string]error{"77:200": errors.New("write failed")},
	}
	worker := &RedisQueueWorker{store: store, notifProvider: &mockProvider{}}

	if _, err := worker.processNotification(77); err == nil {
		t.Fatal("expected error, got nil")
	}

	want := []int{int(status.StatusProcessing), int(status.StatusFailure)}
	if got := store.updates[77]; !reflect.DeepEqual(got, want) {
		t.Fatalf("expected status flow %v, got %v", want, got)
	}
}
