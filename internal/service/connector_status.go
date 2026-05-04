package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"

	"ev-charging-status-service/internal/clients/intelbras"
)

// ConnectorStatusStore guarda o último mapa status-por-conector por usuário (Fase 1: memória; futuro: Redis).
type ConnectorStatusStore interface {
	Get(ctx context.Context, userID uuid.UUID) (map[string]string, bool)
	Set(ctx context.Context, userID uuid.UUID, snapshot map[string]string) error
}

// InMemoryConnectorStatusStore implementa ConnectorStatusStore em processo.
type InMemoryConnectorStatusStore struct {
	mu     sync.RWMutex
	byUser map[uuid.UUID]map[string]string
}

func NewInMemoryConnectorStatusStore() *InMemoryConnectorStatusStore {
	return &InMemoryConnectorStatusStore{
		byUser: make(map[uuid.UUID]map[string]string),
	}
}

func (s *InMemoryConnectorStatusStore) Get(ctx context.Context, userID uuid.UUID) (map[string]string, bool) {
	_ = ctx
	s.mu.RLock()
	defer s.mu.RUnlock()
	m, ok := s.byUser[userID]
	if !ok {
		return nil, false
	}
	return cloneStringMap(m), true
}

func (s *InMemoryConnectorStatusStore) Set(ctx context.Context, userID uuid.UUID, snapshot map[string]string) error {
	_ = ctx
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.byUser == nil {
		s.byUser = make(map[uuid.UUID]map[string]string)
	}
	s.byUser[userID] = cloneStringMap(snapshot)
	return nil
}

func cloneStringMap(m map[string]string) map[string]string {
	if m == nil {
		return nil
	}
	out := make(map[string]string, len(m))
	for k, v := range m {
		out[k] = v
	}
	return out
}

// ConnectorStatusKey monta a chave estável chargeBoxId + connectorId.
func ConnectorStatusKey(chargeBoxID string, connectorID int) string {
	return fmt.Sprintf("%s#%d", chargeBoxID, connectorID)
}

// ConnectorStatusMapFromFlattened extrai apenas connectors[].status por estação.
func ConnectorStatusMapFromFlattened(list []intelbras.FlattenedChargePoint) map[string]string {
	out := make(map[string]string)
	for _, cp := range list {
		for _, conn := range cp.Connectors {
			out[ConnectorStatusKey(cp.ChargeBoxID, conn.ConnectorID)] = conn.Status
		}
	}
	return out
}

// ConnectorStatusMapsEqual compara dois snapshots de status (inclui chaves novas/removidas).
func ConnectorStatusMapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}
