package service

import (
	"context"
	"fmt"
	"time"

	"ev-charging-status-service/internal/clients/intelbras"
	"ev-charging-status-service/internal/crypto"
	"ev-charging-status-service/internal/repository"
	"github.com/google/uuid"
)

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

func (s *StationService) getStationsWithCreds(ctx context.Context, creds *repository.ThirdPartyCredentials) ([]intelbras.FlattenedChargePoint, error) {
	// Descriptografar senha e api_key para uso no login (compatível com dados em texto plano).
	// Só substitui quando o resultado é diferente do valor em DB (evita usar o fallback como senha quando a chave está errada).
	if len(s.encryptionKey) > 0 {
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
