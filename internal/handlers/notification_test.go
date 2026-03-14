package handlers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"notif-api/internal/db"
	channel "notif-api/internal/domain/messageChannel"
	"notif-api/internal/domain/priority"
	"notif-api/internal/domain/status"
	"notif-api/internal/models/notification"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type MockNotificationDB struct {
	mock.Mock
}

func (m *MockNotificationDB) CreateNotificationsBatch(notifications []notification.Notification) (string, []notification.Notification, error) {
	args := m.Called(notifications)
	if args.Get(1) == nil {
		return args.String(0), nil, args.Error(2)
	}
	return args.String(0), args.Get(1).([]notification.Notification), args.Error(2)
}

func (m *MockNotificationDB) GetNotificationsByBatchID(batchID string) ([]notification.Notification, error) {
	args := m.Called(batchID)
	return args.Get(0).([]notification.Notification), args.Error(1)
}

func (m *MockNotificationDB) GetNotificationByID(id int) (*notification.Notification, error) {
	args := m.Called(id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*notification.Notification), args.Error(1)
}

func (m *MockNotificationDB) UpdateNotificationStatus(id int, st int) error {
	args := m.Called(id, st)
	return args.Error(0)
}

func (m *MockNotificationDB) ListNotifications(filter notification.ListFilter) ([]notification.Notification, int, error) {
	args := m.Called(filter)
	if args.Get(0) == nil {
		return nil, args.Int(1), args.Error(2)
	}
	return args.Get(0).([]notification.Notification), args.Int(1), args.Error(2)
}

func (m *MockNotificationDB) CancelAllPendingNotifications() (int64, error) {
	args := m.Called()
	return args.Get(0).(int64), args.Error(1)
}

// fakeCancelableNotificationDB simulates DB state transitions for cancellation tests.
type fakeCancelableNotificationDB struct {
	notifications []notification.Notification
	err           error
}

func (f *fakeCancelableNotificationDB) CreateNotificationsBatch([]notification.Notification) (string, []notification.Notification, error) {
	return "", nil, nil
}

func (f *fakeCancelableNotificationDB) GetNotificationsByBatchID(string) ([]notification.Notification, error) {
	return nil, nil
}

func (f *fakeCancelableNotificationDB) GetNotificationByID(int) (*notification.Notification, error) {
	return nil, nil
}

func (f *fakeCancelableNotificationDB) UpdateNotificationStatus(int, int) error {
	return nil
}

func (f *fakeCancelableNotificationDB) ListNotifications(notification.ListFilter) ([]notification.Notification, int, error) {
	return nil, 0, nil
}

func (f *fakeCancelableNotificationDB) CancelAllPendingNotifications() (int64, error) {
	if f.err != nil {
		return 0, f.err
	}

	var cancelledCount int64
	for i := range f.notifications {
		if f.notifications[i].Status == int(status.StatusPending) {
			f.notifications[i].Status = int(status.StatusCancelled)
			cancelledCount++
		}
	}
	return cancelledCount, nil
}

var _ db.NotificationRepository = (*MockNotificationDB)(nil)
var _ db.NotificationRepository = (*fakeCancelableNotificationDB)(nil)

