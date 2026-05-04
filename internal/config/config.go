
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
	// IntelbrasChargePointMaxRPM: máximo de GET /chargepoints por minuto neste processo (0 = sem limite). Default 55.
	IntelbrasChargePointMaxRPM int
	APIKey           string
	EncryptionKey    []byte
	WSJWTSecret      []byte
	WSTokenTTL       int
	WSPublishIntervalSeconds int
	// WSStationPollIntervalSeconds, se > 0, define o poll à API externa para o publisher WS; senão usa WSPublishIntervalSeconds.
	WSStationPollIntervalSeconds int
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
	// WS_PUBLISH_INTERVAL_SECONDS: fallback do poll WS quando WS_STATION_POLL_INTERVAL_SECONDS não está definido.
	wsPublishIntervalSec := 180
	if raw := strings.TrimSpace(os.Getenv("WS_PUBLISH_INTERVAL_SECONDS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			wsPublishIntervalSec = parsed
		}
	}
	wsStationPollSec := 0
	if raw := strings.TrimSpace(os.Getenv("WS_STATION_POLL_INTERVAL_SECONDS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			wsStationPollSec = parsed
		}
	}
	intelbrasChargePointMaxRPM := 55
	if raw := strings.TrimSpace(os.Getenv("INTELBRAS_CHARGEPOINT_MAX_RPM")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			intelbrasChargePointMaxRPM = parsed
		}
	}
	return Config{
		PostgresURL:      os.Getenv("POSTGRES_URL"),
		RedisAddr:        os.Getenv("REDIS_ADDR"),
		KafkaBroker:      os.Getenv("KAFKA_BROKER"),
		IntelbrasBaseURL:           os.Getenv("INTELBRAS_BASE_URL"),
		IntelbrasChargePointMaxRPM: intelbrasChargePointMaxRPM,
		APIKey:           os.Getenv("API_KEY"),
		EncryptionKey:    encKey,
		WSJWTSecret:      []byte(wsSecret),
		WSTokenTTL:                   wsTTLSec,
		WSPublishIntervalSeconds:     wsPublishIntervalSec,
		WSStationPollIntervalSeconds: wsStationPollSec,
	}
}

// WSStationPollSeconds intervalo (segundos) entre polls à API de estações para o publisher WebSocket.
func (c Config) WSStationPollSeconds() int {
	if c.WSStationPollIntervalSeconds > 0 {
		return c.WSStationPollIntervalSeconds
	}
	return c.WSPublishIntervalSeconds
}
