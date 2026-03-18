package intelbras

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
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
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal login request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/login", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create login request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "*/*")
	httpReq.Header.Set("Platform", "API")
	httpReq.Header.Set("API-Key", req.APIKey)

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("execute login request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		respBody, _ := io.ReadAll(resp.Body)
		// Não expor o body ao cliente; logar só no servidor para debug
		if len(respBody) > 0 {
			log.Printf("[intelbras] login failed %d: %s", resp.StatusCode, string(respBody))
		}
		return nil, fmt.Errorf("login failed with status %d", resp.StatusCode)
	}

	var lr LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&lr); err != nil {
		return nil, fmt.Errorf("decode login response: %w", err)
	}
	if lr.AccessToken == "" && lr.Token != "" {
		lr.AccessToken = lr.Token
	}
	return &lr, nil
}