func TestCreateBatch_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockDB := new(MockNotificationDB)

	handler := &NotificationHandler{db: mockDB}

	requestBody := notification.CreateNotificationRequest{
		Notifications: []notification.NotificationRequest{
			{
				Recipient: "user@example.com",
				Channel:   channel.ChannelEmail,
				Content:   "Test notification message",
				Priority:  priority.PriorityHighLabel,
			},
			{
				Recipient: "+1234567890",
				Channel:   channel.ChannelSMS,
				Content:   "Another test message",
				Priority:  priority.PriorityMediumLabel,
			},
		},
	}

	expectedBatchID := "batch-123"
	createdNotifications := []notification.Notification{
		{ID: 1, Recipient: "user@example.com", Channel: "email", Content: "Test notification message", Priority: priority.PriorityHigh, Status: int(status.StatusPending), BatchID: expectedBatchID},
		{ID: 2, Recipient: "+1234567890", Channel: "sms", Content: "Another test message", Priority: priority.PriorityMedium, Status: int(status.StatusPending), BatchID: expectedBatchID},
	}

	mockDB.On("CreateNotificationsBatch", mock.MatchedBy(func(notifications []notification.Notification) bool {
		if len(notifications) != 2 {
			return false
		}
		if notifications[0].Recipient != "user@example.com" ||
			notifications[0].Channel != "email" ||
			notifications[0].Content != "Test notification message" ||
			notifications[0].Priority != priority.PriorityHigh ||
			notifications[0].Status != int(status.StatusPending) {
			return false
		}

		if notifications[1].Recipient != "+1234567890" ||
			notifications[1].Channel != "sms" ||
			notifications[1].Content != "Another test message" ||
			notifications[1].Priority != priority.PriorityMedium ||
			notifications[1].Status != int(status.StatusPending) {
			return false
		}
		return true
	})).Return(expectedBatchID, createdNotifications, nil)

	jsonBody, err := json.Marshal(requestBody)
	assert.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "/notifications/batch", bytes.NewBuffer(jsonBody))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()

	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.CreateBatch(c)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response notification.BatchCreateResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Equal(t, expectedBatchID, response.BatchID)
	assert.Equal(t, 2, response.Count)
	assert.NotEmpty(t, response.CreatedAt)

	_, err = time.Parse(time.RFC3339, response.CreatedAt)
	assert.NoError(t, err, "CreatedAt should be in RFC3339 format")

	mockDB.AssertExpectations(t)
}

func TestCreateBatch_EmptyNotifications(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockDB := new(MockNotificationDB)
	handler := &NotificationHandler{db: mockDB}

	requestBody := notification.CreateNotificationRequest{
		Notifications: []notification.NotificationRequest{},
	}

	jsonBody, err := json.Marshal(requestBody)
	assert.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "/notifications/batch", bytes.NewBuffer(jsonBody))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.CreateBatch(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)

	assert.Contains(t, response["error"], "Notifications")
}

func TestCreateBatch_InvalidChannel(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockDB := new(MockNotificationDB)
	handler := &NotificationHandler{db: mockDB}

	requestJSON := `{
		"notifications": [
			{
				"recipient": "user@example.com",
				"channel": "invalid_channel",
				"content": "Test message",
				"priority": "high"
			}
		]
	}`

	req, err := http.NewRequest(http.MethodPost, "/notifications/batch", bytes.NewBufferString(requestJSON))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.CreateBatch(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response["error"], "channel must be one of")
}

func TestCreateBatch_InvalidPriority(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockDB := new(MockNotificationDB)
	handler := &NotificationHandler{db: mockDB}

	requestJSON := `{
		"notifications": [
			{
				"recipient": "user@example.com",
				"channel": "email",
				"content": "Test message",
				"priority": "urgent"
			}
		]
	}`

	req, err := http.NewRequest(http.MethodPost, "/notifications/batch", bytes.NewBufferString(requestJSON))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.CreateBatch(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response["error"], "priority must be one of")
}

func TestCreateBatch_ContentTooLong(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockDB := new(MockNotificationDB)
	handler := &NotificationHandler{db: mockDB}

	longContent := string(make([]byte, 1001))
	for i := range longContent {
		longContent = longContent[:i] + "a" + longContent[i+1:]
	}

	requestBody := notification.CreateNotificationRequest{
		Notifications: []notification.NotificationRequest{
			{
				Recipient: "user@example.com",
				Channel:   channel.ChannelEmail,
				Content:   longContent,
				Priority:  priority.PriorityHighLabel,
			},
		},
	}

	jsonBody, err := json.Marshal(requestBody)
	assert.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "/notifications/batch", bytes.NewBuffer(jsonBody))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.CreateBatch(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Contains(t, response["error"], "content must be at most 1000 characters")
}

