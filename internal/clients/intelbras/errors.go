package intelbras

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

// APIError representa uma resposta de erro HTTP da API Move/Intelbras.
type APIError struct {
	StatusCode int
	Message    string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("intelbras api error %d: %s", e.StatusCode, e.Message)
}

type upstreamErrorBody struct {
	Message string `json:"message"`
	Status  int    `json:"status"`
	Error   string `json:"error"`
}

func newAPIError(statusCode int, body []byte) *APIError {
	msg := extractUpstreamMessage(statusCode, body)
	return &APIError{
		StatusCode: statusCode,
		Message:    msg,
	}
}

func extractUpstreamMessage(statusCode int, body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return http.StatusText(statusCode)
	}

	var parsed upstreamErrorBody
	if err := json.Unmarshal(body, &parsed); err == nil {
		if m := strings.TrimSpace(parsed.Message); m != "" {
			return m
		}
		if e := strings.TrimSpace(parsed.Error); e != "" {
			return e
		}
	}

	return trimmed
}
