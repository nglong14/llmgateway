# LLM Gateway

## 1. Introduction
This project is an API gateway for LLMs that solves two primary problems:
1. **Cost Management:** Managing and monitoring token costs across multiple LLM providers.
2. **Format Unification:** Eliminating the integration complexity of multiple LLM providers. Clients send requests in one unified format, and the gateway automatically handles provider-specific downstream schemas.

Built in Go, this is a  project demonstrating production-ready backend patterns, focusing on resilience, observability, and performance.

## 2. Architecture Diagram

```text
+--------+     +-----------------------------------------------------------------+     +---------------------+
|        |     |                           LLM Gateway                           |     |                     |
| Client | --> | +----------------------+     +-----------------------------+    | --> |       OpenAI        |
|        |     | | Rate Limiter (Redis) | --> | Circuit Breaker (in-memory) |    |     |                     |
+--------+     | +----------------------+     +-----------------------------+    |     +---------------------+
               |                                             |                   |
               |                                             v                   |     +---------------------+
               |                                  +-------------------+          |     |                     |
               |                                  |  Provider Router  | -------- | --> |      Anthropic      |
               |                                  +-------------------+          |     |                     |
               +-----------------------------------------------------------------+     +---------------------+

Request Flows:
1. Happy path: Client → Rate Limiter (pass) → Circuit Breaker (closed) → Provider Router → Provider → 200 OK
2. Rate limit exceeded: Client → Rate Limiter (reject) → 429 Too Many Requests returned
3. Circuit breaker open: Client → Rate Limiter (pass) → Circuit Breaker (open) → 503 Service Unavailable returned

Observability Stack:
App stdout → Promtail → Loki → Grafana ← Prometheus ← App /metrics
```

## 3. Features
* **Dual-layer Token Bucket Rate Limiting (Redis):** A centralized token bucket mechanism controlling traffic at two levels: **Client-side** (identifying users via API keys/IPs to prevent one tenant from monopolizing resources) and **Provider-side** (globally enforcing safe limits on outgoing LLM requests to prevent upstream 429s). Matters because LLM APIs are expensive and we must prevent abuse across distributed gateway instances.
* **In-memory circuit breaker per provider:** A state machine that trips and stops sending requests to a provider if it fails consecutively. Matters because if an upstream like OpenAI goes down, the gateway should fail fast (503) rather than hanging client connections and exhausting gateway infrastructure with timeouts.
* **Unified request format (OpenAI, Anthropic, Gemini, Deepseek):** A single, consistent API schema accepted by the gateway, which translates requests on the fly. Matters because clients maintain one unified integration, dramatically simplifying client logic and standardizing inputs across distinct LLMs.
* **Prometheus metrics:** Application-level instrumentation exposing real-time operational data. Matters for alerting on SLA breaches and understanding exact system and upstream behavior under distinct loads.
* **Structured JSON logging with correlation IDs:** Deterministic JSON logs containing a unique request ID. Matters because it enables robust tracing of a single request's lifecycle across all subsystems, critical for debugging concurrent API streams.
* **Loki + Promtail log aggregation:** Promtail ships container stdout logs natively to Loki. Matters for centralizing infrastructure logs without the massive memory overhead of an ELK stack.
* **Grafana dashboards:** Unified visual dashboards for metrics and logs. Matters for providing actionable insights to rapidly diagnose issues and identify gateway health regressions all in one place.

## 4. Tech Stack

| Layer | Technology | Purpose |
| --- | --- | --- |
| **Language** | Go 1.21+ | High concurrency, strong standard library, fast startup and low memory footprint. |
| **Rate Limiter** | Redis | Centralized state for cross-replica token bucket rate limiting. |
| **Metrics** | Prometheus | Scraping and storing time-series observability data. |
| **Log Aggregation** | Loki & Promtail | Lightweight log indexing and aggregation via label matching. |
| **Visualization** | Grafana | Dashboarding metrics and querying logs via LogQL. |
| **Infrastructure** | Docker & Compose | Deterministic local environments and reliable deployment of the entire stack. |

