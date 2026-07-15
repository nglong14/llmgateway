# AGENTS.md

## Commands

```bash
make build          # go build -o bin/gateway.exe ./cmd/gateway
make run            # go run ./cmd/gateway --config configs/gateway.yaml
make test           # go test ./... -v -count=1 -race
make docker-up      # docker compose up --build -d  (gateway + redis + prometheus + grafana + loki + promtail)
make docker-down    # docker compose down
make k6-smoke       # k6 run loadtests/smoke.js
```

## Architecture

Single-module Go project (`github.com/nglong14/llmgateway`, go 1.25.0). No monorepo, no codegen, no database migrations.

**Entrypoint:** `cmd/gateway/main.go` — wires providers, middleware, and servers with graceful shutdown.

**Routes** (`:8080`):
- `GET /health`
- `GET /v1/models`
- `POST /v1/chat/completions` (streaming + non-streaming)

**Admin** (`:9091`): `GET /metrics` (Prometheus)

**Middleware chain** (order matters in `internal/router/router.go:17`):
```
LoggingMiddleware → Recoverer → PrometheusMiddleware → (if auth enabled) AuthMiddleware → RateLimitMiddleware → Handler
```

## Config

`configs/gateway.yaml` uses `${ENV_VAR}` expansion via regex. API keys and Redis credentials are never hardcoded. Load order: `.env` (godotenv) → `--config` YAML → env var substitution → validation.

Auth keys are SHA-256 hex digests (64-char hex), not plaintext. The incoming `Authorization: Bearer <token>` is hashed and looked up via O(1) map.

## Provider system

**Interface** (`internal/provider/provider.go`): `Name()`, `ChatCompletion()`, `ChatCompletionStream()`, `ListModels()`, `HealthCheck()`.

**Model resolution**: by prefix match (`gpt-` → openai, `claude-` → anthropic, `gemini-`/`g-` → gemini, `deepseek-` → deepseek). Can also resolve by explicit `provider` field in the request body.

**Decorator chain** (wrapping order: outer → inner): Rate Limiter → Circuit Breaker → Core Provider. Wired in `main.go:79-88`.

**Format translation**: Anthropic and Gemini require request/response translation to/from OpenAI-compatible format. OpenAI and DeepSeek pass through directly (DeepSeek is OpenAI-compatible).

## Rate limiting

Two layers:
1. **Per-IP/key** (HTTP middleware): token bucket, Redis-backed when available, falls back to in-memory.
2. **Per-provider RPM** (decorator): protects upstream API quotas, same Redis/in-memory pattern.

Redis is optional. Gateway starts with in-memory limiters; wraps with Redis-backed ones if connection succeeds. If Redis is unreachable, startup logs a warning and continues with in-memory fallback.

## Streaming

SSE format (`data: {...}\n\n`). Chunks written via `internal/streaming/sse.go`. Stream ends with `data: [DONE]\n\n`.

## Known issues (from CODE_REVIEW.md)

- **Zero test coverage** — no `*_test.go` files exist.
- **Streaming handler spin loop** — when `chunks` channel closes before `errCh` (`handlers.go:229-231`).
- **Circuit breaker goroutine leak** — proxy goroutine not canceled when caller cancels context.
- **No request timeout for streaming** — `WriteTimeout: 0` in server config.
- **Health checks call `ListModels`** — wastes upstream quota.
- **In-memory rate limiter unbounded growth** — `sync.Map` with no size cap.

## Testing

No unit or integration tests exist. k6 load test scripts in `loadtests/` test the running gateway as a black box. The `make test` target runs `go test ./...` but finds nothing.

## Logging

Structured JSON via `log/slog` to stdout. If `/var/log/gateway/` exists, also writes to `/var/log/gateway/gateway.log`.
