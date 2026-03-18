package service

import (
	"bytes"
	"context"
	"io"
	"log"
	"net/http"
	"strconv"
	"time"

	"ev-charging-status-service/internal/repository"
)

type WebhookService struct {
	webhookRepo      *repository.WebhookRepository
	webhookEventRepo *repository.WebhookEventRepository
	httpClient       *http.Client
}

func NewWebhookService(
	webhookRepo *repository.WebhookRepository,
	webhookEventRepo *repository.WebhookEventRepository,
) *WebhookService {
	return &WebhookService{
		webhookRepo:      webhookRepo,
		webhookEventRepo: webhookEventRepo,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

// computeRetryDelay calcula o atraso até a próxima tentativa. Respeita Retry-After em 429/503;
// para 429 usa pelo menos 5 min; caso contrário backoff exponencial (1, 2, 4... minutos).
func computeRetryDelay(resp *http.Response, attemptCount int) time.Duration {
	const min429Delay = 5 * time.Minute
	const maxDelay = 30 * time.Minute

	if resp != nil && (resp.StatusCode == 429 || resp.StatusCode == 503) {
		if s := resp.Header.Get("Retry-After"); s != "" {
			if sec, err := strconv.Atoi(s); err == nil && sec > 0 {
				d := time.Duration(sec) * time.Second
				if d > maxDelay {
					d = maxDelay
				}
				if d < time.Minute {
					d = time.Minute
				}
				return d
			}
			if t, err := time.Parse(time.RFC1123, s); err == nil {
				if d := time.Until(t); d > 0 {
					if d > maxDelay {
						return maxDelay
					}
					return d
				}
			}
		}
		if resp.StatusCode == 429 {
			backoff := time.Duration(1<<attemptCount) * time.Minute
			if backoff < min429Delay {
				backoff = min429Delay
			}
			if backoff > maxDelay {
				backoff = maxDelay
			}
			return backoff
		}
	}
	// Backoff exponencial: 1, 2, 4, 8, 16 min (cap em maxDelay)
	delay := time.Duration(1<<attemptCount) * time.Minute
	if delay > maxDelay {
		delay = maxDelay
	}
	return delay
}

func (s *WebhookService) EnqueueEvent(ctx context.Context, webhookID repository.Webhook, payload []byte) error {
	event := &repository.WebhookEvent{
		WebhookID:    webhookID.ID,
		Payload:      payload,
		Status:       repository.WebhookEventStatusPending,
		AttemptCount: 0,
	}
	return s.webhookEventRepo.Create(ctx, event)
}

func (s *WebhookService) RunSender(ctx context.Context, interval time.Duration, maxAttempts int) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processDueEvents(ctx, maxAttempts)
		}
	}
}

func (s *WebhookService) processDueEvents(ctx context.Context, maxAttempts int) {
	events, err := s.webhookEventRepo.FindDue(ctx, 100)
	if err != nil {
		return
	}

	for _, e := range events {
		w, err := s.webhookRepo.GetByID(ctx, e.WebhookID)
		if err != nil {
			continue
		}
		if !w.Active {
			continue
		}

		status := repository.WebhookEventStatusSent
		var lastErr *string
		nextAttempt := (*time.Time)(nil)

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, w.TargetURL, bytes.NewReader(e.Payload))
		if err != nil {
			errStr := err.Error()
			lastErr = &errStr
			status = repository.WebhookEventStatusFailed
			log.Printf("[webhook-sender] webhook id %s build request failed", w.ID)
		} else {
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("User-Agent", "ev-charging-status-service/1.0")
			req.ContentLength = int64(len(e.Payload))
			resp, err := s.httpClient.Do(req)
			if err != nil || resp.StatusCode < 200 || resp.StatusCode >= 300 {
				if err != nil {
					errStr := err.Error()
					lastErr = &errStr
					log.Printf("[webhook-sender] webhook id %s delivery failed", w.ID)
				}
				if resp != nil && (resp.StatusCode < 200 || resp.StatusCode >= 300) {
					errStr := resp.Status
					lastErr = &errStr
					log.Printf("[webhook-sender] webhook id %s returned %s", w.ID, resp.Status)
				}
				if e.AttemptCount+1 >= maxAttempts {
					status = repository.WebhookEventStatusFailed
				} else {
					status = repository.WebhookEventStatusRetrying
					delay := computeRetryDelay(resp, e.AttemptCount)
					t := time.Now().Add(delay)
					nextAttempt = &t
				}
			} else {
				log.Printf("[webhook-sender] webhook id %s ok (%d)", w.ID, resp.StatusCode)
			}
			if resp != nil {
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}

		e.Status = status
		e.LastError = lastErr
		e.AttemptCount++
		e.NextAttempt = nextAttempt
		_ = s.webhookEventRepo.Update(ctx, &e)
	}
}

