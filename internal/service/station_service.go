package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"ev-charging-status-service/internal/clients/intelbras"
	"ev-charging-status-service/internal/crypto"
	"ev-charging-status-service/internal/repository"

	"github.com/google/uuid"
)

// ErrIntelbrasAPIKeyMismatch indica que a apiKey do cliente não bate com a salva em third_party_credentials.
var ErrIntelbrasAPIKeyMismatch = errors.New("intelbras api key mismatch")

type StationService struct {
	credsRepo       *repository.CredentialsRepository
	intelbrasClient *intelbras.Client
	encryptionKey   []byte
}

func NewStationService(
	credsRepo *repository.CredentialsRepository,
	intelbrasClient *intelbras.Client,
	encryptionKey []byte,
) *StationService {
	return &StationService{
		credsRepo:       credsRepo,
		intelbrasClient: intelbrasClient,
		encryptionKey:   encryptionKey,
	}
}

// GetStations busca a lista de estações na API de terceiros usando o token salvo.
// Se o token estiver expirado ou ausente, faz login de novo e atualiza as credenciais.
func (s *StationService) GetStations(ctx context.Context) ([]intelbras.FlattenedChargePoint, error) {
	creds, err := s.credsRepo.GetFirst(ctx)
	if err != nil {
		return nil, fmt.Errorf("no credentials configured: %w", err)
	}
	return s.getStationsWithCreds(ctx, creds)
}

func (s *StationService) GetStationsByUserID(ctx context.Context, userID uuid.UUID) ([]intelbras.FlattenedChargePoint, error) {
	creds, err := s.credsRepo.GetByUserID(ctx, userID)
	if err != nil {
		return nil, fmt.Errorf("no credentials configured for user: %w", err)
	}
	return s.getStationsWithCreds(ctx, creds)
}

// VerifyIntelbrasAPIKeyForUser compara a apiKey informada pelo cliente com a armazenada para o usuário (após descriptografar se necessário).
func (s *StationService) VerifyIntelbrasAPIKeyForUser(ctx context.Context, userID uuid.UUID, clientAPIKey string) error {
	creds, err := s.credsRepo.GetByUserID(ctx, userID)
	if err != nil {
		return err
	}
	s.decryptCredsInPlace(creds)
	stored := ""
	if creds.APIKey != nil {
		stored = *creds.APIKey
	}
	if strings.TrimSpace(clientAPIKey) != stored {
		return ErrIntelbrasAPIKeyMismatch
	}
	return nil
}

// BuildStationsPushPayload monta o mesmo JSON que o WebSocket envia (userId, timestamp, stations).
func (s *StationService) BuildStationsPushPayload(ctx context.Context, userID uuid.UUID) (*WebhookPayload, error) {
	list, err := s.GetStationsByUserID(ctx, userID)
	if err != nil {
		return nil, err
	}
	return &WebhookPayload{
		UserID:    userID.String(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Stations:  flattenToWebhookStations(list),
	}, nil
}

func (s *StationService) decryptCredsInPlace(creds *repository.ThirdPartyCredentials) {
	if len(s.encryptionKey) == 0 {
		return
	}
	if dec, _ := crypto.Decrypt(creds.APIPassword, s.encryptionKey); len(dec) > 0 && string(dec) != creds.APIPassword {
		creds.APIPassword = string(dec)
	}
	if creds.APIKey != nil {
		if dec, _ := crypto.Decrypt(*creds.APIKey, s.encryptionKey); len(dec) > 0 && string(dec) != *creds.APIKey {
			k := string(dec)
			creds.APIKey = &k
		}
	}
}

func (s *StationService) getStationsWithCreds(ctx context.Context, creds *repository.ThirdPartyCredentials) ([]intelbras.FlattenedChargePoint, error) {
	s.decryptCredsInPlace(creds)

	token := ""
	if creds.AccessToken != nil {
		token = *creds.AccessToken
	}
	needRefresh := token == "" || creds.TokenExpiresAt == nil || creds.TokenExpiresAt.Before(time.Now().Add(5*time.Minute))

	if needRefresh {
		loginReq := intelbras.LoginRequest{
			Email:             creds.APIUsername,
			Password:          creds.APIPassword,
			RecaptchaResponse: "",
		}
		if creds.APIKey != nil {
			loginReq.APIKey = *creds.APIKey
		}
		loginResp, err := s.intelbrasClient.Login(ctx, loginReq)
		if err != nil {
			return nil, fmt.Errorf("refresh token: %w", err)
		}
		if loginResp != nil {
			token = loginResp.AccessToken
			expires := loginResp.ExpiresAt
			if expires.IsZero() {
				expires = time.Now().Add(30 * time.Minute)
			}
			_ = s.credsRepo.UpdateToken(ctx, creds.ID, token, expires)
		}
	}

	resp, err := s.intelbrasClient.GetChargePointList(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("fetch charge points: %w", err)
	}

	return intelbras.FlattenChargePointList(resp.ChargePointList), nil
}
