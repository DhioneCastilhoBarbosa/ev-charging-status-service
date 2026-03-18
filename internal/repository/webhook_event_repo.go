package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type WebhookEventStatus string

const (
	WebhookEventStatusPending  WebhookEventStatus = "PENDING"
	WebhookEventStatusSent     WebhookEventStatus = "SENT"
	WebhookEventStatusFailed   WebhookEventStatus = "FAILED"
	WebhookEventStatusRetrying WebhookEventStatus = "RETRYING"
)

type WebhookEvent struct {
	ID           uuid.UUID         `db:"id"`
	WebhookID    uuid.UUID         `db:"webhook_id"`
	Payload      []byte            `db:"payload"`
	Status       WebhookEventStatus `db:"status"`
	LastError    *string           `db:"last_error"`
	AttemptCount int               `db:"attempt_count"`
	NextAttempt  *time.Time        `db:"next_attempt_at"`
	CreatedAt    time.Time         `db:"created_at"`
	UpdatedAt    time.Time         `db:"updated_at"`
}

type WebhookEventRepository struct {
	db *sqlx.DB
}

func NewWebhookEventRepository(db *sqlx.DB) *WebhookEventRepository {
	return &WebhookEventRepository{db: db}
}

func (r *WebhookEventRepository) Create(ctx context.Context, e *WebhookEvent) error {
	if e.ID == uuid.Nil {
		e.ID = uuid.New()
	}
	if e.Status == "" {
		e.Status = WebhookEventStatusPending
	}

	query := `
INSERT INTO webhook_events (
    id, webhook_id, payload, status, last_error, attempt_count, next_attempt_at
) VALUES (
    :id, :webhook_id, :payload, :status, :last_error, :attempt_count, :next_attempt_at
);
`
	_, err := r.db.NamedExecContext(ctx, query, e)
	return err
}

func (r *WebhookEventRepository) FindDue(ctx context.Context, limit int) ([]WebhookEvent, error) {
	query := `
SELECT id, webhook_id, payload, status, last_error, attempt_count, next_attempt_at, created_at, updated_at
FROM webhook_events
WHERE (status = 'PENDING' OR status = 'RETRYING')
  AND (next_attempt_at IS NULL OR next_attempt_at <= NOW())
ORDER BY created_at
LIMIT $1;
`
	var events []WebhookEvent
	if err := r.db.SelectContext(ctx, &events, query, limit); err != nil {
		return nil, err
	}
	return events, nil
}

func (r *WebhookEventRepository) Update(ctx context.Context, e *WebhookEvent) error {
	query := `
UPDATE webhook_events
SET status = :status,
    last_error = :last_error,
    attempt_count = :attempt_count,
    next_attempt_at = :next_attempt_at,
    updated_at = NOW()
WHERE id = :id;
`
	_, err := r.db.NamedExecContext(ctx, query, e)
	return err
}