func TestCreateBatch_DatabaseError(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockDB := new(MockNotificationDB)
	handler := &NotificationHandler{db: mockDB}

	requestBody := notification.CreateNotificationRequest{
		Notifications: []notification.NotificationRequest{
			{
				Recipient: "user@example.com",
				Channel:   channel.ChannelEmail,
				Content:   "Test notification message",
				Priority:  priority.PriorityHighLabel,
			},
		},
	}

	mockDB.On("CreateNotificationsBatch", mock.Anything).Return("", nil, assert.AnError)

	jsonBody, err := json.Marshal(requestBody)
	assert.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "/notifications/batch", bytes.NewBuffer(jsonBody))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.CreateBatch(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.NotNil(t, response["error"])

	mockDB.AssertExpectations(t)
}

func TestCreateBatch_LargeBatch(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mockDB := new(MockNotificationDB)
	handler := &NotificationHandler{db: mockDB}

	notifications := make([]notification.NotificationRequest, 100)
	for i := 0; i < 100; i++ {
		notifications[i] = notification.NotificationRequest{
			Recipient: fmt.Sprintf("user%d@example.com", i),
			Channel:   channel.ChannelEmail,
			Content:   fmt.Sprintf("Test message %d", i),
			Priority:  priority.PriorityMediumLabel,
		}
	}

	requestBody := notification.CreateNotificationRequest{
		Notifications: notifications,
	}

	expectedBatchID := "batch-999"
	createdNotifications := make([]notification.Notification, 100)
	for i := 0; i < 100; i++ {
		createdNotifications[i] = notification.Notification{ID: i + 1, BatchID: expectedBatchID}
	}

	mockDB.On("CreateNotificationsBatch", mock.MatchedBy(func(notifications []notification.Notification) bool {
		return len(notifications) == 100
	})).Return(expectedBatchID, createdNotifications, nil)

	jsonBody, err := json.Marshal(requestBody)
	assert.NoError(t, err)

	req, err := http.NewRequest(http.MethodPost, "/notifications/batch", bytes.NewBuffer(jsonBody))
	assert.NoError(t, err)
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.CreateBatch(c)

	assert.Equal(t, http.StatusCreated, w.Code)

	var response notification.BatchCreateResponse
	err = json.Unmarshal(w.Body.Bytes(), &response)
	assert.NoError(t, err)
	assert.Equal(t, expectedBatchID, response.BatchID)
	assert.Equal(t, 100, response.Count)

	mockDB.AssertExpectations(t)
}

func TestGetByID_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockDB := new(MockNotificationDB)
	handler := &NotificationHandler{db: mockDB}

	notif := &notification.Notification{ID: 42, Recipient: "u@example.com", Channel: "email", Content: "x", Priority: priority.PriorityHigh, Status: int(status.StatusPending), BatchID: "b-1"}
	mockDB.On("GetNotificationByID", 42).Return(notif, nil)

	req, _ := http.NewRequest(http.MethodGet, "/notifications/42", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "id", Value: "42"}}

	handler.GetByID(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockDB.AssertExpectations(t)
}

func TestCancel_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)

	fakeDB := &fakeCancelableNotificationDB{
		notifications: []notification.Notification{
			{ID: 1, Status: int(status.StatusPending)},
			{ID: 2, Status: int(status.StatusPending)},
			{ID: 3, Status: int(status.StatusProcessing)},
		},
	}
	handler := &NotificationHandler{db: fakeDB}

	req, _ := http.NewRequest(http.MethodPatch, "/notifications/cancel", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.Cancel(c)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "\"cancelledCount\":2")

	assert.Equal(t, int(status.StatusCancelled), fakeDB.notifications[0].Status)
	assert.Equal(t, int(status.StatusCancelled), fakeDB.notifications[1].Status)
	assert.Equal(t, int(status.StatusProcessing), fakeDB.notifications[2].Status)
}

func TestCancel_DatabaseError(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockDB := new(MockNotificationDB)
	handler := &NotificationHandler{db: mockDB}

	mockDB.On("CancelAllPendingNotifications").Return(int64(0), assert.AnError)

	req, _ := http.NewRequest(http.MethodPatch, "/notifications/cancel", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.Cancel(c)

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "error")
	mockDB.AssertExpectations(t)
}

