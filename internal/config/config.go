
package config

import (
	"os"
	"strconv"
	"strings"

	"ev-charging-status-service/internal/crypto"
)

type Config struct {
	PostgresURL      string
	RedisAddr        string
	KafkaBroker      string
	IntelbrasBaseURL string
	APIKey           string
	EncryptionKey    []byte
	WSJWTSecret      []byte
	WSTokenTTL       int
	WSPublishIntervalSeconds int
}

func Load() Config {
	encKey := crypto.KeyFromEnv(strings.TrimSpace(os.Getenv("ENCRYPTION_KEY")))
	wsSecret := strings.TrimSpace(os.Getenv("WS_JWT_SECRET"))
	if wsSecret == "" {
		// Fallback para facilitar rollout inicial quando somente API_KEY existe.
		wsSecret = os.Getenv("API_KEY")
	}
	wsTTLSec := 300
	if raw := strings.TrimSpace(os.Getenv("WS_TOKEN_TTL_SECONDS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			wsTTLSec = parsed
		}
	}
	// Alinhado ao job do worker (3 min): uma coleta por ciclo, um envio WS por usuário.
	wsPublishIntervalSec := 180
	if raw := strings.TrimSpace(os.Getenv("WS_PUBLISH_INTERVAL_SECONDS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			wsPublishIntervalSec = parsed
		}
	}
	return Config{
		PostgresURL:      os.Getenv("POSTGRES_URL"),
		RedisAddr:        os.Getenv("REDIS_ADDR"),
		KafkaBroker:      os.Getenv("KAFKA_BROKER"),
		IntelbrasBaseURL: os.Getenv("INTELBRAS_BASE_URL"),
		APIKey:           os.Getenv("API_KEY"),
		EncryptionKey:    encKey,
		WSJWTSecret:      []byte(wsSecret),
		WSTokenTTL:       wsTTLSec,
		WSPublishIntervalSeconds: wsPublishIntervalSec,
	}
}
