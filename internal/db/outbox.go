package db

import (
	"database/sql"
	"fmt"
	"log"
	"notif-api/internal/models"
	"time"
)

type OutboxRepository interface {
	CreateOutboxEvent(aggregateID string, eventType models.OutboxEventType) (*models.OutboxEvent, error)
	GetPendingEvents(limit int) ([]models.OutboxEvent, error)
	MarkEventAsPublished(eventID int) error
	MarkEventAsFailed(eventID int, errorMessage string) error
	GetEventByID(eventID int) (*models.OutboxEvent, error)
	DeletePublishedEvents(olderThanHours int) error
}

type OutboxDB struct {
	db *sql.DB
}

func NewOutboxDB(db *sql.DB) *OutboxDB {
	return &OutboxDB{db: db}
}

func (odb *OutboxDB) CreateOutboxEvent(aggregateID string, eventType models.OutboxEventType) (*models.OutboxEvent, error) {
	query := `
		INSERT INTO outbox (aggregate_id, event_type, status, created_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, aggregate_id, event_type, status, created_at, published_at, error_message
	`

	var event models.OutboxEvent
	var eventTypeStr string
	var publishedAt sql.NullTime
	var errorMessage sql.NullString

	err := odb.db.QueryRow(
		query,
		aggregateID,
		string(eventType),
		models.OutboxStatusPending,
		time.Now(),
		0,
	).Scan(
		&event.ID,
		&event.AggregateID,
		&eventTypeStr,
		&event.Status,
		&event.CreatedAt,
		&publishedAt,
		&errorMessage,
	)

	if err != nil {
		return nil, fmt.Errorf("failed to create outbox event: %w", err)
	}

	event.EventType = models.OutboxEventType(eventTypeStr)
	if publishedAt.Valid {
		event.PublishedAt = &publishedAt.Time
	}
	if errorMessage.Valid {
		event.ErrorMessage = &errorMessage.String
	}

	return &event, nil
}

func (odb *OutboxDB) GetPendingEvents(limit int) ([]models.OutboxEvent, error) {
	log.Printf("GetPendingEvents: limit=%d", limit)
	query := `
		SELECT id, aggregate_id, event_type, status, created_at, published_at, error_message
		FROM outbox
		WHERE status = $1
		ORDER BY created_at ASC
		LIMIT $2
	`

	rows, err := odb.db.Query(query, models.OutboxStatusPending, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to query pending events: %w", err)
	}
	defer func() {
		_ = rows.Close()
	}()

	var events []models.OutboxEvent
	for rows.Next() {
		var event models.OutboxEvent
		var publishedAt sql.NullTime
		var errorMessage sql.NullString
		var eventTypeStr string

		err := rows.Scan(
			&event.ID,
			&event.AggregateID,
			&eventTypeStr,
			&event.Status,
			&event.CreatedAt,
			&publishedAt,
			&errorMessage,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to scan event: %w", err)
		}

		event.EventType = models.OutboxEventType(eventTypeStr)
		if publishedAt.Valid {
			event.PublishedAt = &publishedAt.Time
		}
		if errorMessage.Valid {
			event.ErrorMessage = &errorMessage.String
		}

		events = append(events, event)
	}

	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating events: %w", err)
	}

	return events, nil
}

func (odb *OutboxDB) MarkEventAsPublished(eventID int) error {
	query := `
		UPDATE outbox
		SET status = $1, published_at = $2
		WHERE id = $3
	`

	result, err := odb.db.Exec(query, models.OutboxStatusPublished, time.Now(), eventID)
	if err != nil {
		return fmt.Errorf("failed to update event status: %w", err)
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

func (odb *OutboxDB) MarkEventAsFailed(eventID int, errorMessage string) error {
	query := `
		UPDATE outbox
		SET status = $1, error_message = $2
		WHERE id = $3
	`

	result, err := odb.db.Exec(query, models.OutboxStatusFailed, errorMessage, eventID)
	if err != nil {
		return fmt.Errorf("failed to mark event as failed: %w", err)
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

func (odb *OutboxDB) GetEventByID(eventID int) (*models.OutboxEvent, error) {
	query := `
		SELECT id, aggregate_id, event_type, status, created_at, published_at, error_message
		FROM outbox
		WHERE id = $1
	`

	var event models.OutboxEvent
	var publishedAt sql.NullTime
	var errorMessage sql.NullString
	var eventTypeStr string

	err := odb.db.QueryRow(query, eventID).Scan(
		&event.ID,
		&event.AggregateID,
		&eventTypeStr,
		&event.Status,
		&event.CreatedAt,
		&publishedAt,
		&errorMessage,
	)

	if err != nil {
		return nil, err
	}

	event.EventType = models.OutboxEventType(eventTypeStr)
	if publishedAt.Valid {
		event.PublishedAt = &publishedAt.Time
	}
	if errorMessage.Valid {
		event.ErrorMessage = &errorMessage.String
	}

	return &event, nil
}

func (odb *OutboxDB) DeletePublishedEvents(olderThanHours int) error {
	query := `
		DELETE FROM outbox
		WHERE status = $1 AND published_at < NOW() - INTERVAL '1 hour' * $2
	`

	_, err := odb.db.Exec(query, models.OutboxStatusPublished, olderThanHours)
	if err != nil {
		return fmt.Errorf("failed to delete published events: %w", err)
	}

	return nil
}
