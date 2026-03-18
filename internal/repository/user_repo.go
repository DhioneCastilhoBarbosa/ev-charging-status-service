package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type User struct {
	ID        uuid.UUID `db:"id"`
	Username  string    `db:"username"`
	CreatedAt time.Time `db:"created_at"`
	UpdatedAt time.Time `db:"updated_at"`
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
ON CONFLICT (username) DO UPDATE SET username = EXCLUDED.username
RETURNING id, username, created_at, updated_at;
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

