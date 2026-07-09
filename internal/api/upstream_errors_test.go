package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"ev-charging-status-service/internal/clients/intelbras"
)

func TestRespondServiceErrorUpstreamUnauthorized(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)

	err := fmt.Errorf("login intelbras: %w", &intelbras.APIError{
		StatusCode: http.StatusUnauthorized,
		Message:    "Invalid credentials",
	})
	respondServiceError(c, err, http.StatusBadGateway, "configuration failed")

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status: got %d want %d", w.Code, http.StatusUnauthorized)
	}
	var body ErrorResponse
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Error != "Usuário sem autorização na plataforma CVE-Pro" {
		t.Fatalf("error: got %q", body.Error)
	}
}
