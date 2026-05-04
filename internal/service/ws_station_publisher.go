package service

import (
	"context"
	"log"
	"time"

	"github.com/google/uuid"

	"ev-charging-status-service/internal/clients/intelbras"
	"ev-charging-status-service/internal/repository"
)

type UserPublisher interface {
	PublishToUser(userID string, payload []byte)
}

// StationListProvider obtém estações achatadas por usuário (permite testes com stub).
type StationListProvider interface {
	GetStationsByUserID(ctx context.Context, userID uuid.UUID) ([]intelbras.FlattenedChargePoint, error)
}

type WSStationPublisher struct {
	credsRepo      *repository.CredentialsRepository
	stationService StationListProvider
	publisher      UserPublisher
	statusStore    ConnectorStatusStore
}

func NewWSStationPublisher(
	credsRepo *repository.CredentialsRepository,
	stationService StationListProvider,
	publisher UserPublisher,
	statusStore ConnectorStatusStore,
) *WSStationPublisher {
	if statusStore == nil {
		statusStore = NewInMemoryConnectorStatusStore()
	}
	return &WSStationPublisher{
		credsRepo:      credsRepo,
		stationService: stationService,
		publisher:      publisher,
		statusStore:    statusStore,
	}
}

func (p *WSStationPublisher) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

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
		p.publishForUserIfStatusChanged(ctx, userID)
	}
}

func (p *WSStationPublisher) publishForUserIfStatusChanged(ctx context.Context, userID uuid.UUID) {
	stations, err := p.stationService.GetStationsByUserID(ctx, userID)
	if err != nil {
		log.Printf("[ws-publisher] get stations failed userId=%s err=%v", userID, err)
		return
	}
	newStatus := ConnectorStatusMapFromFlattened(stations)
	prev, hasPrev := p.statusStore.Get(ctx, userID)
	if hasPrev && ConnectorStatusMapsEqual(prev, newStatus) {
		return
	}
	p.publishPayloadAndPersistStatus(ctx, userID, stations, newStatus)
}

// OnWebSocketConnected envia snapshot completo e atualiza o store (novo cliente WS).
func (p *WSStationPublisher) OnWebSocketConnected(ctx context.Context, userIDStr string) {
	uid, err := uuid.Parse(userIDStr)
	if err != nil {
		log.Printf("[ws-publisher] on connect invalid userId=%q: %v", userIDStr, err)
		return
	}
	stations, err := p.stationService.GetStationsByUserID(ctx, uid)
	if err != nil {
		log.Printf("[ws-publisher] connect snapshot get stations failed userId=%s err=%v", uid, err)
		return
	}
	newStatus := ConnectorStatusMapFromFlattened(stations)
	p.publishPayloadAndPersistStatus(ctx, uid, stations, newStatus)
}

func (p *WSStationPublisher) publishPayloadAndPersistStatus(ctx context.Context, userID uuid.UUID, stations []intelbras.FlattenedChargePoint, newStatus map[string]string) {
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
	if err := p.statusStore.Set(ctx, userID, newStatus); err != nil {
		log.Printf("[ws-publisher] status store set failed userId=%s err=%v", userID, err)
	}
}
