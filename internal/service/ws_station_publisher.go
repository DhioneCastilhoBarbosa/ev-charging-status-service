package service

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"

	"ev-charging-status-service/internal/repository"
)

type UserPublisher interface {
	PublishToUser(userID string, payload []byte)
}

type WSStationPublisher struct {
	credsRepo      *repository.CredentialsRepository
	stationService *StationService
	publisher      UserPublisher
}

func NewWSStationPublisher(
	credsRepo *repository.CredentialsRepository,
	stationService *StationService,
	publisher UserPublisher,
) *WSStationPublisher {
	return &WSStationPublisher{
		credsRepo:      credsRepo,
		stationService: stationService,
		publisher:      publisher,
	}
}

func (p *WSStationPublisher) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Primeiro ciclo imediato para clientes recém conectados.
	p.publishOnce(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.publishOnce(ctx)
		}
	}
}

func (p *WSStationPublisher) publishOnce(ctx context.Context) {
	userIDs, err := p.credsRepo.ListDistinctUserIDs(ctx)
	if err != nil {
		log.Printf("[ws-publisher] list credential users failed: %v", err)
		return
	}
	for _, userID := range userIDs {
		p.publishForUser(ctx, userID)
	}
}

func (p *WSStationPublisher) publishForUser(ctx context.Context, userID uuid.UUID) {
	stations, err := p.stationService.GetStationsByUserID(ctx, userID)
	if err != nil {
		log.Printf("[ws-publisher] get stations failed userId=%s err=%v", userID, err)
		return
	}
	payload := WebhookPayload{
		UserID:    userID.String(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Stations:  flattenToWebhookStations(stations),
	}
	body, err := buildWebhookPayloadJSON(payload)
	if err != nil {
		log.Printf("[ws-publisher] marshal payload failed userId=%s err=%v", userID, err)
		return
	}
	p.publisher.PublishToUser(userID.String(), body)
}
