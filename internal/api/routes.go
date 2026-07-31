package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"ev-charging-status-service/internal/clients/csmsstomp"
	"ev-charging-status-service/internal/clients/intelbras"
	"ev-charging-status-service/internal/config"
	"ev-charging-status-service/internal/repository"
	"ev-charging-status-service/internal/service"
)

// healthHandler responde ao health check.
//
//	@Summary		Health check
//	@Description	Verifica se o serviço está ativo.
//	@Tags			Health
//	@Produce		json
//	@Success		200	{object}	HealthResponse	"OK"
//	@Router			/health [get]
func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

// SetupRoutes configura o roteador HTTP principal.
func SetupRoutes(db *sqlx.DB, cfg config.Config) *gin.Engine {
	router := gin.Default()

	// Healthcheck e Swagger
	router.GET("/health", healthHandler)
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Dependências de domínio
	intelbrasClient := intelbras.NewClient(cfg.IntelbrasBaseURL, 30*time.Second).
		WithChargePointListRateLimit(cfg.IntelbrasChargePointMaxRPM)

	userRepo := repository.NewUserRepository(db)
	credsRepo := repository.NewCredentialsRepository(db)

	configService := service.NewConfigService(userRepo, credsRepo, intelbrasClient, cfg.EncryptionKey)
	wsAuth := service.NewWSAuthService(cfg.WSJWTSecret, cfg.WSIdleTimeoutSeconds, userRepo, credsRepo)
	wsHub := NewWSHub()
	configHandler := NewConfigHandler(configService, wsAuth, wsHub, cfg.APIKey)

	stationService := service.NewStationService(credsRepo, intelbrasClient, cfg.EncryptionKey)
	stationsHandler := NewStationsHandler(stationService, wsAuth, cfg.APIKey)
	wsStatusStore := service.NewInMemoryConnectorStatusStore()
	wsPublisher := service.NewWSStationPublisher(stationService, wsHub, wsStatusStore)
	wsHandler := NewWSHandler(cfg.APIKey, wsAuth, userRepo, credsRepo, wsHub, func(ctx context.Context, userID string) {
		wsPublisher.OnWebSocketConnected(ctx, userID)
	})

	v1 := router.Group("/v1")
	v1.Use(RateLimitMiddleware(0.25, 10)) // 15 req/min por IP, burst 10
	configHandler.RegisterRoutes(v1)
	stationsHandler.RegisterRoutes(v1)
	wsHandler.RegisterRoutes(v1)

	// Snapshot inicial no WebSocket: um GET /chargepoints ao conectar (OnWebSocketConnected).
	// Atualizações: apenas CSMS STOMP (sem poll periódico — evita rate limit).

	if cfg.CSMSSTOMPEnabled {
		host, useTLS, err := csmsstomp.ParseAPIHost(cfg.IntelbrasBaseURL)
		if err != nil {
			log.Printf("[status-subscriber] disabled: invalid INTELBRAS_BASE_URL: %v", err)
		} else {
			prefix := cfg.CSMSSockJSPrefix
			sub := service.NewCSMSStatusSubscriber(credsRepo, stationService, wsHub, host, useTLS, prefix, 10*time.Minute)
			go sub.Run(context.Background())
			log.Printf("[status-subscriber] CSMS STOMP enabled host=%s tls=%v prefix=%q", host, useTLS, prefix)
		}
	} else {
		log.Printf("[ws] CSMS STOMP disabled: WebSocket sends only the initial snapshot on connect (no incremental push)")
	}

	return router
}
