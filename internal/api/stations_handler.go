package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"ev-charging-status-service/internal/service"
)

type StationsHandler struct {
	service *service.StationService
	apiKey  string
	timeout time.Duration
}

func NewStationsHandler(s *service.StationService, apiKey string) *StationsHandler {
	return &StationsHandler{
		service: s,
		apiKey:  apiKey,
		timeout: 20 * time.Second,
	}
}

func (h *StationsHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/stations", h.handleGetStations)
}

// handleGetStations lista estações e conectores da API de terceiros.
//
//	@Summary		Lista estações
//	@Description	Retorna a lista de estações de recarga (e conectores) da API Move/Intelbras, usando o token salvo ou renovando-o.
//	@Tags			stations
//	@Produce		json
//	@Success		200	{object}	object	"stations: array de estações"
//	@Failure		401	{object}	object	"unauthorized"
//	@Failure		502	{object}	object	"stations unavailable"
//	@Security		ApiKeyAuth
//	@Router			/v1/stations [get]
func (h *StationsHandler) handleGetStations(c *gin.Context) {
	if h.apiKey != "" {
		if c.GetHeader("X-API-Key") != h.apiKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	list, err := h.service.GetStations(ctx)
	if err != nil {
		log.Printf("[stations] get error: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "stations unavailable"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"stations": list})
}
