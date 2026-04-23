package service

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

type WSAuthService struct {
	secret []byte
	ttl    time.Duration
}

type WSClaims struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

func NewWSAuthService(secret []byte, ttlSeconds int) *WSAuthService {
	ttl := time.Duration(ttlSeconds) * time.Second
	if ttl <= 0 {
		ttl = 5 * time.Minute
	}
	return &WSAuthService{
		secret: secret,
		ttl:    ttl,
	}
}

func (s *WSAuthService) GenerateToken(userID uuid.UUID, username string) (string, error) {
	if len(s.secret) == 0 {
		return "", fmt.Errorf("ws auth secret is not configured")
	}
	now := time.Now().UTC()
	claims := WSClaims{
		UserID:   userID.String(),
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(s.ttl)),
			Subject:   userID.String(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString(s.secret)
}

func (s *WSAuthService) ValidateToken(tokenStr string) (*WSClaims, error) {
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
		return nil, fmt.Errorf("invalid ws token")
	}
	if _, err := uuid.Parse(claims.UserID); err != nil {
		return nil, fmt.Errorf("invalid user id in token")
	}
	return claims, nil
}
