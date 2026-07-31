package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"ev-charging-status-service/internal/repository"
)

var (
	ErrWSTokenInvalid   = errors.New("invalid ws token")
	ErrWSSessionExpired = errors.New("ws session idle timeout")
	ErrWSUserGone       = errors.New("ws user not found")
)

type WSAuthService struct {
	secret      []byte
	idleTimeout time.Duration
	usersRepo   *repository.UserRepository
	credsRepo   *repository.CredentialsRepository
}

type WSClaims struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func NewWSAuthService(
	secret []byte,
	idleTimeoutSeconds int,
	usersRepo *repository.UserRepository,
	credsRepo *repository.CredentialsRepository,
) *WSAuthService {
	idle := time.Duration(idleTimeoutSeconds) * time.Second
	if idle <= 0 {
		idle = time.Hour
	}
	return &WSAuthService{
		secret:      secret,
		idleTimeout: idle,
		usersRepo:   usersRepo,
		credsRepo:   credsRepo,
	}
}

func (s *WSAuthService) IdleTimeout() time.Duration {
	return s.idleTimeout
}

// GenerateToken emite JWT sem expira (exp). A sessão vive até delete do usuário ou idle sem tráfego de app no WS.
func (s *WSAuthService) GenerateToken(ctx context.Context, userID uuid.UUID, username string) (string, error) {
	if len(s.secret) == 0 {
		return "", fmt.Errorf("ws auth secret is not configured")
	}
	if err := s.usersRepo.TouchWSActivity(ctx, userID); err != nil {
		return "", fmt.Errorf("touch ws activity: %w", err)
	}
	now := time.Now().UTC()
	claims := WSClaims{
		UserID:   userID.String(),
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt: jwt.NewNumericDate(now),
			Subject:  userID.String(),
			// Sem ExpiresAt: validade controlada por ws_last_activity_at + idle.
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

// ValidateToken valida assinatura, existência do usuário/credenciais e idle da sessão.
func (s *WSAuthService) ValidateToken(ctx context.Context, tokenStr string) (*WSClaims, error) {
	if len(s.secret) == 0 {
		return nil, fmt.Errorf("ws auth secret is not configured")
	}
	token, err := jwt.ParseWithClaims(tokenStr, &WSClaims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method")
		}
		return s.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := token.Claims.(*WSClaims)
	if !ok || !token.Valid {
		return nil, ErrWSTokenInvalid
	}
	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		return nil, ErrWSTokenInvalid
	}

	user, err := s.usersRepo.GetByID(ctx, userID)
	if err != nil {
		return nil, ErrWSUserGone
	}
	hasCreds, err := s.credsRepo.ExistsByUserID(ctx, userID)
	if err != nil || !hasCreds {
		return nil, ErrWSUserGone
	}
	_ = user

	last, okAct, err := s.usersRepo.GetWSLastActivity(ctx, userID)
	if err != nil {
		return nil, err
	}
	if !okAct {
		return nil, ErrWSSessionExpired
	}
	if time.Since(last) > s.idleTimeout {
		_ = s.usersRepo.ClearWSActivity(ctx, userID)
		return nil, ErrWSSessionExpired
	}
	return claims, nil
}

// TouchActivity registra tráfego de aplicação (push WS / mensagem do cliente).
func (s *WSAuthService) TouchActivity(ctx context.Context, userID uuid.UUID) error {
	return s.usersRepo.TouchWSActivity(ctx, userID)
}

// InvalidateSession limpa a atividade (idle no servidor ou política explícita).
func (s *WSAuthService) InvalidateSession(ctx context.Context, userID uuid.UUID) error {
	return s.usersRepo.ClearWSActivity(ctx, userID)
}
