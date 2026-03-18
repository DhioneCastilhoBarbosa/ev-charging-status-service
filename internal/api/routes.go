
package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"ev-charging-status-service/internal/clients/intelbras"
	"ev-charging-status-service/internal/config"
	"ev-charging-status-service/internal/repository"
	"ev-charging-status-service/internal/service"
)

// healthHandler responde ao health check.
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
	intelbrasClient := intelbras.NewClient(cfg.IntelbrasBaseURL, 15*time.Second)

	userRepo := repository.NewUserRepository(db)
	credsRepo := repository.NewCredentialsRepository(db)
	webhookRepo := repository.NewWebhookRepository(db)

	configService := service.NewConfigService(userRepo, credsRepo, webhookRepo, intelbrasClient, cfg.EncryptionKey)
	configHandler := NewConfigHandler(configService, cfg.APIKey)

	stationService := service.NewStationService(credsRepo, intelbrasClient, cfg.EncryptionKey)
	stationsHandler := NewStationsHandler(stationService, cfg.APIKey)

	v1 := router.Group("/v1")
	v1.Use(RateLimitMiddleware(0.25, 10)) // 15 req/min por IP, burst 10
	configHandler.RegisterRoutes(v1)
	stationsHandler.RegisterRoutes(v1)

	return router
}
