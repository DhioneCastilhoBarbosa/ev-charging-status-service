package api

import (
	"context"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"ev-charging-status-service/internal/service"
)

type ConfigHandler struct {
	service  *service.ConfigService
	wsAuth   *service.WSAuthService
	apiKey   string
	tokenTTL int
	timeout  time.Duration
}

type ConfigRequest struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
	// Ignorado apenas na geração Swagger/OpenAPI (o campo continua aceito no body do endpoint).
	RecaptchaResponse string  `json:"recaptchaResponse" swaggerignore:"true"`
	APIKey            *string `json:"apiKey"`
}

func NewConfigHandler(
	s *service.ConfigService,
	wsAuth *service.WSAuthService,
	apiKey string,
	tokenTTL int,
) *ConfigHandler {
	return &ConfigHandler{
		service:  s,
		wsAuth:   wsAuth,
		apiKey:   apiKey,
		tokenTTL: tokenTTL,
		timeout:  30 * time.Second,
	}
}

func (h *ConfigHandler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.POST("/config", h.handleConfig)
	rg.GET("/config/status", h.handleConfigStatus)
	rg.DELETE("/config", h.handleDeleteConfig)
}

// handleConfig configura credenciais na Intelbras; retorna JWT de sessão.
//
//	@Summary		Configura credenciais (e token JWT)
//	@Description	Faz login na API Move/Intelbras, persiste credenciais (criptografadas se `ENCRYPTION_KEY` existir). Resposta inclui `token` (JWT) e `expiresIn` (segundos, ver `WS_TOKEN_TTL_SECONDS`). Use o JWT no header `Authorization: Bearer` em `POST /v1/stations` e `DELETE /v1/config`. O JWT não substitui `X-API-Key` nas rotas que exigem esse header.
//	@Tags			Configuração
//	@Accept			json
//	@Produce		json
//	@Param			body	body		ConfigRequest	true	"email e password (obrigatórios); apiKey (opcional)"
//	@Success		200		{object}	ConfigResponse	"token, expiresIn"
//	@Failure		400		{object}	ErrorResponse	"invalid request"
//	@Failure		401		{object}	ErrorResponse	"unauthorized"
//	@Failure		500		{object}	ErrorResponse	"token unavailable"
//	@Failure		429		{object}	ErrorResponse	"rate limit exceeded (API de terceiros)"
//	@Failure		502		{object}	ErrorResponse	"configuration failed"
//	@Security		ApiKeyAuth
//	@Router			/v1/config [post]
func (h *ConfigHandler) handleConfig(c *gin.Context) {
	if h.apiKey != "" {
		if c.GetHeader("X-API-Key") != h.apiKey {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			return
		}
	}

	var req ConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		log.Printf("[config] bind error: %v", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), h.timeout)
	defer cancel()

	input := service.ConfigInput{
		Email:             req.Email,
		Password:          req.Password,
		RecaptchaResponse: req.RecaptchaResponse,
		APIKey:            req.APIKey,
	}

	user, err := h.service.Configure(ctx, input)
	if err != nil {
		log.Printf("[config] configure error: %v", err)
		respondServiceError(c, err, http.StatusBadGateway, "configuration failed")
		return
	}

	// Gera token WS imediatamente após configurar, para reduzir round-trip do cliente.
	token, err := h.wsAuth.GenerateToken(user.ID, user.Username)
	if err != nil {
		log.Printf("[config] token generation error: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "token unavailable"})
		return
	}

	c.JSON(http.StatusOK, ConfigResponse{
		Token:     token,
		ExpiresIn: h.tokenTTL,
	})
}

// handleConfigStatus retorna o status da configuração (sem expor token).
//
//	@Summary		Status da configuração
//	@Description	Indica se há configuração e se o token de acesso está presente.
//	@Tags			Configuração
//	@Produce		json
//	@Success		200	{object}	ConfigStatusResponse	"configured, tokenPresent, tokenExpiresAt, apiUsername"
//	@Failure		401	{object}	ErrorResponse	"unauthorized"
//	@Failure		500	{object}	ErrorResponse	"configuration unavailable"
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

// handleDeleteConfig remove o usuário identificado pelo JWT (Authorization: Bearer) e dados relacionados.
//
//	@Summary		Remove configuração e dados do usuário
//	@Description	Apaga o usuário identificado pelo JWT retornado em `POST /v1/config` (header `Authorization: Bearer`) e todos os dados relacionados (credenciais, etc.) via cascade.
//	@Tags			Configuração
//	@Param			Authorization	header		string	true	"Bearer &lt;JWT&gt;"
//	@Success		204				"No content"
//	@Failure		401				{object}	ErrorResponse	"unauthorized"
//	@Security		ApiKeyAuth
//	@Router			/v1/config [delete]
func (h *ConfigHandler) handleDeleteConfig(c *gin.Context) {
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

	ctx, cancel := context.WithTimeout(c.Request.Context(), 10*time.Second)
	defer cancel()
	if err := h.service.DeleteUserByID(ctx, userID); err != nil {
		log.Printf("[config] delete user error: %v", err)
		// não expor detalhes; manter 204 para tornar operação idempotente
	}
	c.Status(http.StatusNoContent)
}
