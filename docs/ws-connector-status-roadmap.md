# Roadmap: WebSocket só em mudança de status (conectores)

Plano para publicar no WebSocket apenas quando `connectors[].status` mudar em relação ao poll anterior da API externa (`GET …/chargepoints`). **Uma única instância da API** (sem réplicas); **Redis fica para uma fase futura**, com abstração já preparada.

**Fase 1:** implementada (`internal/service/connector_status.go`, `ws_station_publisher.go`; callback em `internal/api/ws_handler.go` + `routes.go`).

## Contexto

- O serviço **não** faz mais poll periódico a `GET …/chargepoints` para o WebSocket (rate limit); atualizações incrementais vêm do **STOMP** do CSMS.
- JSON da API usa `lastStatus.status` por conector; o modelo em `internal/clients/intelbras/stations_client.go` já cobre isso após o flatten.
- Estado “último snapshot de status por conector” **não** precisa ser compartilhado entre processos enquanto existir só um pod.

## Fase 1 — Agora (memória, uma instância)

1. **Interface** (ex.: `ConnectorStatusStore`):
   - `Get(ctx, userID) (map[string]string, bool)` — último `status` por chave de conector.
   - `Set(ctx, userID, map[string]string) error` — persistir após publicar com sucesso.
   - Opcional: `Delete(ctx, userID)` para invalidação futura.

2. **Implementação `InMemoryConnectorStatusStore`**
   - `sync.RWMutex` + `map[uuid.UUID]map[string]string` (ou mapa serializado).
   - `WSStationPublisher` depende **apenas** da interface.

3. **Diff**
   - De `[]FlattenedChargePoint` montar `map[chave]status` (ex.: `chargeBoxId + "#" + connectorId`).
   - Comparar com `Get`; se não houver snapshot **ou** os mapas forem diferentes → montar `WebhookPayload`, `PublishToUser`, depois `Set`.

4. **Primeiro ciclo e novo cliente WebSocket**
   - Primeiro poll com sucesso: sem snapshot → publicar.
   - Ao conectar no WS: disparar publicação de snapshot (fetch + envio sempre, ou reutilizar último payload se existir cache).

5. **Testes**
   - Mock da store e do `UserPublisher`: mesmo status → zero publishes; mudou um status → um publish.

6. **Documentação operacional** (quando fizer sentido): comportamento “só em mudança” + premissa de instância única.

## Fase 2 — Futuro (Redis)

1. **`RedisConnectorStatusStore`** com a **mesma interface**.
   - Chave exemplo: `ws:connector-status:{userID}`.
   - Valor: JSON do mapa ou hash Redis por campo de conector.
   - TTL opcional para tenants inativos.

2. **Composição** (`cmd/api/main.go`)
   - Ex.: se `REDIS_ADDR` (ou variável dedicada) estiver definida → Redis; senão → memória.
   - Mesmo `WSStationPublisher`.

3. **Resiliência**
   - Definir política se Redis cair: fallback “sem estado” (possível publish extra) vs. não publicar até voltar.

4. **Réplicas (se um dia existirem)**
   - Redis alinha o último estado entre pods; pode ser necessário **poller único** ou **lock distribuído** para não multiplicar chamadas à API externa — evolução após a store Redis.

## Resumo

| Agora | Depois (Redis) |
|--------|----------------|
| Interface + implementação em memória | Nova implementação Redis |
| Publisher só conhece a interface | Troca na injeção / env |
| Uma instância | Estado compartilhado entre pods (quando houver) |

## Referências no código

- Publisher atual: `internal/service/ws_station_publisher.go`
- Hub WS: `internal/api/ws_hub.go`, handler: `internal/api/ws_handler.go`
- Lista de estações / flatten: `internal/clients/intelbras/stations_client.go`, `internal/service/station_service.go`
- Snapshot ao conectar WebSocket: um `GET …/chargepoints` em `OnWebSocketConnected`.
