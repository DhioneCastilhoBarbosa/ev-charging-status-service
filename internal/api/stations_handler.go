package api

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"ev-charging-status-service/internal/service"
)

type StationsHandler struct {
	service *service.StationService
	wsAuth  *service.WSAuthService
	apiKey  string
	timeout time.Duration
}

// StationsPostRequest corpo de POST /v1/stations: apiKey informada no /v1/config (JWT vai no header Authorization).
type StationsPostRequest struct {
	APIKey string `json:"apiKey" example:"sua-api-key-intelbras"`
}

func NewStationsHandler(s *service.StationService, wsAuth *service.WSAuthService, apiKey string) *StationsHandler {
	return &StationsHandler{
		service: s,
		wsAuth:  wsAuth,
		apiKey:  apiKey,
		timeout: 30 * time.Second,
	}
}

func (h *StationsHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/stations", h.handlePostStations)
}

// handlePostStations lista estações (userId, timestamp, stations), após validar JWT + apiKey da config.
//
//	@Summary		Lista estações (Authorization: Bearer + apiKey no corpo)
//	@Description	Valida o JWT (header `Authorization: Bearer`) retornado por `POST /v1/config` e confere `apiKey` do JSON com a salva no configure. O corpo da resposta segue o mesmo esquema dos frames JSON do WebSocket de estações.
//	@Tags			Estação
//	@Accept			json
//	@Produce		json
//	@Param			Authorization	header		string				true	"Bearer &lt;JWT&gt;"
//	@Param			body			body		StationsPostRequest	true	"apiKey (igual ao /v1/config; use string vazia ou omita se não houver apiKey na config)"
//	@Success		200				{object}	StationsPushResponse	"userId, timestamp, stations"
//	@Failure		400				{object}	ErrorResponse	"invalid request"
//	@Failure		401				{object}	ErrorResponse	"unauthorized"
//	@Failure		502				{object}	ErrorResponse	"stations unavailable"
//	@Security		ApiKeyAuth
//	@Router			/v1/stations [post]
func (h *StationsHandler) handlePostStations(c *gin.Context) {
	if h.apiKey != "" {
		if c.GetHeader("X-API-Key") != h.apiKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
	}

	tokenStr := strings.TrimSpace(c.GetHeader("Authorization"))
	if len(tokenStr) > 7 && strings.EqualFold(tokenStr[:7], "bearer ") {
		tokenStr = strings.TrimSpace(tokenStr[7:])
	}
	if tokenStr == "" {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req StationsPostRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[stations] bind error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	claims, err := h.wsAuth.ValidateToken(tokenStr)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	if err := h.service.VerifyIntelbrasAPIKeyForUser(ctx, userID, req.APIKey); err != nil {
		if errors.Is(err, service.ErrIntelbrasAPIKeyMismatch) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
		log.Printf("[stations] verify api key error: %v", err)
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	payload, err := h.service.BuildStationsPushPayload(ctx, userID)
	if err != nil {
		log.Printf("[stations] get error: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "stations unavailable"})
		return
	}

	c.JSON(http.StatusOK, payload)
}
