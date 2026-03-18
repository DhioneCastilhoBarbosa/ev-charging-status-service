package repository

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
)

type ThirdPartyCredentials struct {
	ID            uuid.UUID `db:"id"`
	UserID        uuid.UUID `db:"user_id"`
	APIUsername   string    `db:"api_username"`
	APIPassword   string    `db:"api_password"`
	APIKey        *string   `db:"api_key"`
	AccessToken   *string   `db:"access_token"`
	TokenExpiresAt *time.Time `db:"token_expires_at"`
	CreatedAt     time.Time `db:"created_at"`
	UpdatedAt     time.Time `db:"updated_at"`
}

type CredentialsRepository struct {
	db *sqlx.DB
}

func NewCredentialsRepository(db *sqlx.DB) *CredentialsRepository {
	return &CredentialsRepository{db: db}
}

func (r *CredentialsRepository) Upsert(ctx context.Context, c *ThirdPartyCredentials) error {
	if c.ID == uuid.Nil {
		c.ID = uuid.New()
	}

	query := `
INSERT INTO third_party_credentials (
    id, user_id, api_username, api_password, api_key, access_token, token_expires_at
) VALUES (
    :id, :user_id, :api_username, :api_password, :api_key, :access_token, :token_expires_at
)
ON CONFLICT (id) DO UPDATE SET
    api_username = EXCLUDED.api_username,
    api_password = EXCLUDED.api_password,
    api_key = EXCLUDED.api_key,
    access_token = EXCLUDED.access_token,
    token_expires_at = EXCLUDED.token_expires_at,
    updated_at = NOW();
`
	_, err := r.db.NamedExecContext(ctx, query, c)
	return err
}

// GetByUserID retorna as credenciais do usuário (uma por usuário).
func (r *CredentialsRepository) GetByUserID(ctx context.Context, userID uuid.UUID) (*ThirdPartyCredentials, error) {
	var c ThirdPartyCredentials
	err := r.db.GetContext(ctx, &c, `
		SELECT id, user_id, api_username, api_password, api_key, access_token, token_expires_at, created_at, updated_at
		FROM third_party_credentials WHERE user_id = $1 LIMIT 1
	`, userID)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// GetFirst retorna a primeira credencial cadastrada (útil para single-tenant).
func (r *CredentialsRepository) GetFirst(ctx context.Context) (*ThirdPartyCredentials, error) {
	var c ThirdPartyCredentials
	err := r.db.GetContext(ctx, &c, `
		SELECT id, user_id, api_username, api_password, api_key, access_token, token_expires_at, created_at, updated_at
		FROM third_party_credentials ORDER BY created_at DESC LIMIT 1
	`)
	if err != nil {
		return nil, err
	}
	return &c, nil
}

// UpdateToken atualiza apenas access_token e token_expires_at (evita reescrever senha/api_key).
func (r *CredentialsRepository) UpdateToken(ctx context.Context, id uuid.UUID, accessToken string, expiresAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE third_party_credentials SET access_token = $1, token_expires_at = $2, updated_at = NOW() WHERE id = $3
	`, accessToken, expiresAt, id)
	return err
}

