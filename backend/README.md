# backend

One Go module, three entrypoints — `api`, `bot`, `worker` (see `docs/architecture.md`, ADR-003/005).

## Layout

```
cmd/{api,bot,worker}   entrypoints (thin main + wiring)
internal/config        ENV configuration
internal/logging       structured JSON logger (slog)
internal/httpx         shared HTTP: /healthz, /readyz, /metrics, graceful run,
                        request-id/logging/recover middleware, error envelope
internal/auth          Auth Module attachment points (initData/adminJWT);
                        pass-through stubs until issue #5
internal/openapi       generated from docs/api/openapi.yaml via oapi-codegen
                        (models, ServerInterface, StrictServerInterface); see
                        generate.go and specprep/
internal/postgres      pgx pool + goose migration runner (api owns the schema)
internal/redis         Redis client (cache + cart; not a queue)
internal/rabbit        AMQP connection (queue via outbox relay)
migrations/            embedded goose SQL migrations
```

`api` mounts the contract surface under `/api/v1` with the request-id →
logging → recover middleware chain applied. No domain handlers exist yet, so
every contract operation currently answers with `501 Not Implemented` in the
unified error envelope (`{"error":{"code","message","details"}}`) via a
catch-all route; domain routers replace it incrementally as they implement
`openapi.StrictServerInterface`.

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

`docs/api/openapi.yaml` is OpenAPI 3.1 and uses 3.1's `type: [X, "null"]` /
`oneOf: [<schema>, {type: null}]` nullable idioms, which oapi-codegen v2 does
not yet understand (oapi-codegen/oapi-codegen#373). `go generate` first runs
`internal/openapi/specprep` to rewrite a throwaway, gitignored copy of the
spec into the 3.0-compatible `type: X` + `nullable: true` form, then runs
oapi-codegen against that copy. The contract file itself is never touched.
