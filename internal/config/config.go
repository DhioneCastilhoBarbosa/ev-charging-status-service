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
	APIKey                     string
	EncryptionKey              []byte
	WSJWTSecret []byte
	// WSIdleTimeoutSeconds: sem tráfego de aplicação no WebSocket (não ping/pong) por este período → fecha WS e invalida JWT (default 3600).
	WSIdleTimeoutSeconds int
	// CSMSSTOMPEnabled: assina /topic/status/chargeBox/{uuid} no CSMS e publica mudanças no WebSocket (default true).
	CSMSSTOMPEnabled bool
	// CSMSSockJSPrefix: prefixo SockJS no host Move (default /ws).
	CSMSSockJSPrefix string
	// StationsInventoryPollSeconds: intervalo para detectar estação add/remove e publicar no WS (default 15).
	StationsInventoryPollSeconds int
}

func Load() Config {
	encKey := crypto.KeyFromEnv(strings.TrimSpace(os.Getenv("ENCRYPTION_KEY")))
	wsSecret := strings.TrimSpace(os.Getenv("WS_JWT_SECRET"))
	if wsSecret == "" {
		// Fallback para facilitar rollout inicial quando somente API_KEY existe.
		wsSecret = os.Getenv("API_KEY")
	}
	wsIdleSec := 3600
	if raw := strings.TrimSpace(os.Getenv("WS_IDLE_TIMEOUT_SECONDS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			wsIdleSec = parsed
		}
	}
	intelbrasChargePointMaxRPM := 55
	if raw := strings.TrimSpace(os.Getenv("INTELBRAS_CHARGEPOINT_MAX_RPM")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			intelbrasChargePointMaxRPM = parsed
		}
	}
	csmsStompEnabled := true
	if v := strings.TrimSpace(os.Getenv("CSMS_STATUS_STOMP_ENABLED")); v != "" {
		switch strings.ToLower(v) {
		case "0", "false", "no", "off":
			csmsStompEnabled = false
		case "1", "true", "yes", "on":
			csmsStompEnabled = true
		}
	}
	csmsSockPrefix := strings.TrimSpace(os.Getenv("CSMS_SOCKJS_PREFIX"))
	inventoryPollSec := 15
	if raw := strings.TrimSpace(os.Getenv("STATIONS_INVENTORY_POLL_SECONDS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			inventoryPollSec = parsed
		}
	}
	return Config{
		PostgresURL:                  os.Getenv("POSTGRES_URL"),
		RedisAddr:                    os.Getenv("REDIS_ADDR"),
		KafkaBroker:                  os.Getenv("KAFKA_BROKER"),
		IntelbrasBaseURL:             os.Getenv("INTELBRAS_BASE_URL"),
		IntelbrasChargePointMaxRPM:   intelbrasChargePointMaxRPM,
		APIKey:                       os.Getenv("API_KEY"),
		EncryptionKey:                encKey,
		WSJWTSecret:                  []byte(wsSecret),
		WSIdleTimeoutSeconds:         wsIdleSec,
		CSMSSTOMPEnabled:             csmsStompEnabled,
		CSMSSockJSPrefix:             csmsSockPrefix,
		StationsInventoryPollSeconds: inventoryPollSec,
	}
}
