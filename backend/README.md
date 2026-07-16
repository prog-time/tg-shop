# backend

One Go module, three entrypoints — `api`, `bot`, `worker` (see `docs/architecture.md`, ADR-003/005).

## Layout

```
cmd/{api,bot,worker}   entrypoints (thin main + wiring)
internal/config        ENV configuration
internal/logging       structured JSON logger (slog)
internal/httpx         shared HTTP: /healthz, /readyz, /metrics, graceful run
internal/postgres      pgx pool + goose migration runner (api owns the schema)
internal/redis         Redis client (cache + cart; not a queue)
internal/rabbit        AMQP connection (queue via outbox relay)
migrations/            embedded goose SQL migrations
```

## Run

Locally the services run in Docker Compose from the repo root:

```
docker compose up --build
```

`api` (:8080) applies migrations on startup, then serves. `bot` (:8081) and
`worker` (:8082) expose only `/healthz` and `/metrics`.

## Build & checks

```
go build ./...
go vet ./...
```

## Code generation (contract-first)

Handlers are generated from the API contract via oapi-codegen (ADR-005):

```
go generate ./...   # regenerates internal/openapi from ../docs/api/openapi.yaml
```
