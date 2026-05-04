# EV Charging Status Service

Microserviço em Go para monitoramento de estações de recarga (EV). Autentica na API Move/Intelbras, consulta estações e conectores, pode enviar os dados por **webhook** (opcional) e faz **push periódico por WebSocket** por usuário (JWT no handshake).

---

## Índice

- [Stack](#stack)
- [Arquitetura](#arquitetura)
- [Fluxo de dados](#fluxo-de-dados)
- [Variáveis de ambiente](#variáveis-de-ambiente)
- [Como rodar](#como-rodar)
- [API](#api)
- [Documentação Swagger](#documentação-swagger)
- [Migrations](#migrations)
- [Segurança](#segurança)
- [Estrutura do projeto](#estrutura-do-projeto)

---

## Stack

| Tecnologia | Uso |
|------------|-----|
| **Go 1.25** | Linguagem |
| **Gin** | API HTTP |
| **Gorilla WebSocket** | Conexão WS + JWT curto (`WS_JWT_SECRET` / fallback `API_KEY`) |
| **PostgreSQL** | Persistência (usuários, credenciais, webhooks, eventos) |
| **Docker / Docker Compose** | API, Worker e banco em containers |

Redis e Kafka estão no `go.mod` para uso futuro; na versão atual não são utilizados.

---

## Arquitetura

```mermaid
flowchart TB
    %% Clientes da API
    subgraph Client["Cliente - sistema integrador"]
        C[HTTP client backend]
    end

    %% Serviço principal
    subgraph Service["ev-charging-status-service"]
        subgraph API["API HTTP 8085"]
            R[Router Gin]
            R --> Health["GET /health"]
            R --> V1["/v1 (rate limit,\nX-API-Key)"]
            V1 --> Config["ConfigHandler\nPOST /config (+token WS)\nGET /config/status\nDELETE /config (Bearer)"]
            V1 --> Stations["StationsHandler\nPOST /stations"]
            V1 --> WS["WebSocket\nGET /ws/token\nGET /ws"]
            WSPub["WSStationPublisher\nintervalo env"]
            WSPub --> WS
        end

        subgraph Worker["Worker - background"]
            Scheduler["SchedulerService 3 min"]
            Job["StationWebhookJob\nmonta payload"]
            Sender["WebhookService\nsender 30s + retry"]
            Scheduler --> Job
            Job --> Queue["Tabela webhook_events\n(status=PENDING)"]
            Sender --> Queue
            Sender --> POST["POST webhook"]
        end
    end

    %% Persistência
    subgraph DB["PostgreSQL - persistência"]
        Users[(users)]
        Creds[(third_party_credentials)]
        Webhooks[(webhooks)]
        Events[(webhook_events)]
    end

    %% Sistemas externos
    subgraph External["Sistemas externos"]
        Intelbras["API Move Intelbras"]
        WebhookURL["URL do Webhook (n8n, Pipedream, etc.)"]
    end

    %% Relações
    C -->|X-API-Key + JSON / WS JWT| API
    API --> Users
    API --> Creds
    API --> Webhooks
    API -->|Login / ChargePoints| Intelbras

    Worker --> Creds
    Worker --> Events
    Worker -->|Login / ChargePoints| Intelbras
    Worker -->|POST JSON\npayload stations| WebhookURL
```

### Componentes

| Componente | Descrição |
|------------|-----------|
| **API** | Servidor HTTP (porta **8085**). Expõe `/health`, `POST /v1/config`, `GET /v1/config/status`, `DELETE /v1/config` (Bearer JWT), `POST /v1/stations` (Bearer JWT + `apiKey` no corpo), `GET /v1/ws/token`, `GET /v1/ws` (upgrade WebSocket, **só JWT**, sem `X-API-Key`), `GET /v1/ws/stats`. Rotas HTTP `/v1/*` (exceto handshake `/v1/ws`) usam `X-API-Key` se `API_KEY` definida; rate limit 15 req/min por IP no grupo `/v1`. |
| **Worker** | Processo em background. A cada **3 minutos** executa o job: busca estações na Intelbras **por usuário** e enfileira webhook só para quem tem URL ativa. A cada **30 segundos** o sender processa `webhook_events` e faz POST com retentativas. |
| **Push WS (API)** | No mesmo processo da API, publicador envia JSON de estações **por `user_id`** no intervalo `WS_PUBLISH_INTERVAL_SECONDS` (default 180 s). Uma credencial por usuário no banco (migration `003`). |
| **PostgreSQL** | Armazena usuários, credenciais (senha e API key criptografadas com `ENCRYPTION_KEY`), webhooks e fila de eventos (`webhook_events`). |

---

## Fluxo de dados

1. **Configuração**  
   `POST /v1/config` com email, senha e, se quiser, `apiKey` (Intelbras) e `webhookUrl`. A API faz login na Move/Intelbras e persiste credenciais (criptografadas se `ENCRYPTION_KEY` estiver definida). A resposta inclui **`token` e `expiresIn`** para abrir o WebSocket sem chamar `/v1/ws/token`.

2. **Consulta de estações (HTTP)**  
   `POST /v1/stations` com header `Authorization: Bearer <JWT>` (o mesmo do WebSocket) e corpo JSON `{"apiKey":"..."}` igual ao configurado em `/v1/config` (string vazia se não houver). Resposta: mesmo JSON do push WS (`userId`, `timestamp`, `stations`). Exige `X-API-Key` da API quando configurado.

3. **Webhook periódico (opcional)**  
   Se existir webhook ativo para o usuário, o worker a cada **3 min** enfileira um evento; o sender envia POST com retentativas e backoff (429/503 e `Retry-After`).

4. **WebSocket**  
   - **Handshake:** `GET /v1/ws` com `?token=<JWT>` **ou** header `Authorization: Bearer <JWT>`. **Não** enviar `X-API-Key` nesta URL — apenas o JWT emitido por `POST /v1/config` ou `GET /v1/ws/token?username=`. Token inválido ou expirado (`WS_TOKEN_TTL_SECONDS`) → **401** JSON, sem upgrade.  
   - **Dados:** após o upgrade, o servidor envia **frames de texto** JSON no intervalo `WS_PUBLISH_INTERVAL_SECONDS` (padrão **180 s**), com um ciclo **logo na subida** da API. O payload é o mesmo do webhook/`POST /v1/stations`: `userId`, `stations`, `timestamp` (RFC3339). Só entram no ciclo usuários com credenciais; cada conexão recebe apenas o lote do `userId` do seu JWT. Se a Intelbras falhar naquele ciclo para aquele usuário, nada é enviado naquele ciclo.  
   - **Transporte:** Ping do servidor a cada **25 s** (responder com Pong); fila de **32** mensagens por conexão — cliente lento pode ser desconectado (backpressure). O servidor não revalida o JWT depois do handshake até o cliente reconectar.

---

## Variáveis de ambiente

Copie `.env.example` para `.env` e ajuste. No Docker Compose, as variáveis do `.env` são usadas nos serviços `api` e `worker`.

| Variável | Obrigatória | Descrição |
|----------|-------------|-----------|
| `POSTGRES_URL` | Sim | URL de conexão com o PostgreSQL (ex.: `postgres://user:pass@db:5432/charging?sslmode=disable`). |
| `INTELBRAS_BASE_URL` | Sim | Base da API Move/Intelbras (ex.: `https://cs-test.use-move.com/api/v1`). |
| `API_KEY` | Sim* | Chave para autorizar chamadas à API (`X-API-Key`). \* Use `ALLOW_EMPTY_API_KEY=true` só em dev. |
| `ENCRYPTION_KEY` | Não | Chave para criptografar senha e API key no banco. Pode ser UUID, senha ou base64 (32 bytes). Se vazia, dados ficam em texto. |
| `WS_JWT_SECRET` | Não* | Segredo para assinar o token curto do WebSocket. \* Se vazio, usa `API_KEY` como fallback. |
| `WS_TOKEN_TTL_SECONDS` | Não | TTL do token de conexão WS em segundos (default: `300`). |
| `WS_PUBLISH_INTERVAL_SECONDS` | Não | Intervalo de envio do WebSocket em segundos (default: `180`, 3 min, alinhado ao worker). |

---

## Como rodar

### Com Docker Compose (recomendado)

```bash
# Subir API, Worker e PostgreSQL
docker compose up -d --build

# Ver logs
docker compose logs -f
```

- **API**: http://localhost:8085  
- **Health**: http://localhost:8085/health  
- **Swagger**: http://localhost:8085/swagger/index.html  

### Aplicar migrations

Após o primeiro `up`, crie as tabelas (se ainda não existirem):

```bash
# Linux/macOS
cat migrations/001_init.sql migrations/002_webhook_events_payload_text.sql migrations/003_third_party_credentials_unique_user.sql | docker exec -i ev-charging-db psql -U postgres -d charging

# Windows (PowerShell)
Get-Content migrations/001_init.sql | docker exec -i ev-charging-db psql -U postgres -d charging
Get-Content migrations/002_webhook_events_payload_text.sql | docker exec -i ev-charging-db psql -U postgres -d charging
Get-Content migrations/003_third_party_credentials_unique_user.sql | docker exec -i ev-charging-db psql -U postgres -d charging
```

**Banco externo ou Beekeeper:** abra o SQL da pasta `migrations/` (incluindo `003_...`) e execute na ordem no seu Postgres; o `003` remove credenciais duplicadas por `user_id` e cria índice único (necessário para o upsert e para evitar rajadas no WebSocket).

### Local (sem Docker)

1. PostgreSQL rodando e banco `charging` criado.  
2. Defina as variáveis de ambiente. O `go run` **não carrega `.env` sozinho**; no PowerShell você pode injetar o arquivo antes de rodar ou exportar `POSTGRES_URL`, `API_KEY`, etc. manualmente.  
3. Rode a API e o worker em terminais separados:

```bash
go run cmd/api/main.go
go run cmd/worker/main.go
```

---

## API

| Método | Rota | Descrição |
|--------|------|-----------|
| GET | `/health` | Health check (sem autenticação). |
| POST | `/v1/config` | Configura credenciais (email e password obrigatórios; `webhookUrl` e `apiKey` opcionais). Resposta: `token` (JWT WS) e `expiresIn`. Requer `X-API-Key` se `API_KEY` estiver definida. |
| DELETE | `/v1/config` | Remove o usuário identificado pelo JWT: header `Authorization: Bearer <token>`. Requer `X-API-Key` se `API_KEY` estiver definida. Resposta 204. |
| GET | `/v1/config/status` | Retorna se há configuração e se o token Intelbras está presente (sem expor tokens). Requer `X-API-Key` quando configurado. |
| POST | `/v1/stations` | Corpo `{"apiKey":"..."}`; header `Authorization: Bearer <JWT WS>`. Retorna `userId`, `timestamp`, `stations` (igual ao push WebSocket). Requer `X-API-Key` quando configurado. |
| GET | `/v1/ws/token?username={email}` | Emite JWT para handshake WS (`token`, `expiresIn`). O `username` é o e-mail já configurado. Requer `X-API-Key` quando configurado. |
| GET | `/v1/ws` | Upgrade para WebSocket. Autenticação **somente** com `?token=` ou `Authorization: Bearer` (sem `X-API-Key`). Ver seção *Fluxo de dados* para payload e intervalo. |
| GET | `/v1/ws/stats` | Métricas do hub WS (conexões, mensagens, drops por backpressure). Requer `X-API-Key` quando configurado. |

Respostas de erro usam mensagens genéricas (`invalid request`, `configuration failed`, `stations unavailable`, etc.); o detalhe é logado no servidor.

---

## Documentação Swagger

A documentação OpenAPI (Swagger) está disponível com a API rodando:

- **UI**: [http://localhost:8085/swagger/index.html](http://localhost:8085/swagger/index.html)
- **JSON**: [http://localhost:8085/swagger/doc.json](http://localhost:8085/swagger/doc.json)

Para regenerar a spec a partir dos comentários no código (requer [swag](https://github.com/swaggo/swag)):

```bash
go install github.com/swaggo/swag/cmd/swag@latest
swag init -g cmd/api/main.go -o docs
```

---

## Migrations

| Arquivo | Descrição |
|---------|-----------|
| `001_init.sql` | Cria tabelas: users, third_party_credentials, webhooks, stations, connector_status, webhook_events. |
| `002_webhook_events_payload_text.sql` | Altera `webhook_events.payload` de JSONB para TEXT (preserva ordem das chaves do JSON). |
| `003_third_party_credentials_unique_user.sql` | Remove credenciais duplicadas por `user_id`, cria índice único e permite `UPSERT` correto por usuário (evita rajadas no WebSocket/job). |
| `clear_data.sql` | Limpa todos os dados (TRUNCATE), mantendo a estrutura. |

---

## Segurança

- **API_KEY**: Exigida no startup (exceto com `ALLOW_EMPTY_API_KEY=true`). Usada no header `X-API-Key` nas rotas `/v1/*`.  
- **WS_JWT_SECRET**: Assinatura do JWT do WebSocket; se vazio, usa `API_KEY` como fallback (recomenda-se segredo dedicado em produção).  
- **ENCRYPTION_KEY**: Criptografia AES-256-GCM para senha e API key da Intelbras no banco.  
- **Rate limit**: 15 requisições/minuto por IP no grupo `/v1`.  
- **Isolamento WS**: O hub só encaminha para conexões cujo JWT contém o mesmo `userId`; o payload é montado com `GetStationsByUserID` (credencial daquele usuário).  
- **Handshake WS**: `/v1/ws` não valida `X-API-Key`; quem protege a sessão é o JWT (curta duração).  
- **Erros**: Respostas 4xx/5xx com mensagens genéricas; corpo de erro do login de terceiros não é exposto ao cliente.  
- **Logs**: URL completa do webhook e dados sensíveis não são logados.

---

## Estrutura do projeto

```
ev-charging-status-service/
├── cmd/
│   ├── api/           # Entrypoint da API
│   └── worker/        # Entrypoint do worker
├── docs/              # Swagger (gerado por swag)
├── internal/
│   ├── api/           # Handlers, rotas, rate limit, WebSocket (hub + handler)
│   ├── clients/
│   │   └── intelbras/ # Cliente HTTP (login + charge-points)
│   ├── config/        # Carregamento de env
│   ├── crypto/        # Criptografia (ENCRYPTION_KEY)
│   ├── database/      # Conexão PostgreSQL
│   ├── repository/    # Acesso a dados
│   └── service/       # Regras de negócio (config, stations, webhook, job, scheduler, auth WS, publisher WS)
├── migrations/        # SQL de schema e utilitários
├── docker-compose.yml
├── Dockerfile         # Multi-stage: API + Worker
├── go.mod
├── .env.example
└── README.md
```
