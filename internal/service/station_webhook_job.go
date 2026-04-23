package service

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/google/uuid"

	"ev-charging-status-service/internal/clients/intelbras"
	"ev-charging-status-service/internal/repository"
)

// WebhookPayload é o JSON enviado para a URL do webhook a cada 3 minutos.
type WebhookPayload struct {
	UserID    string           `json:"userId"`
	Timestamp string           `json:"timestamp"`
	Stations  []WebhookStation `json:"stations"`
}

// WebhookStation representa uma estação no payload do webhook (connectors por último).
type WebhookStation struct {
	ChargeBoxID       string             `json:"chargeBoxId"`
	Description       string             `json:"description"`
	OcppProtocol      string             `json:"ocppProtocol"`
	ChargePointModel  string             `json:"chargePointModel"`
	ChargePointVendor string             `json:"chargePointVendor"`
	UUID              string             `json:"uuid"`
	FwVersion         string             `json:"fwVersion"`
	Connectors        []WebhookConnector `json:"connectors"`
}

// buildWebhookPayloadJSON monta o JSON do webhook com ordem fixa: userId, stations, timestamp.
func buildWebhookPayloadJSON(p WebhookPayload) ([]byte, error) {
	buf := &bytes.Buffer{}
	buf.WriteString(`{"userId":`)
	buf.Write(quoteString(p.UserID))
	buf.WriteString(`,"stations":[`)
	for i, st := range p.Stations {
		if i > 0 {
			buf.WriteByte(',')
		}
		buf.Write(buildStationJSON(st))
	}
	buf.WriteString(`],"timestamp":`)
	buf.Write(quoteString(p.Timestamp))
	buf.WriteString(`}`)
	return buf.Bytes(), nil
}

func quoteString(s string) []byte {
	b, _ := json.Marshal(s)
	return b
}

func buildStationJSON(s WebhookStation) []byte {
	buf := &bytes.Buffer{}
	buf.WriteString(`{"chargeBoxId":`)
	buf.Write(quoteString(s.ChargeBoxID))
	buf.WriteString(`,"description":`)
	buf.Write(quoteString(s.Description))
	buf.WriteString(`,"ocppProtocol":`)
	buf.Write(quoteString(s.OcppProtocol))
	buf.WriteString(`,"chargePointModel":`)
	buf.Write(quoteString(s.ChargePointModel))
	buf.WriteString(`,"chargePointVendor":`)
	buf.Write(quoteString(s.ChargePointVendor))
	buf.WriteString(`,"uuid":`)
	buf.Write(quoteString(s.UUID))
	buf.WriteString(`,"fwVersion":`)
	buf.Write(quoteString(s.FwVersion))
	buf.WriteString(`,"connectors":`)
	connEnc, _ := json.Marshal(s.Connectors)
	buf.Write(connEnc)
	buf.WriteString(`}`)
	return buf.Bytes()
}

// WebhookConnector representa um conector no payload do webhook (ordem dos campos definida).
type WebhookConnector struct {
	Status        string `json:"status"`
	ErroInfo      string `json:"erroInfo"`
	PowerMax      int    `json:"powerMax"`
	ErrorCode     string `json:"errorCode"`
	ConnectorID   int    `json:"connectorId"`
	ConnectorPK   int64  `json:"connectorPk"`
	ConnectorType string `json:"connectorType"`
}

func flattenToWebhookStations(list []intelbras.FlattenedChargePoint) []WebhookStation {
	out := make([]WebhookStation, 0, len(list))
	for _, cp := range list {
		conns := make([]WebhookConnector, 0, len(cp.Connectors))
		for _, c := range cp.Connectors {
			conns = append(conns, WebhookConnector{
				Status:        c.Status,
				ErroInfo:      c.ErroInfo,
				PowerMax:      c.PowerMax,
				ErrorCode:     c.ErrorCode,
				ConnectorID:   c.ConnectorID,
				ConnectorPK:   c.ConnectorPK,
				ConnectorType: c.ConnectorType,
			})
		}
		out = append(out, WebhookStation{
			ChargeBoxID:       cp.ChargeBoxID,
			Description:       cp.Description,
			OcppProtocol:      cp.OcppProtocol,
			ChargePointModel:  cp.ChargePointModel,
			ChargePointVendor: cp.ChargePointVendor,
			UUID:              cp.UUID,
			FwVersion:         cp.FwVersion,
			Connectors:        conns,
		})
	}
	return out
}

// StationWebhookJob orquestra: buscar estações na API de terceiros e enfileirar envio por webhook.
type StationWebhookJob struct {
	credsRepo       *repository.CredentialsRepository
	stationService  *StationService
	webhookRepo     *repository.WebhookRepository
	webhookService  *WebhookService
}

// NewStationWebhookJob cria o job que roda a cada 3 minutos.
func NewStationWebhookJob(
	credsRepo *repository.CredentialsRepository,
	stationService *StationService,
	webhookRepo *repository.WebhookRepository,
	webhookService *WebhookService,
) *StationWebhookJob {
	return &StationWebhookJob{
		credsRepo:      credsRepo,
		stationService: stationService,
		webhookRepo:    webhookRepo,
		webhookService: webhookService,
	}
}

// Run busca estações, monta o payload e enfileira um evento de webhook para cada URL ativa do usuário.
func (j *StationWebhookJob) Run(ctx context.Context) {
	log.Println("[station-webhook-job] run started")

	userIDs, err := j.credsRepo.ListDistinctUserIDs(ctx)
	if err != nil {
		log.Printf("[station-webhook-job] no credentials configured (run POST /v1/config first): %v", err)
		return
	}
	if len(userIDs) == 0 {
		log.Printf("[station-webhook-job] no credentials configured (run POST /v1/config first)")
		return
	}

	for _, userID := range userIDs {
		j.runForUser(ctx, userID)
	}
}

func (j *StationWebhookJob) runForUser(ctx context.Context, userID uuid.UUID) {
	stations, err := j.stationService.GetStationsByUserID(ctx, userID)
	if err != nil {
		log.Printf("[station-webhook-job] get stations failed userId=%s err=%v", userID, err)
		return
	}
	log.Printf("[station-webhook-job] got %d stations userId=%s", len(stations), userID)

	webhooks, err := j.webhookRepo.ListActiveByUserID(ctx, userID)
	if err != nil {
		log.Printf("[station-webhook-job] list webhooks failed userId=%s err=%v", userID, err)
		return
	}
	if len(webhooks) == 0 {
		log.Printf("[station-webhook-job] no active webhook for userId=%s", userID)
		return
	}
	// Um envio por usuário (evita duplicata se houver mais de um webhook ativo).
	webhooks = webhooks[:1]

	payload := WebhookPayload{
		UserID:    userID.String(),
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Stations:  flattenToWebhookStations(stations),
	}
	body, err := buildWebhookPayloadJSON(payload)
	if err != nil {
		log.Printf("[station-webhook-job] marshal payload userId=%s err=%v", userID, err)
		return
	}

	for _, w := range webhooks {
		if err := j.webhookService.EnqueueEvent(ctx, w, body); err != nil {
			log.Printf("[station-webhook-job] enqueue webhook id=%s userId=%s failed", w.ID, userID)
		} else {
			log.Printf("[station-webhook-job] webhook enqueued id=%s userId=%s (sender will POST in up to 30s)", w.ID, userID)
		}
	}
}