func TestList_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockDB := new(MockNotificationDB)
	handler := &NotificationHandler{db: mockDB}

	items := []notification.Notification{
		{ID: 1, Recipient: "a@example.com", Channel: "email", Content: "hello", Priority: priority.PriorityLow, Status: int(status.StatusPending), BatchID: "b-1"},
	}

	mockDB.On("ListNotifications", mock.MatchedBy(func(f notification.ListFilter) bool {
		return f.Page == 1 && f.PageSize == 20
	})).Return(items, 1, nil)

	req, _ := http.NewRequest(http.MethodGet, "/notifications", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.List(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockDB.AssertExpectations(t)
}

func TestList_ParsesStartAndEndQueryParams(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockDB := new(MockNotificationDB)
	handler := &NotificationHandler{db: mockDB}

	start := time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC)
	end := time.Date(2026, 3, 2, 10, 0, 0, 0, time.UTC)

	mockDB.On("ListNotifications", mock.MatchedBy(func(f notification.ListFilter) bool {
		if f.DateFrom == nil || f.DateTo == nil {
			return false
		}
		return f.DateFrom.Equal(start) && f.DateTo.Equal(end)
	})).Return([]notification.Notification{}, 0, nil)

	req, _ := http.NewRequest(http.MethodGet, "/notifications?start=2026-03-01T10:00:00Z&end=2026-03-02T10:00:00Z", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.List(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockDB.AssertExpectations(t)
}

func TestList_InvalidStartFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockDB := new(MockNotificationDB)
	handler := &NotificationHandler{db: mockDB}

	req, _ := http.NewRequest(http.MethodGet, "/notifications?start=invalid", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.List(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "start must be RFC3339 format")
	mockDB.AssertNotCalled(t, "ListNotifications", mock.Anything)
}

func TestList_InvalidEndFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockDB := new(MockNotificationDB)
	handler := &NotificationHandler{db: mockDB}

	req, _ := http.NewRequest(http.MethodGet, "/notifications?end=invalid", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req

	handler.List(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "end must be RFC3339 format")
	mockDB.AssertNotCalled(t, "ListNotifications", mock.Anything)
}

func TestGetByBatchID_Success(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockDB := new(MockNotificationDB)
	handler := &NotificationHandler{db: mockDB}

	batchID := "batch-123"
	rows := []notification.Notification{
		{ID: 1, Recipient: "u@example.com", Channel: "email", Content: "hello", Priority: priority.PriorityHigh, Status: int(status.StatusPending), BatchID: batchID},
	}
	mockDB.On("GetNotificationsByBatchID", batchID).Return(rows, nil)

	req, _ := http.NewRequest(http.MethodGet, "/notifications/batch/"+batchID, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "batchId", Value: batchID}}

	handler.GetByBatchID(c)

	assert.Equal(t, http.StatusOK, w.Code)
	mockDB.AssertExpectations(t)
}

func TestGetByBatchID_InvalidBatchID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockDB := new(MockNotificationDB)
	handler := &NotificationHandler{db: mockDB}

	req, _ := http.NewRequest(http.MethodGet, "/notifications/batch/", nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "batchId", Value: ""}}

	handler.GetByBatchID(c)

	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "Invalid batchId")
	mockDB.AssertNotCalled(t, "GetNotificationsByBatchID", mock.Anything)
}

func TestGetByBatchID_NotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	mockDB := new(MockNotificationDB)
	handler := &NotificationHandler{db: mockDB}

	batchID := "missing-batch"
	mockDB.On("GetNotificationsByBatchID", batchID).Return([]notification.Notification{}, nil)

	req, _ := http.NewRequest(http.MethodGet, "/notifications/batch/"+batchID, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	c.Params = gin.Params{{Key: "batchId", Value: batchID}}

	handler.GetByBatchID(c)

	assert.Equal(t, http.StatusNotFound, w.Code)
	assert.Contains(t, w.Body.String(), "No notifications found")
	mockDB.AssertExpectations(t)
}
