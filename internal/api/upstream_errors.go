package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"ev-charging-status-service/internal/clients/intelbras"
)

func respondServiceError(c *gin.Context, err error, defaultStatus int, defaultMessage string) {
	var apiErr *intelbras.APIError
	if errors.As(err, &apiErr) && apiErr.StatusCode == http.StatusTooManyRequests {
		c.JSON(http.StatusTooManyRequests, ErrorResponse{Error: apiErr.Message})
		return
	}
	c.JSON(defaultStatus, ErrorResponse{Error: defaultMessage})
}
