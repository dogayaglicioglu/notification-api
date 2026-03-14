package db

import (
	"database/sql"
	"fmt"
	"notif-api/internal/domain/status"
	"notif-api/internal/models"
	"notif-api/internal/models/notification"
	"strconv"
	"time"

	"github.com/google/uuid"
)

type NotificationRepository interface {
	CreateNotificationsBatch(notifications []notification.Notification) (string, []notification.Notification, error)
	GetNotificationsByBatchID(batchID string) ([]notification.Notification, error)
	GetNotificationByID(id int) (*notification.Notification, error)
	UpdateNotificationStatus(id int, status int) error
	CancelAllPendingNotifications() (int64, error)
	ListNotifications(filter notification.ListFilter) ([]notification.Notification, int, error)
}

type NotificationDB struct {
	db *sql.DB
}

func NewNotificationDB(db *sql.DB) *NotificationDB {
	return &NotificationDB{db: db}
}

func (ndb *NotificationDB) CreateNotificationsBatch(notifications []notification.Notification) (string, []notification.Notification, error) {
	return ndb.CreateNotificationsBatchWithOutbox(notifications, models.OutboxEventNotificationCreated)
}

func (ndb *NotificationDB) CreateNotificationsBatchWithOutbox(notifications []notification.Notification, eventType models.OutboxEventType) (string, []notification.Notification, error) {
	if len(notifications) == 0 {
		return "", nil, fmt.Errorf("no notifications to create")
	}

	tx, err := ndb.db.Begin()
	if err != nil {
		return "", nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	batchID := uuid.New().String()
	now := time.Now()

	query := `INSERT INTO notifications (recipient, channel, content, priority, status, batchId, date) VALUES `
	args := make([]interface{}, 0, len(notifications)*7)
	values := ""

	for i, n := range notifications {
		idx := i * 7
		values += fmt.Sprintf("($%d,$%d,$%d,$%d,$%d,$%d,$%d),",
			idx+1, idx+2, idx+3, idx+4, idx+5, idx+6, idx+7)
		args = append(args, n.Recipient, n.Channel, n.Content, n.Priority, n.Status, batchID, now)
	}

	values = values[:len(values)-1]
	query += values + ` RETURNING id, recipient, channel, content, priority, status, batchId, date`

	rows, err := tx.Query(query, args...)
	if err != nil {
		return "", nil, fmt.Errorf("insert notifications: %w", err)
	}
	defer rows.Close()

	created := make([]notification.Notification, 0, len(notifications))
	for rows.Next() {
		var n notification.Notification
		if err := rows.Scan(
			&n.ID,
			&n.Recipient,
			&n.Channel,
			&n.Content,
			&n.Priority,
			&n.Status,
			&n.BatchID,
			&n.CreatedAt,
		); err != nil {
			return "", nil, err
		}
		created = append(created, n)
	}

	if err := rows.Err(); err != nil {
		return "", nil, err
	}

	if err := ndb.insertOutboxEventsTx(tx, created, eventType, now); err != nil {
		return "", nil, err
	}

	if err := tx.Commit(); err != nil {
		return "", nil, err
	}

	return batchID, created, nil
}

func (ndb *NotificationDB) insertOutboxEventsTx(tx *sql.Tx, notifications []notification.Notification, eventType models.OutboxEventType, now time.Time) error {
	query := `
	INSERT INTO outbox
	(aggregate_id, event_type, status, created_at)
	VALUES ($1,$2,$3,$4)
	`

	for _, n := range notifications {

		_, err := tx.Exec(
			query,
			strconv.Itoa(n.ID),
			string(eventType),
			models.OutboxStatusPending,
			now,
		)

		if err != nil {
			return fmt.Errorf("insert outbox event: %w", err)
		}
	}

	return nil
}

func (ndb *NotificationDB) GetNotificationsByBatchID(batchID string) ([]notification.Notification, error) {
	query := `SELECT id, recipient, channel, content, priority, status, batchId, date, cancelled_at
	          FROM notifications WHERE batchId = $1 ORDER BY id`

	rows, err := ndb.db.Query(query, batchID)
	if err != nil {
		return nil, fmt.Errorf("failed to query notifications: %w", err)
	}
	defer rows.Close()

	var notifications []notification.Notification
	for rows.Next() {
		var notif notification.Notification
		var cancelledAt sql.NullTime
		err := rows.Scan(&notif.ID, &notif.Recipient, &notif.Channel, &notif.Content,
			&notif.Priority, &notif.Status, &notif.BatchID, &notif.CreatedAt, &cancelledAt)
		if err != nil {
			return nil, fmt.Errorf("failed to scan notification: %w", err)
		}
		if cancelledAt.Valid {
			notif.CancelledAt = &cancelledAt.Time
		}
		notifications = append(notifications, notif)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating notifications: %w", err)
	}

	return notifications, nil
}

func (ndb *NotificationDB) GetNotificationByID(id int) (*notification.Notification, error) {
	query := `SELECT id, recipient, channel, content, priority, status, batchId, date, cancelled_at
	          FROM notifications WHERE id = $1`

	var notif notification.Notification
	var cancelledAt sql.NullTime
	if err := ndb.db.QueryRow(query, id).Scan(
		&notif.ID,
		&notif.Recipient,
		&notif.Channel,
		&notif.Content,
		&notif.Priority,
		&notif.Status,
		&notif.BatchID,
		&notif.CreatedAt,
		&cancelledAt,
	); err != nil {
		return nil, err
	}
	if cancelledAt.Valid {
		notif.CancelledAt = &cancelledAt.Time
	}

	return &notif, nil
}

func (ndb *NotificationDB) UpdateNotificationStatus(id int, status int) error {
	query := "UPDATE notifications SET status = $1 WHERE id = $2"

	result, err := ndb.db.Exec(query, status, id)
	if err != nil {
		return fmt.Errorf("failed to update notification status: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return sql.ErrNoRows
	}

	return nil
}

func (ndb *NotificationDB) CancelAllPendingNotifications() (int64, error) {
	query := `
		WITH cancelled_notifications AS (
			UPDATE notifications
			SET status = $1, cancelled_at = NOW()
			WHERE status = $2
			RETURNING id
		),
		cancelled_outbox AS (
			UPDATE outbox o
			SET status = $3
			FROM cancelled_notifications cn
			WHERE o.aggregate_id = cn.id::text AND o.status = $4
			RETURNING o.id
		)
		SELECT COUNT(*) FROM cancelled_notifications
	`

	var cancelledCount int64
	err := ndb.db.QueryRow(
		query,
		int(status.StatusCancelled),
		int(status.StatusPending),
		models.OutboxStatusCancelled,
		models.OutboxStatusPending,
	).Scan(&cancelledCount)
	if err != nil {
		return 0, fmt.Errorf("failed to cancel pending notifications: %w", err)
	}

	return cancelledCount, nil
}

func (ndb *NotificationDB) ListNotifications(filter notification.ListFilter) ([]notification.Notification, int, error) {
	if filter.PageSize <= 0 {
		filter.PageSize = 20
	}
	if filter.Page <= 0 {
		filter.Page = 1
	}

	args := []interface{}{}
	argIdx := 1
	where := "WHERE 1=1"

	if filter.Status != nil {
		where += fmt.Sprintf(" AND status = $%d", argIdx)
		args = append(args, *filter.Status)
		argIdx++
	}
	if filter.Channel != nil {
		where += fmt.Sprintf(" AND channel = $%d", argIdx)
		args = append(args, *filter.Channel)
		argIdx++
	}
	if filter.DateFrom != nil {
		where += fmt.Sprintf(" AND date >= $%d", argIdx)
		args = append(args, *filter.DateFrom)
		argIdx++
	}
	if filter.DateTo != nil {
		where += fmt.Sprintf(" AND date <= $%d", argIdx)
		args = append(args, *filter.DateTo)
		argIdx++
	}

	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM notifications %s", where)
	var total int
	if err := ndb.db.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count notifications: %w", err)
	}

	offset := (filter.Page - 1) * filter.PageSize
	listQuery := fmt.Sprintf(`
		SELECT id, recipient, channel, content, priority, status, batchId, date, cancelled_at
		FROM notifications %s
		ORDER BY id DESC
		LIMIT $%d OFFSET $%d
	`, where, argIdx, argIdx+1)
	args = append(args, filter.PageSize, offset)

	rows, err := ndb.db.Query(listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list notifications: %w", err)
	}
	defer rows.Close()

	var notifications []notification.Notification
	for rows.Next() {
		var notif notification.Notification
		var cancelledAt sql.NullTime
		if err := rows.Scan(&notif.ID, &notif.Recipient, &notif.Channel, &notif.Content,
			&notif.Priority, &notif.Status, &notif.BatchID, &notif.CreatedAt, &cancelledAt); err != nil {
			return nil, 0, fmt.Errorf("failed to scan notification: %w", err)
		}
		if cancelledAt.Valid {
			notif.CancelledAt = &cancelledAt.Time
		}
		notifications = append(notifications, notif)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating notifications: %w", err)
	}

	return notifications, total, nil
}
