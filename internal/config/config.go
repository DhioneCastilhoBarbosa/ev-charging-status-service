
package config

import (
	"os"
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
}

func Load() Config {
	encKey := crypto.KeyFromEnv(strings.TrimSpace(os.Getenv("ENCRYPTION_KEY")))
	return Config{
		PostgresURL:      os.Getenv("POSTGRES_URL"),
		RedisAddr:        os.Getenv("REDIS_ADDR"),
		KafkaBroker:      os.Getenv("KAFKA_BROKER"),
		IntelbrasBaseURL: os.Getenv("INTELBRAS_BASE_URL"),
		APIKey:           os.Getenv("API_KEY"),
		EncryptionKey:    encKey,
	}
}
