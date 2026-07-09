package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"ev-charging-status-service/internal/clients/intelbras"
)

const upstreamUnauthorizedMessage = "Usuário sem autorização na plataforma CVE-Pro"

func respondServiceError(c *gin.Context, err error, defaultStatus int, defaultMessage string) {
	var apiErr *intelbras.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.StatusCode {
		case http.StatusUnauthorized:
			c.JSON(http.StatusUnauthorized, ErrorResponse{Error: upstreamUnauthorizedMessage})
			return
		case http.StatusTooManyRequests:
			c.JSON(http.StatusTooManyRequests, ErrorResponse{Error: apiErr.Message})
			return
		}
	}
	c.JSON(defaultStatus, ErrorResponse{Error: defaultMessage})
}
