package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"ev-charging-status-service/internal/service"
)

type ConfigHandler struct {
	service  *service.ConfigService
	apiKey   string
	timeout  time.Duration
}

type configRequest struct {
	Email              string  `json:"email"`
	Username           string  `json:"username"`
	Password           string  `json:"password" binding:"required"`
	RecaptchaResponse  string  `json:"recaptchaResponse"`
	APIKey             *string `json:"apiKey"`
	WebhookURL         string  `json:"webhookUrl" binding:"required,url"`
}

type deleteConfigRequest struct {
	Email    string `json:"email"`
	Username string `json:"username"`
}

func NewConfigHandler(s *service.ConfigService, apiKey string) *ConfigHandler {
	return &ConfigHandler{
		service: s,
		apiKey:  apiKey,
		timeout: 30 * time.Second,
	}
}

func (h *ConfigHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/config", h.handleConfig)
	rg.GET("/config/status", h.handleConfigStatus)
	rg.DELETE("/config", h.handleDeleteConfig)
}

// handleConfig configura credenciais e URL do webhook.
//
//	@Summary		Configura credenciais e webhook
//	@Description	Faz login na API Move/Intelbras, persiste credenciais e salva a URL para envio do webhook.
//	@Tags			config
//	@Accept			json
//	@Produce		json
//	@Param			body	body		object	true	"email ou username, password, webhookUrl (obrigatório), apiKey e recaptchaResponse (opcionais)"
//	@Success		204		"No content"
//	@Failure		400		{object}	object	"invalid request"
//	@Failure		401		{object}	object	"unauthorized"
//	@Failure		502		{object}	object	"configuration failed"
//	@Security		ApiKeyAuth
//	@Router			/v1/config [post]
func (h *ConfigHandler) handleConfig(c *gin.Context) {
	if h.apiKey != "" {
		if c.GetHeader("X-API-Key") != h.apiKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
	}

	var req configRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[config] bind error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	// Login na API de terceiros exige email; aceita "email" ou "username" no body
	if req.Email == "" && req.Username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "email or username is required"})
		return
	}
	loginEmail := req.Email
	if loginEmail == "" {
		loginEmail = req.Username
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	input := service.ConfigInput{
		Email:              loginEmail,
		Password:           req.Password,
		RecaptchaResponse:  req.RecaptchaResponse,
		APIKey:             req.APIKey,
		WebhookURL:         req.WebhookURL,
	}

	if err := h.service.Configure(ctx, input); err != nil {
		log.Printf("[config] configure error: %v", err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "configuration failed"})
		return
	}

	c.Status(http.StatusNoContent)
}

// handleConfigStatus retorna o status da configuração (sem expor token).
//
//	@Summary		Status da configuração
//	@Description	Indica se há configuração e se o token de acesso está presente.
//	@Tags			config
//	@Produce		json
//	@Success		200	{object}	object	"configured, tokenPresent, tokenExpiresAt, apiUsername"
//	@Failure		401	{object}	object	"unauthorized"
//	@Failure		500	{object}	object	"configuration unavailable"
//	@Security		ApiKeyAuth
//	@Router			/v1/config/status [get]
func (h *ConfigHandler) handleConfigStatus(c *gin.Context) {
	if h.apiKey != "" {
		if c.GetHeader("X-API-Key") != h.apiKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()
	st, err := h.service.GetConfigStatus(ctx)
	if err != nil {
		log.Printf("[config] get status error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "configuration unavailable"})
		return
	}
	c.JSON(http.StatusOK, st)
}

// handleDeleteConfig remove um usuário (por email/username) e todos os dados relacionados.
//
//	@Summary		Remove configuração e dados do usuário
//	@Description	Apaga o usuário indicado e todos os dados relacionados (credenciais, webhooks, eventos) via cascade.
//	@Tags			config
//	@Accept			json
//	@Success		204	"No content"
//	@Failure		401	{object}	object	"unauthorized"
//	@Security		ApiKeyAuth
//	@Router			/v1/config [delete]
func (h *ConfigHandler) handleDeleteConfig(c *gin.Context) {
	if h.apiKey != "" {
		if c.GetHeader("X-API-Key") != h.apiKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
	}

	var req deleteConfigRequest
	_ = c.ShouldBindJSON(&req)
	email := req.Email
	if email == "" {
		email = req.Username
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := h.service.DeleteUserByEmail(ctx, email); err != nil {
		log.Printf("[config] delete user error: %v", err)
		// não expor detalhes; manter 204 para tornar operação idempotente
	}
	c.Status(http.StatusNoContent)
}

