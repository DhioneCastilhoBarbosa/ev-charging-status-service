package intelbras

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type Client struct {
	baseURL    string
	httpClient *http.Client
}

// LoginRequest é o body da API de terceiros (Move/Intelbras).
type LoginRequest struct {
	Email            string `json:"email"`
	Password         string `json:"password"`
	RecaptchaResponse string `json:"recaptchaResponse"`
	// APIKey não vai no body; é enviada no header API-Key.
	APIKey string `json:"-"`
}

type LoginResponse struct {
	AccessToken string    `json:"accessToken"`
	Token       string    `json:"token"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

func NewClient(baseURL string, timeout time.Duration) *Client {
	return &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (c *Client) Login(ctx context.Context, req LoginRequest) (*LoginResponse, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal login request: %w", err)
	}

	const maxAttempts = 3
	var lastErr error

	for attempt := 0; attempt < maxAttempts; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/login", bytes.NewReader(payload))
		if err != nil {
			return nil, fmt.Errorf("create login request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Accept", "*/*")
		httpReq.Header.Set("Platform", "API")
		httpReq.Header.Set("API-Key", req.APIKey)

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			lastErr = fmt.Errorf("execute login request: %w", err)
			// Retry apenas para erros transitórios de rede (inclui TLS handshake timeout)
			if attempt < maxAttempts-1 && (isTimeoutErr(err) || isTLSHandshakeTimeout(err)) {
				time.Sleep(time.Duration(1<<attempt) * time.Second)
				continue
			}
			return nil, lastErr
		}

		respBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			// Não expor o body ao cliente; logar só no servidor para debug
			if len(respBody) > 0 {
				log.Printf("[intelbras] login failed %d: %s", resp.StatusCode, string(respBody))
			}

			// Retry para rate limit/indisponibilidade.
			if attempt < maxAttempts-1 && (resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusServiceUnavailable) {
				delay := retryAfterDelay(resp.Header.Get("Retry-After"), time.Duration(1<<attempt)*time.Second)
				time.Sleep(delay)
				continue
			}

			lastErr = fmt.Errorf("login failed with status %d", resp.StatusCode)
			return nil, lastErr
		}

		var lr LoginResponse
		if err := json.Unmarshal(respBody, &lr); err != nil {
			lastErr = fmt.Errorf("decode login response: %w", err)
			return nil, lastErr
		}
		if lr.AccessToken == "" && lr.Token != "" {
			lr.AccessToken = lr.Token
		}
		return &lr, nil
	}

	return nil, lastErr
}

func isTimeoutErr(err error) bool {
	var ne net.Error
	return errors.As(err, &ne) && ne.Timeout()
}

func isTLSHandshakeTimeout(err error) bool {
	return strings.Contains(err.Error(), "TLS handshake timeout")
}

func retryAfterDelay(retryAfter string, fallback time.Duration) time.Duration {
	if retryAfter == "" {
		return fallback
	}
	// retry-after pode ser segundos
	if sec, err := strconv.Atoi(strings.TrimSpace(retryAfter)); err == nil && sec > 0 {
		return time.Duration(sec) * time.Second
	}
	// ou timestamp HTTP (vamos manter simples: usar fallback)
	return fallback
}

