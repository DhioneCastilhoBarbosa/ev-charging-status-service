package service

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"ev-charging-status-service/internal/clients/intelbras"
)

func TestConnectorStatusMapFromFlattened(t *testing.T) {
	list := []intelbras.FlattenedChargePoint{
		{
			ChargeBoxID: "A",
			Connectors: []intelbras.FlattenedConnector{
				{ConnectorID: 1, Status: "Available"},
				{ConnectorID: 2, Status: "Occupied"},
			},
		},
		{ChargeBoxID: "B", Connectors: []intelbras.FlattenedConnector{{ConnectorID: 1, Status: "Faulted"}}},
	}
	m := ConnectorStatusMapFromFlattened(list)
	if got := len(m); got != 3 {
		t.Fatalf("len=%d want 3", got)
	}
	if m["A#1"] != "Available" || m["A#2"] != "Occupied" || m["B#1"] != "Faulted" {
		t.Fatalf("unexpected map: %#v", m)
	}
}

func TestConnectorStatusMapsEqual(t *testing.T) {
	a := map[string]string{"x#1": "Available", "y#1": "Occupied"}
	b := map[string]string{"y#1": "Occupied", "x#1": "Available"}
	if !ConnectorStatusMapsEqual(a, b) {
		t.Fatal("same content should be equal")
	}
	if ConnectorStatusMapsEqual(a, map[string]string{"x#1": "Available"}) {
		t.Fatal("different len should not be equal")
	}
	if ConnectorStatusMapsEqual(a, map[string]string{"x#1": "Faulted", "y#1": "Occupied"}) {
		t.Fatal("different status should not be equal")
	}
}

func TestInMemoryConnectorStatusStore(t *testing.T) {
	ctx := context.Background()
	uid := uuid.MustParse("11111111-1111-1111-1111-111111111111")
	s := NewInMemoryConnectorStatusStore()
	if _, ok := s.Get(ctx, uid); ok {
		t.Fatal("expected miss")
	}
	snap := map[string]string{"cb#1": "Available"}
	if err := s.Set(ctx, uid, snap); err != nil {
		t.Fatal(err)
	}
	got, ok := s.Get(ctx, uid)
	if !ok || got["cb#1"] != "Available" {
		t.Fatalf("got %#v ok=%v", got, ok)
	}
	snap["cb#1"] = "Mutated"
	got2, _ := s.Get(ctx, uid)
	if got2["cb#1"] != "Available" {
		t.Fatal("store should keep copy, not alias")
	}
}
