package service

import (
	"context"
	"fmt"
	"time"

	"ev-charging-status-service/internal/clients/intelbras"
	"ev-charging-status-service/internal/crypto"
	"ev-charging-status-service/internal/repository"
)

type ConfigService struct {
	usersRepo       *repository.UserRepository
	credsRepo       *repository.CredentialsRepository
	webhookRepo     *repository.WebhookRepository
	intelbrasClient *intelbras.Client
	encryptionKey   []byte
}

type ConfigInput struct {
	Email              string
	Password           string
	RecaptchaResponse  string
	APIKey             *string
	WebhookURL         string
}

func NewConfigService(
	usersRepo *repository.UserRepository,
	credsRepo *repository.CredentialsRepository,
	webhookRepo *repository.WebhookRepository,
	intelbrasClient *intelbras.Client,
	encryptionKey []byte,
) *ConfigService {
	return &ConfigService{
		usersRepo:       usersRepo,
		credsRepo:       credsRepo,
		webhookRepo:     webhookRepo,
		intelbrasClient: intelbrasClient,
		encryptionKey:   encryptionKey,
	}
}

func (s *ConfigService) Configure(ctx context.Context, in ConfigInput) (*repository.User, error) {
	// 1. Upsert user (identificado pelo email)
	user, err := s.usersRepo.UpsertByUsername(ctx, in.Email)
	if err != nil {
		return nil, fmt.Errorf("upsert user: %w", err)
	}

	// 2. Login na API de terceiros: body = email, password, recaptchaResponse; header = API-Key
	loginReq := intelbras.LoginRequest{
		Email:              in.Email,
		Password:           in.Password,
		RecaptchaResponse:  in.RecaptchaResponse,
	}
	if in.APIKey != nil {
		loginReq.APIKey = *in.APIKey
	}

	loginResp, err := s.intelbrasClient.Login(ctx, loginReq)
	if err != nil {
		return nil, fmt.Errorf("login intelbras: %w", err)
	}

	// 3. Salvar credenciais e token (criptografar senha e api_key se chave definida)
	creds := &repository.ThirdPartyCredentials{
		UserID:      user.ID,
		APIUsername: in.Email,
		APIPassword: in.Password,
		APIKey:      in.APIKey,
	}
	if len(s.encryptionKey) > 0 {
		encPwd, errEnc := crypto.Encrypt([]byte(in.Password), s.encryptionKey)
		if errEnc != nil {
			return nil, fmt.Errorf("encrypt password: %w", errEnc)
		}
		creds.APIPassword = encPwd
		if in.APIKey != nil && *in.APIKey != "" {
			encKey, errEnc := crypto.Encrypt([]byte(*in.APIKey), s.encryptionKey)
			if errEnc != nil {
				return nil, fmt.Errorf("encrypt api_key: %w", errEnc)
			}
			creds.APIKey = &encKey
		}
	}
	if loginResp != nil {
		creds.AccessToken = &loginResp.AccessToken
		expires := loginResp.ExpiresAt
		if expires.IsZero() {
			expires = time.Now().Add(30 * time.Minute)
		}
		creds.TokenExpiresAt = &expires
	}

	if err := s.credsRepo.Upsert(ctx, creds); err != nil {
		return nil, fmt.Errorf("upsert credentials: %w", err)
	}

	// 4. Salvar webhook apenas quando informado.
	if in.WebhookURL != "" {
		if _, err := s.webhookRepo.UpsertForUser(ctx, user.ID, in.WebhookURL); err != nil {
			return nil, fmt.Errorf("upsert webhook: %w", err)
		}
	}

	return user, nil
}

// ConfigStatus indica se há config e se o token foi salvo (sem expor o token).
type ConfigStatus struct {
	Configured    bool       `json:"configured"`
	TokenPresent  bool       `json:"tokenPresent"`
	TokenExpiresAt *string   `json:"tokenExpiresAt,omitempty"`
	APIUsername   string     `json:"apiUsername,omitempty"`
}

// GetConfigStatus retorna o status da configuração (primeira credencial em single-tenant).
func (s *ConfigService) GetConfigStatus(ctx context.Context) (*ConfigStatus, error) {
	creds, err := s.credsRepo.GetFirst(ctx)
	if err != nil {
		return &ConfigStatus{Configured: false}, nil
	}
	st := &ConfigStatus{
		Configured:   true,
		APIUsername:  creds.APIUsername,
		TokenPresent: creds.AccessToken != nil && *creds.AccessToken != "",
	}
	if creds.TokenExpiresAt != nil {
		t := creds.TokenExpiresAt.Format("2006-01-02T15:04:05Z07:00")
		st.TokenExpiresAt = &t
	}
	return st, nil
}

// DeleteConfiguredUser remove o usuário configurado (single-tenant) e todos os dados relacionados.
// A remoção depende de ON DELETE CASCADE nas FKs (credenciais, webhooks, webhook_events, etc.).
func (s *ConfigService) DeleteConfiguredUser(ctx context.Context) error {
	creds, err := s.credsRepo.GetFirst(ctx)
	if err != nil {
		return nil
	}
	return s.usersRepo.DeleteByID(ctx, creds.UserID)
}

// DeleteUserByEmail remove o usuário pelo email/username e todos os dados relacionados.
// A remoção depende de ON DELETE CASCADE nas FKs.
func (s *ConfigService) DeleteUserByEmail(ctx context.Context, email string) error {
	if email == "" {
		return nil
	}
	return s.usersRepo.DeleteByUsername(ctx, email)
}

