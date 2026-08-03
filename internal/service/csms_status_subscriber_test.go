package service

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"ev-charging-status-service/internal/clients/csmsstomp"
	"ev-charging-status-service/internal/clients/intelbras"
)

func TestCSMSFingerprintCache(t *testing.T) {
	c := newCSMSFingerprintCache()
	u := "dd5db20e-b296-4c43-9270-6aec8d931ea2"
	ev1 := csmsstomp.StatusEvent{Status: "Charging", ErrorCode: "NO_ERROR"}
	if c.isDuplicate(u, 1, ev1) {
		t.Fatal("first event should not be duplicate")
	}
	c.remember(u, 1, ev1)
	if !c.isDuplicate(u, 1, ev1) {
		t.Fatal("same fingerprint should be duplicate")
	}
	info := "overheat"
	ev2 := csmsstomp.StatusEvent{Status: "Charging", ErrorCode: "NO_ERROR", ErrorInfo: &info}
	if c.isDuplicate(u, 1, ev2) {
		t.Fatal("errorInfo change should not be duplicate")
	}
	c.remember(u, 1, ev2)
	ev3 := csmsstomp.StatusEvent{Status: "Available", ErrorCode: "NO_ERROR", ErrorInfo: &info}
	if c.isDuplicate(u, 1, ev3) {
		t.Fatal("status change should not be duplicate")
	}
}

type stubActiveWS struct {
	n int
}

func (s *stubActiveWS) ActiveConnections(userID string) int {
	_ = userID
	return s.n
}

func (s *stubActiveWS) PublishToUser(userID string, payload []byte) {
	_ = userID
	_ = payload
}

func TestHasActiveWS(t *testing.T) {
	sub := NewCSMSStatusSubscriber(nil, nil, &stubActiveWS{n: 0}, "", false, "", time.Minute, 15*time.Second)
	uid := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	if sub.hasActiveWS(uid) {
		t.Fatal("expected no active ws")
	}
	sub.wsActive = &stubActiveWS{n: 2}
	if !sub.hasActiveWS(uid) {
		t.Fatal("expected active ws")
	}
	sub.wsActive = nil
	if sub.hasActiveWS(uid) {
		t.Fatal("nil checker should be inactive")
	}
}

func TestInventoryChanged(t *testing.T) {
	if inventoryChanged(nil, []string{"a"}) {
		t.Fatal("nil prev is baseline — should not report change")
	}
	if inventoryChanged([]string{"a"}, []string{"a"}) {
		t.Fatal("same inventory should not change")
	}
	if !inventoryChanged([]string{"a"}, []string{"a", "b"}) {
		t.Fatal("added station should change")
	}
	if !inventoryChanged([]string{"a", "b"}, []string{"a"}) {
		t.Fatal("removed station should change")
	}
	if !inventoryChanged([]string{"a"}, []string{}) {
		t.Fatal("cleared inventory should change")
	}
	// chargeBoxUUIDsFromStations ordena — comparar listas já ordenadas
	a := chargeBoxUUIDsFromStations([]intelbras.FlattenedChargePoint{
		{UUID: "bbbb"},
		{UUID: "aaaa"},
	})
	b := chargeBoxUUIDsFromStations([]intelbras.FlattenedChargePoint{
		{UUID: "aaaa"},
		{UUID: "bbbb"},
	})
	if inventoryChanged(a, b) {
		t.Fatal("same UUIDs different order should be equal after sort")
	}
}

func TestApplyCSMSEventToFlattened(t *testing.T) {
	live := []intelbras.FlattenedChargePoint{{
		UUID:        "dd5db20e-b296-4c43-9270-6aec8d931ea2",
		ChargeBoxID: "MOVE_LAB_INTELBRAS01",
		Connectors: []intelbras.FlattenedConnector{{
			ConnectorID: 1,
			Status:      "Available",
			ErrorCode:   "NoError",
			ErroInfo:    "NoError",
		}},
	}}
	ev := csmsstomp.StatusEvent{
		ChargeBoxUUID: "dd5db20e-b296-4c43-9270-6aec8d931ea2",
		ConnectorID:   1,
		Status:        "Preparing",
		ErrorCode:     "NoError",
	}
	if !applyCSMSEventToFlattened(&live, ev) {
		t.Fatal("expected apply ok")
	}
	if live[0].Connectors[0].Status != "Preparing" {
		t.Fatalf("status got %q", live[0].Connectors[0].Status)
	}
}
