package service

import (
	"testing"

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
