
package main

import (
	"context"
	"log"
	"time"

	"ev-charging-status-service/internal/clients/intelbras"
	"ev-charging-status-service/internal/config"
	"ev-charging-status-service/internal/database"
	"ev-charging-status-service/internal/repository"
	"ev-charging-status-service/internal/service"
)

func main() {
	log.Println("Worker started")

	cfg := config.Load()
	db := database.ConnectPostgres(cfg.PostgresURL)

	credsRepo := repository.NewCredentialsRepository(db)
	webhookRepo := repository.NewWebhookRepository(db)
	webhookEventRepo := repository.NewWebhookEventRepository(db)

	intelbrasClient := intelbras.NewClient(cfg.IntelbrasBaseURL, 15*time.Second)
	stationService := service.NewStationService(credsRepo, intelbrasClient, cfg.EncryptionKey)
	webhookService := service.NewWebhookService(webhookRepo, webhookEventRepo)
	stationWebhookJob := service.NewStationWebhookJob(credsRepo, stationService, webhookRepo, webhookService)

	scheduler := service.NewSchedulerService(3 * time.Minute)
	ctx := context.Background()

	go webhookService.RunSender(ctx, 30*time.Second, 5)

	// Roda o job uma vez ao subir; depois a cada 3 minutos.
	log.Println("[worker] running station-webhook job once on startup...")
	stationWebhookJob.Run(ctx)

	scheduler.Run(ctx, stationWebhookJob.Run)
}
