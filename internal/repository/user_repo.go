package repository

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type User struct {
	ID               uuid.UUID  `db:"id"`
	Username         string     `db:"username"`
	WSLastActivityAt *time.Time `db:"ws_last_activity_at"`
	CreatedAt        time.Time  `db:"created_at"`
	UpdatedAt        time.Time  `db:"updated_at"`
}

type UserRepository struct {
	db *sqlx.DB
}

func NewUserRepository(db *sqlx.DB) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) UpsertByUsername(ctx context.Context, username string) (*User, error) {
	u := &User{
		ID:       uuid.New(),
		Username: username,
	}

	query := `
INSERT INTO users (id, username)
VALUES (:id, :username)
ON CONFLICT (username) DO UPDATE SET username = EXCLUDED.username, updated_at = NOW()
RETURNING id, username, ws_last_activity_at, created_at, updated_at;
`
	rows, err := r.db.NamedQueryContext(ctx, query, u)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if rows.Next() {
		if err := rows.StructScan(u); err != nil {
			return nil, err
		}
	}

	return u, nil
}

func (r *UserRepository) DeleteByID(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE id = $1`, id)
	return err
}

func (r *UserRepository) DeleteByUsername(ctx context.Context, username string) error {
	_, err := r.db.ExecContext(ctx, `DELETE FROM users WHERE username = $1`, username)
	return err
}

func (r *UserRepository) GetByUsername(ctx context.Context, username string) (*User, error) {
	var u User
	err := r.db.GetContext(ctx, &u, `
		SELECT id, username, ws_last_activity_at, created_at, updated_at
		FROM users
		WHERE username = $1
		LIMIT 1
	`, username)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

func (r *UserRepository) GetByID(ctx context.Context, id uuid.UUID) (*User, error) {
	var u User
	err := r.db.GetContext(ctx, &u, `
		SELECT id, username, ws_last_activity_at, created_at, updated_at
		FROM users
		WHERE id = $1
		LIMIT 1
	`, id)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// TouchWSActivity atualiza ws_last_activity_at = NOW() (tráfego de aplicação / emissão de token).
func (r *UserRepository) TouchWSActivity(ctx context.Context, id uuid.UUID) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE users SET ws_last_activity_at = NOW(), updated_at = NOW() WHERE id = $1
	`, id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return sql.ErrNoRows
	}
	return nil
}

// ClearWSActivity invalida a sessão (idle ou logout explícito).
func (r *UserRepository) ClearWSActivity(ctx context.Context, id uuid.UUID) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE users SET ws_last_activity_at = NULL, updated_at = NOW() WHERE id = $1
	`, id)
	return err
}

// GetWSLastActivity retorna a última atividade; ok=false se NULL ou usuário inexistente.
func (r *UserRepository) GetWSLastActivity(ctx context.Context, id uuid.UUID) (time.Time, bool, error) {
	var ts sql.NullTime
	err := r.db.GetContext(ctx, &ts, `
		SELECT ws_last_activity_at FROM users WHERE id = $1 LIMIT 1
	`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, err
	}
	if !ts.Valid {
		return time.Time{}, false, nil
	}
	return ts.Time, true, nil
}