## 5. Getting Started
**Prerequisites:** Docker, Docker Compose, Go 1.21+

**Steps:**
1. Clone the repository
   ```bash
   git clone <repo_url>
   cd llmgateway
   ```
2. Setup environment
   ```bash
   cp .env.example .env
   # Set OPENAI_API_KEY and ANTHROPIC_API_KEY inside .env
   ```
3. Boot the environment
   ```bash
   docker-compose up -d
   ```
4. Verify the gateway works:
   ```bash
   curl -X POST localhost:8080/v1/chat \
     -H "Content-Type: application/json" \
     -d '{
       "model": "gpt-3.5-turbo",
       "messages": [{"role": "user", "content": "Hello!"}]
     }'
   ```

**Services:**
* Gateway: `:8080`
* Grafana: `http://localhost:3000` (admin/admin)
* Prometheus: `http://localhost:9090`

## 6. Observability
Understanding the metrics:
* `gateway_requests_total`: The total number of HTTP requests processed. Extremely valuable for measuring load and identifying distinct error rates across providers.
* `gateway_request_duration_seconds`: Histogram of latency across requests. Essential to track p95 and p99 speeds upstream and pinpoint network degradation.
* `circuit_breaker_state`: Gauge indicating actual states (closed=0, half-open=1, open=2) of provider circuits.
* `rate_limit_hits_total`: Counter highlighting the total volume of rejected requests directly from the Redis layer.

**LogQL Queries (Grafana Loki Explorer):**
* All errors:
  ```logql
  {compose_service="gateway"} | json | level="error"
  ```
* Trace one request:
  ```logql
  {compose_service="gateway"} | json | correlation_id="<paste-id-from-logs>"
  ```

## 7. Load Test Results

| Metric | Result |
| --- | --- |
| Requests/sec | [INSERT RESULT] |
| p99 latency | [INSERT RESULT] |
| p95 latency | [INSERT RESULT] |
| error rate | [INSERT RESULT] |
| circuit breaker trips | [INSERT RESULT] |

## 8. Design Decisions

1. **Deep dive: Redis for dual rate limiting**
   Rate limiting requires a shared external state (Redis) to reliably throttle globally across multiple gateway instances. We implement this at two distinct layers:
   - **Client limitation:** Protects our own infrastructure from abusive clients or runaway scripts.
   - **Provider limitation:** Protects our upstream LLM accounts from getting blocked due to concurrency limit breaches.
   Using Redis guarantees a unified count, but we strictly avoid Redis for circuit breakers (see below).
2. **Token bucket algorithm**
   A standard fixed window allows unpredictable traffic bursts right after window resets, overloading services. A token bucket guarantees sustained steady-state traffic handling and matches upstream behaviors of primary LLM providers like OpenAI.
3. **Deep dive: In-memory circuit breaker, NOT Redis**
   Circuit breakers are a reactive protection mechanism assessing immediate network health. Maintaining this as localized in-memory state means we fail quickly (reducing latency) and accurately without an extra network hop to Redis. If Redis goes down, our rate limiter might fail open or closed, but our circuit breaker MUST still protect upstream connections and client latency.
4. **Unified request format**
   If integrating deeply with Anthropic, OpenAI, Gemini, and Deepseek, downstream clients often suffer rewriting their business logic payloads. Standardizing this into a single standard input resolves architectural sprawl on the frontend/client implementations by managing adaptation within a single middleware component.
5. **Deep dive: Loki over Elasticsearch**
   Elasticsearch uses heavy inverted indices that scan and index all text, requiring significant memory (JVM overhead) and CPU. Loki only indexes core metadata labels (like `level`, `service`, `correlation_id`) while leaving the actual log lines raw and compressed. This makes Loki highly resource-efficient for our lightweight infrastructure, perfectly suited for structured JSON logs, and natively integrated with Grafana for immediate querying.

## 9. License

MIT License