package intelbras

import (
	"net/http"
	"testing"
)

func TestNewAPIErrorParsesRateLimitMessage(t *testing.T) {
	body := []byte(`{"message":"Rate limit exceeded. Try again in 1687 seconds.","status":429,"error":"Too Many Requests"}`)
	err := newAPIError(http.StatusTooManyRequests, body)

	if err.StatusCode != http.StatusTooManyRequests {
		t.Fatalf("status: got %d want %d", err.StatusCode, http.StatusTooManyRequests)
	}
	want := "Rate limit exceeded. Try again in 1687 seconds."
	if err.Message != want {
		t.Fatalf("message: got %q want %q", err.Message, want)
	}
}

func TestNewAPIErrorFallbackStatusText(t *testing.T) {
	err := newAPIError(http.StatusTooManyRequests, nil)
	if err.Message != http.StatusText(http.StatusTooManyRequests) {
		t.Fatalf("message: got %q", err.Message)
	}
}
