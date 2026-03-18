package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type Webhook struct {
	ID        uuid.UUID `db:"id"`
	UserID    uuid.UUID `db:"user_id"`
	TargetURL string    `db:"target_url"`
	Active    bool      `db:"active"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
}

type WebhookRepository struct {
	db *sqlx.DB
}

func NewWebhookRepository(db *sqlx.DB) *WebhookRepository {
	return &WebhookRepository{db: db}
}

func (r *WebhookRepository) GetByID(ctx context.Context, id uuid.UUID) (*Webhook, error) {
	var w Webhook
	if err := r.db.GetContext(ctx, &w, `
SELECT id, user_id, target_url, active, created_at, updated_at
FROM webhooks
WHERE id = $1
`, id); err != nil {
		return nil, err
	}
	return &w, nil
}

// GetByUserID retorna um webhook do usuário (qualquer um, para "um webhook por usuário").
func (r *WebhookRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*Webhook, error) {
	var w Webhook
	err := r.db.GetContext(ctx, &w, `
		SELECT id, user_id, target_url, active, created_at, updated_at
		FROM webhooks WHERE user_id = $1 ORDER BY created_at DESC LIMIT 1
	`, userID)
	if err != nil {
		return nil, err
	}
	return &w, nil
}

func (r *WebhookRepository) Update(ctx context.Context, w *Webhook) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE webhooks SET target_url = $1, active = $2, updated_at = NOW() WHERE id = $3
	`, w.TargetURL, w.Active, w.ID)
	return err
}

func (r *WebhookRepository) UpsertForUser(ctx context.Context, userID uuid.UUID, targetURL string) (*Webhook, error) {
	existing, err := r.GetByUserID(ctx, userID)
	if err == nil {
		// Desativa os demais webhooks deste usuário (evita duplicata de envio).
		_, _ = r.db.ExecContext(ctx, `UPDATE webhooks SET active = false WHERE user_id = $1 AND id != $2`, userID, existing.ID)
		existing.TargetURL = targetURL
		existing.Active = true
		if err := r.Update(ctx, existing); err != nil {
			return nil, err
		}
		return existing, nil
	}

	w := &Webhook{
		ID:        uuid.New(),
		UserID:    userID,
		TargetURL: targetURL,
		Active:    true,
	}
	query := `
		INSERT INTO webhooks (id, user_id, target_url, active)
		VALUES ($1, $2, $3, $4)
		RETURNING id, user_id, target_url, active, created_at, updated_at
	`
	err = r.db.QueryRowContext(ctx, query, w.ID, w.UserID, w.TargetURL, w.Active).Scan(
		&w.ID, &w.UserID, &w.TargetURL, &w.Active, &w.CreatedAt, &w.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return w, nil
}

// ListActiveByUserID retorna os webhooks ativos do usuário, mais recente primeiro (updated_at DESC).
func (r *WebhookRepository) ListActiveByUserID(ctx context.Context, userID uuid.UUID) ([]Webhook, error) {
	var list []Webhook
	err := r.db.SelectContext(ctx, &list, `
		SELECT id, user_id, target_url, active, created_at, updated_at
		FROM webhooks
		WHERE user_id = $1 AND active = true
		ORDER BY updated_at DESC
	`, userID)
	return list, err
}

