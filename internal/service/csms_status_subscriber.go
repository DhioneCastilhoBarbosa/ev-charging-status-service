package service

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"ev-charging-status-service/internal/clients/csmsstomp"
	"ev-charging-status-service/internal/clients/intelbras"
	"ev-charging-status-service/internal/repository"
)

// CSMSStatusSubscriber inscreve no STOMP do CSMS por usuário (token + UUIDs das estações)
// e publica no WebSocket apenas quando status, errorCode ou errorInfo mudam.
type CSMSStatusSubscriber struct {
	credsRepo      *repository.CredentialsRepository
	stationService *StationService
	publisher      UserPublisher
	host           string
	useTLS         bool
	sockPrefix     string
	sessionRotate  time.Duration
	reconcileEvery time.Duration
}

// NewCSMSStatusSubscriber cria o assinante. sockPrefix vazio usa "/ws". sessionRotate renova inscrições STOMP periodicamente.
func NewCSMSStatusSubscriber(
	credsRepo *repository.CredentialsRepository,
	stationService *StationService,
	publisher UserPublisher,
	host string,
	useTLS bool,
	sockPrefix string,
	sessionRotate time.Duration,
) *CSMSStatusSubscriber {
	if sessionRotate <= 0 {
		sessionRotate = 10 * time.Minute
	}
	p := strings.TrimSpace(sockPrefix)
	if p == "" {
		p = "/ws"
	}
	return &CSMSStatusSubscriber{
		credsRepo:      credsRepo,
		stationService: stationService,
		publisher:      publisher,
		host:           host,
		useTLS:         useTLS,
		sockPrefix:     p,
		sessionRotate:  sessionRotate,
		reconcileEvery: 45 * time.Second,
	}
}

// Run reconcilia usuários com credenciais e mantém uma goroutine STOMP por usuário.
func (s *CSMSStatusSubscriber) Run(ctx context.Context) {
	runs := make(map[uuid.UUID]context.CancelFunc)
	var mu sync.Mutex
	ticker := time.NewTicker(s.reconcileEvery)
	defer ticker.Stop()

	reconcile := func() {
		ids, err := s.credsRepo.ListDistinctUserIDs(ctx)
		if err != nil {
			log.Printf("[status-subscriber] list users: %v", err)
			return
		}
		want := make(map[uuid.UUID]struct{}, len(ids))
		for _, id := range ids {
			want[id] = struct{}{}
		}

		mu.Lock()
		for uid, cancel := range runs {
			if _, ok := want[uid]; !ok {
				cancel()
				delete(runs, uid)
			}
		}
		for uid := range want {
			if _, exists := runs[uid]; exists {
				continue
			}
			c, cancel := context.WithCancel(ctx)
			runs[uid] = cancel
			uid := uid
			go s.runUserLoop(c, uid)
		}
		mu.Unlock()
	}

	reconcile()
	for {
		select {
		case <-ctx.Done():
			mu.Lock()
			for _, cancel := range runs {
				cancel()
			}
			mu.Unlock()
			return
		case <-ticker.C:
			reconcile()
		}
	}
}

