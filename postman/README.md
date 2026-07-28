# Postman — Local Testing

Import these files into Postman to exercise the LLM Gateway locally.

| File | Purpose |
|------|---------|
| `llmgateway.postman_collection.json` | Requests for all public + admin endpoints |
| `llmgateway.postman_environment.json` | Variables (`baseUrl`, `gatewayApiKey`, etc.) |

## Prerequisites

1. **Provider API keys** — add upstream keys to `.env` in the project root:

   ```bash
   OPENAI_API_KEY=sk-...
   ANTHROPIC_API_KEY=sk-ant-...
   GEMINI_API_KEY=...
   DEEPSEEK_API_KEY=sk-...
   ```

2. **Gateway API key** — auth is enabled by default (`auth.enabled: true` in `configs/gateway.yaml`). Generate a hash for your local dev key:

   ```bash
   make hash-key
   ```

   Enter a plaintext key (e.g. `sk-local-dev`), then add the printed hash to `.env`:

   ```bash
   GATEWAY_API_KEY_HASH=<printed-sha256-hex>
   ```

   Set the same plaintext value as `gatewayApiKey` in the Postman environment.

   **No-auth shortcut:** set `auth.enabled: false` in `configs/gateway.yaml` to skip gateway authentication during local development.

3. **Redis** (optional) — the gateway falls back to in-memory rate limiting if Redis is unavailable. For Docker, set `REDIS_ADDR` and `REDIS_PASSWORD` in `.env`.

4. **PostgreSQL** (optional for static keys, required for `/auth/*`) — configure
   `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME`, and `DB_SSLMODE`.
   Also set strong `JWT_ACCESS_SECRET` and `JWT_REFRESH_SECRET` values.

## Start the gateway

```bash
# From project root
make run
```

Or with the full observability stack:

```bash
make docker-up
make migrate-up
```

The gateway listens on `:8080`; Prometheus metrics are on `:9091`.

## Import into Postman

1. Open Postman → **Import** → select both JSON files in this directory.
2. Select the **LLM Gateway Local** environment from the environment dropdown.
3. Set `authEmail` and `authPassword`, then run **Authentication** requests in
   order. Sign Up saves the initial `gatewayApiKey`; Issue Tokens saves
   `accessToken` and `refreshToken`; Create API Key saves `apiKeyId` and the new
   `gatewayApiKey`.
4. Run **Health** first (no auth required) to confirm the server is up.
5. Run **List Models** and **Chat Completion** requests (require
   `gatewayApiKey` + valid provider keys).

## Endpoints

| Request | Method | Auth | Notes |
|---------|--------|------|-------|
| Health | `GET /health` | None | Does not consume provider quota |
| Sign up | `POST /auth/signup` | None | Returns the initial plaintext API key once |
| Login | `POST /auth/login` | None | Verifies credentials; does not issue tokens |
| Issue tokens | `POST /auth/token` | None | Returns access and refresh JWTs |
| Refresh tokens | `POST /auth/refresh` | None | Rotates the refresh token |
| Create API key | `POST /auth/keys` | Access JWT | Returns plaintext key once; body may contain `name` |
| List API keys | `GET /auth/keys` | Access JWT | Never returns key hashes or plaintext keys |
| Revoke API key | `DELETE /auth/keys/{id}` | Access JWT | Revokes a key owned by the authenticated user |
| List Models | `GET /v1/models` | Bearer `gatewayApiKey` | Calls upstream `ListModels` per provider |
| Chat (non-stream) | `POST /v1/chat/completions` | Bearer | `"stream": false` |
| Chat (stream) | `POST /v1/chat/completions` | Bearer | `"stream": true` — SSE in response body |
| Metrics | `GET /metrics` on `:9091` | None | Prometheus text format |
| [Disabled] Chat (BYOK) | `POST /v1/chat/completions` | Bearer + `X-Provider-Key` | Not implemented yet |

## Troubleshooting

| Symptom | Likely cause |
|---------|--------------|
| `401 missing Authorization header` | Set `gatewayApiKey` in the environment, or disable auth in config |
| `401 invalid API key` | Plaintext key in Postman does not match `GATEWAY_API_KEY_HASH` in `.env` |
| `502 upstream_error` on chat | Provider key missing or invalid in `.env` |
| `404 model_not_found` | Change `model` variable to one your providers support |
| `429 rate_limit_exceeded` | Gateway rate limit hit — wait or raise `rate_limit.rps` in config |
| `503` from `/auth/*` | PostgreSQL is unavailable, migrations are pending, or JWT secrets are missing |

## Authentication workflow

1. Call `POST /auth/signup` with `email`, `password`, and optional `name`; save
   the returned `api_key` because it is shown only once.
2. Call `POST /auth/token` with the same credentials and save both JWTs.
3. Use the access JWT as `Authorization: Bearer <access_token>` for `/auth/keys`.
4. Call `POST /auth/refresh` with `{"refresh_token":"..."}` before access expiry;
   replace both stored tokens with the returned pair.
5. Use any generated gateway API key as the Bearer token for `/v1/*`.
