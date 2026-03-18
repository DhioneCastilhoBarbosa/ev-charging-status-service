FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

COPY go.mod go.sum ./
RUN go mod download

COPY . .

RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/api ./cmd/api
RUN CGO_ENABLED=0 GOOS=linux go build -o /bin/worker ./cmd/worker

FROM alpine:3.19

WORKDIR /app

COPY --from=builder /bin/api /bin/api
COPY --from=builder /bin/worker /bin/worker
COPY migrations ./migrations

# Segredos e config (POSTGRES_URL, INTELBRAS_BASE_URL, API_KEY, ENCRYPTION_KEY)
# devem ser injetados em runtime (docker-compose, K8s, etc.). Não definir aqui.
EXPOSE 8085

# Um único container: worker em background, API em primeiro plano (PID 1).
CMD ["sh", "-c", "/bin/worker & exec /bin/api"]