func (s *CSMSStatusSubscriber) runUserLoop(ctx context.Context, userID uuid.UUID) {
	cache := newCSMSFingerprintCache()
	backoff := time.Second
	for ctx.Err() == nil {
		stations, err := s.stationService.GetStationsByUserID(ctx, userID)
		if err != nil {
			log.Printf("[status-subscriber] userId=%s get stations: %v", userID, err)
			sleepBackoff(ctx, backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second

		uuids := chargeBoxUUIDsFromStations(stations)
		if len(uuids) == 0 {
			sleepBackoff(ctx, 30*time.Second)
			continue
		}

		creds, err := s.credsRepo.GetByUserID(ctx, userID)
		if err != nil || creds.AccessToken == nil || strings.TrimSpace(*creds.AccessToken) == "" {
			log.Printf("[status-subscriber] userId=%s missing token after stations fetch", userID)
			sleepBackoff(ctx, backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
			continue
		}
		token := strings.TrimSpace(*creds.AccessToken)

		live := cloneFlattenedStations(stations)

		connCtx, cancelConn := context.WithTimeout(ctx, s.sessionRotate)
		err = csmsstomp.RunConsumer(connCtx, csmsstomp.DialConfig{
			Host:           s.host,
			SockJSPrefix:   s.sockPrefix,
			UseTLS:         s.useTLS,
			BearerToken:    token,
			ChargeBoxUUIDs: uuids,
		}, func(ev csmsstomp.StatusEvent) {
			u := strings.TrimSpace(ev.ChargeBoxUUID)
			if u == "" {
				return
			}
			if cache.isDuplicate(u, ev.ConnectorID, ev) {
				return
			}
			if !applyCSMSEventToFlattened(&live, ev) {
				log.Printf("[status-subscriber] STOMP sem match na lista: uuid=%s chargeBoxId=%q connectorId=%d",
					u, ev.ChargeBoxID, ev.ConnectorID)
				return
			}
			cache.remember(u, ev.ConnectorID, ev)
			payload := StationsPushPayload{
				UserID:    userID.String(),
				Timestamp: time.Now().UTC().Format(time.RFC3339),
				Stations:  flattenToStationsPushStations(live),
			}
			body, mErr := buildStationsPushPayloadJSON(payload)
			if mErr != nil {
				log.Printf("[status-subscriber] marshal payload userId=%s: %v", userID, mErr)
				return
			}
			log.Printf("[status-subscriber] CSMS → WebSocket (snapshot) userId=%s uuid=%s connectorId=%d status=%s",
				userID, u, ev.ConnectorID, ev.Status)
			s.publisher.PublishToUser(userID.String(), body)
		})
		cancelConn()

		if ctx.Err() != nil {
			return
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if err != nil {
			log.Printf("[status-subscriber] userId=%s stomp: %v", userID, err)
			sleepBackoff(ctx, backoff)
			if backoff < 30*time.Second {
				backoff *= 2
			}
		}
	}
}

func sleepBackoff(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}

func chargeBoxUUIDsFromStations(stations []intelbras.FlattenedChargePoint) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, cp := range stations {
		u := strings.TrimSpace(cp.UUID)
		if u == "" {
			continue
		}
		if _, ok := seen[u]; ok {
			continue
		}
		seen[u] = struct{}{}
		out = append(out, u)
	}
	sort.Strings(out)
	return out
}

type csmsFingerprintCache struct {
	mu sync.Mutex
	m  map[string]string
}

func newCSMSFingerprintCache() *csmsFingerprintCache {
	return &csmsFingerprintCache{m: make(map[string]string)}
}

func csmsFingerprint(ev csmsstomp.StatusEvent) string {
	ei := ""
	if ev.ErrorInfo != nil {
		ei = *ev.ErrorInfo
	}
	return strings.Join([]string{ev.Status, ev.ErrorCode, ei}, "\x00")
}

func (c *csmsFingerprintCache) isDuplicate(chargeBoxUUID string, connectorID int, ev csmsstomp.StatusEvent) bool {
	key := fmt.Sprintf("%s|%d", strings.ToLower(strings.TrimSpace(chargeBoxUUID)), connectorID)
	fp := csmsFingerprint(ev)
	c.mu.Lock()
	defer c.mu.Unlock()
	prev, ok := c.m[key]
	return ok && prev == fp
}

func (c *csmsFingerprintCache) remember(chargeBoxUUID string, connectorID int, ev csmsstomp.StatusEvent) {
	key := fmt.Sprintf("%s|%d", strings.ToLower(strings.TrimSpace(chargeBoxUUID)), connectorID)
	fp := csmsFingerprint(ev)
	c.mu.Lock()
	defer c.mu.Unlock()
	c.m[key] = fp
}

func cloneFlattenedStations(list []intelbras.FlattenedChargePoint) []intelbras.FlattenedChargePoint {
	out := make([]intelbras.FlattenedChargePoint, len(list))
	for i := range list {
		out[i] = list[i]
		out[i].Connectors = append([]intelbras.FlattenedConnector(nil), list[i].Connectors...)
	}
	return out
}

func applyCSMSEventToFlattened(list *[]intelbras.FlattenedChargePoint, ev csmsstomp.StatusEvent) bool {
	target := strings.TrimSpace(ev.ChargeBoxUUID)
	if target == "" {
		return false
	}
	ei := ""
	if ev.ErrorInfo != nil {
		ei = *ev.ErrorInfo
	}
	for i := range *list {
		cp := &(*list)[i]
		if !strings.EqualFold(strings.TrimSpace(cp.UUID), target) {
			continue
		}
		for j := range cp.Connectors {
			if cp.Connectors[j].ConnectorID != ev.ConnectorID {
				continue
			}
			cp.Connectors[j].Status = ev.Status
			cp.Connectors[j].ErrorCode = ev.ErrorCode
			cp.Connectors[j].ErroInfo = ei
			return true
		}
		return false
	}
	return false
}
