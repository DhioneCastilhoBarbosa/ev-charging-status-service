package service

import (
	"context"
	"sync"
	"testing"

	"github.com/google/uuid"

	"ev-charging-status-service/internal/clients/intelbras"
)

type fakeStationFetch struct {
	mu   sync.Mutex
	list []intelbras.FlattenedChargePoint
}

func (f *fakeStationFetch) GetStationsByUserID(ctx context.Context, userID uuid.UUID) ([]intelbras.FlattenedChargePoint, error) {
	_ = ctx
	_ = userID
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.list, nil
}

func (f *fakeStationFetch) setList(list []intelbras.FlattenedChargePoint) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.list = list
}

type recordingPublisher struct {
	mu    sync.Mutex
	count int
}

func (r *recordingPublisher) PublishToUser(userID string, payload []byte) {
	_ = userID
	_ = payload
	r.mu.Lock()
	defer r.mu.Unlock()
	r.count++
}

func (r *recordingPublisher) publishes() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.count
}

func TestWSStationPublisher_publishForUserIfStatusChanged_skipsWhenEqual(t *testing.T) {
	ctx := context.Background()
	uid := uuid.MustParse("22222222-2222-2222-2222-222222222222")
	fetch := &fakeStationFetch{}
	pub := &recordingPublisher{}
	store := NewInMemoryConnectorStatusStore()
	p := &WSStationPublisher{
		stationService: fetch,
		publisher:      pub,
		statusStore:    store,
	}

	cp := []intelbras.FlattenedChargePoint{{
		ChargeBoxID: "X",
		Connectors:  []intelbras.FlattenedConnector{{ConnectorID: 1, Status: "Available"}},
	}}
	fetch.setList(cp)

	p.publishForUserIfStatusChanged(ctx, uid)
	if pub.publishes() != 1 {
		t.Fatalf("first fetch should publish, got %d", pub.publishes())
	}

	p.publishForUserIfStatusChanged(ctx, uid)
	if pub.publishes() != 1 {
		t.Fatalf("same status should not publish again, got %d", pub.publishes())
	}

	cp2 := []intelbras.FlattenedChargePoint{{
		ChargeBoxID: "X",
		Connectors:  []intelbras.FlattenedConnector{{ConnectorID: 1, Status: "Occupied"}},
	}}
	fetch.setList(cp2)
	p.publishForUserIfStatusChanged(ctx, uid)
	if pub.publishes() != 2 {
		t.Fatalf("status change should publish, got %d", pub.publishes())
	}
}

func TestWSStationPublisher_OnWebSocketConnected_alwaysPublishes(t *testing.T) {
	ctx := context.Background()
	fetch := &fakeStationFetch{}
	fetch.setList([]intelbras.FlattenedChargePoint{{
		ChargeBoxID: "Y",
		Connectors:  []intelbras.FlattenedConnector{{ConnectorID: 1, Status: "Available"}},
	}})
	pub := &recordingPublisher{}
	store := NewInMemoryConnectorStatusStore()
	p := &WSStationPublisher{
		stationService: fetch,
		publisher:      pub,
		statusStore:    store,
	}
	uidStr := "33333333-3333-3333-3333-333333333333"

	p.OnWebSocketConnected(ctx, uidStr)
	if pub.publishes() != 1 {
		t.Fatalf("connect snapshot expected 1 publish, got %d", pub.publishes())
	}
	p.OnWebSocketConnected(ctx, uidStr)
	if pub.publishes() != 2 {
		t.Fatalf("second connect snapshot should publish again, got %d", pub.publishes())
	}
}
