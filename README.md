# EV Charging Status Service

Microserviço em Go para monitoramento de estações de recarga (EV). Autentica na API Move/Intelbras, consulta estações e conectores e envia os dados periodicamente para uma URL de webhook configurada pelo usuário.

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
| **PostgreSQL** | Persistência (usuários, credenciais, webhooks, eventos) |
| **Docker / Docker Compose** | API, Worker e banco em containers |

Redis e Kafka estão no `go.mod` para uso futuro; na versão atual não são utilizados.

---

## Arquitetura

```mermaid
flowchart TB
    subgraph Client["Cliente"]
        A[Cliente HTTP]
    end

    subgraph Service["ev-charging-status-service"]
        subgraph API["API :8080"]
            R[Router Gin]
            R --> Health["GET /health"]
            R --> V1["/v1"]
            V1 --> Config["POST /config\nGET /config/status"]
            V1 --> Stations["GET /stations"]
        end

        subgraph Worker["Worker"]
            Scheduler[Scheduler 3 min]
            Job[StationWebhookJob]
            Sender[Webhook Sender 30s]
            Scheduler --> Job
            Job --> Enqueue[Enfileirar evento]
            Sender --> POST[POST webhook]
        end
    end

    subgraph DB["PostgreSQL"]
        T[(Tabelas:\nusers, credentials,\nwebhooks, webhook_events)]
    end

    subgraph External["Externos"]
        Intelbras[API Move/Intelbras\nlogin + charge-points]
        WebhookURL[URL do Webhook\n(n8n, Pipedream, etc.)]
    end

    A -->|X-API-Key| API
    API --> DB
    API -->|Login / ChargePoints| Intelbras
    Worker --> DB
    Worker -->|Login / ChargePoints| Intelbras
    Worker -->|POST JSON| WebhookURL
```

### Componentes

| Componente | Descrição |
|------------|-----------|
| **API** | Servidor HTTP (porta 8080). Expõe `/health`, `POST /v1/config`, `GET /v1/config/status`, `GET /v1/stations`. Protegido por `X-API-Key` e rate limit (15 req/min por IP). |
| **Worker** | Processo em background. A cada **3 minutos** executa o job: busca estações na Intelbras e enfileira um evento de webhook por usuário. A cada **30 segundos** o sender processa eventos pendentes e envia POST para a URL cadastrada. |
| **PostgreSQL** | Armazena usuários, credenciais (senha e API key criptografadas com `ENCRYPTION_KEY`), webhooks e fila de eventos (`webhook_events`). |

---

## Fluxo de dados

1. **Configuração**  
   O cliente envia `POST /v1/config` com email, senha, (opcional) API key da Intelbras e URL do webhook. A API faz login na Move/Intelbras, persiste credenciais (criptografadas se `ENCRYPTION_KEY` estiver definida) e salva a URL do webhook.

2. **Consulta de estações**  
   `GET /v1/stations` usa o token salvo (ou renova com login) e retorna a lista de estações/conectores da API de terceiros.

3. **Webhook periódico**  
   O worker, a cada 3 min, obtém estações, monta o JSON (`userId`, `stations`, `timestamp`) e cria um registro em `webhook_events`. O sender envia POST para a URL do webhook com retentativas e backoff (incluindo respeito a 429/503 e `Retry-After`).

---

## Variáveis de ambiente

Copie `.env.example` para `.env` e ajuste. No Docker Compose, as variáveis do `.env` são usadas nos serviços `api` e `worker`.

| Variável | Obrigatória | Descrição |
|----------|-------------|-----------|
| `POSTGRES_URL` | Sim | URL de conexão com o PostgreSQL (ex.: `postgres://user:pass@db:5432/charging?sslmode=disable`). |
| `INTELBRAS_BASE_URL` | Sim | Base da API Move/Intelbras (ex.: `https://cs-test.use-move.com/api/v1`). |
| `API_KEY` | Sim* | Chave para autorizar chamadas à API (`X-API-Key`). \* Use `ALLOW_EMPTY_API_KEY=true` só em dev. |
| `ENCRYPTION_KEY` | Não | Chave para criptografar senha e API key no banco. Pode ser UUID, senha ou base64 (32 bytes). Se vazia, dados ficam em texto. |

---

## Como rodar

### Com Docker Compose (recomendado)

```bash
# Subir API, Worker e PostgreSQL
docker compose up -d --build

# Ver logs
docker compose logs -f
```

- **API**: http://localhost:8080  
- **Health**: http://localhost:8080/health  
- **Swagger**: http://localhost:8080/swagger/index.html  

### Aplicar migrations

Após o primeiro `up`, crie as tabelas (se ainda não existirem):

```bash
# Linux/macOS
cat migrations/001_init.sql migrations/002_webhook_events_payload_text.sql | docker exec -i ev-charging-db psql -U postgres -d charging

# Windows (PowerShell)
Get-Content migrations/001_init.sql | docker exec -i ev-charging-db psql -U postgres -d charging
Get-Content migrations/002_webhook_events_payload_text.sql | docker exec -i ev-charging-db psql -U postgres -d charging
```

### Local (sem Docker)

1. PostgreSQL rodando e banco `charging` criado.  
2. Defina as variáveis de ambiente (ou use um `.env` carregado por outro meio).  
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
| POST | `/v1/config` | Configura credenciais e webhook (body: email/username, password, webhookUrl, opcional apiKey, recaptchaResponse). Requer `X-API-Key` se `API_KEY` estiver definida. |
| DELETE | `/v1/config` | Remove o usuário informado (via body `email`/`username`) e todos os dados relacionados (credenciais, webhooks, eventos). |
| GET | `/v1/config/status` | Retorna se há configuração e se o token está presente (sem expor o token). |
| GET | `/v1/stations` | Lista estações da API de terceiros (usa token salvo ou renova). |

Respostas de erro usam mensagens genéricas (`invalid request`, `configuration failed`, `stations unavailable`, etc.); o detalhe é logado no servidor.

---

## Documentação Swagger

A documentação OpenAPI (Swagger) está disponível com a API rodando:

- **UI**: [http://localhost:8080/swagger/index.html](http://localhost:8080/swagger/index.html)
- **JSON**: [http://localhost:8080/swagger/doc.json](http://localhost:8080/swagger/doc.json)

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
| `clear_data.sql` | Limpa todos os dados (TRUNCATE), mantendo a estrutura. |

---

## Segurança

- **API_KEY**: Exigida no startup (exceto com `ALLOW_EMPTY_API_KEY=true`). Usada no header `X-API-Key` nas rotas `/v1/*`.  
- **ENCRYPTION_KEY**: Criptografia AES-256-GCM para senha e API key da Intelbras no banco.  
- **Rate limit**: 15 requisições/minuto por IP no grupo `/v1`.  
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
│   ├── api/           # Handlers, rotas, rate limit
│   ├── clients/
│   │   └── intelbras/ # Cliente HTTP (login + charge-points)
│   ├── config/        # Carregamento de env
│   ├── crypto/        # Criptografia (ENCRYPTION_KEY)
│   ├── database/      # Conexão PostgreSQL
│   ├── repository/    # Acesso a dados
│   └── service/       # Regras de negócio (config, stations, webhook, job, scheduler)
├── migrations/        # SQL de schema e utilitários
├── docker-compose.yml
├── Dockerfile         # Multi-stage: API + Worker
├── go.mod
├── .env.example
└── README.md
```
