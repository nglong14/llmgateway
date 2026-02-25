# Foundation Plan — Bare-Minimum Project Skeleton

> **Goal:** Get a working Go project that compiles, runs an HTTP server, loads config, and has stub endpoints — **before** any provider abstractions, interfaces, or normalization logic.

> **Status: ✅ COMPLETE** — All files created, project compiles, server runs, and stub endpoints respond.

---

## What This Covers

```mermaid
graph LR
    A["go mod init"] --> B["Config loader"]
    B --> C["HTTP server<br/>(go-chi)"]
    C --> D["Stub endpoints"]
    D --> E["Makefile"]

    style A fill:#1a1a2e,color:#fff
    style B fill:#16213e,color:#fff
    style C fill:#0f3460,color:#fff
    style D fill:#e94560,color:#fff
    style E fill:#16213e,color:#fff
```

This plan produces **5 files** and a runnable server you can `curl` against. Nothing abstract — just concrete, working code.

---

## Project Structure

```
d:\LLMGateway\
├── cmd/
│   └── gateway/
│       └── main.go            ← Entry point (Step 3)
├── configs/
│   └── gateway.yaml           ← Default config (Step 2)
├── internal/
│   └── config/
│       └── config.go          ← Config loader (Step 2)
├── go.mod                     ← Module definition (Step 1)
├── go.sum                     ← Dependency checksums (Step 1)
└── Makefile                   ← Build/run shortcuts (Step 4)
```

> **Why this layout?** This follows the
> [Standard Go Project Layout](https://github.com/golang-standards/project-layout):
> - `cmd/` holds entry points (each subdirectory = one binary).
> - `internal/` holds private packages that cannot be imported by external Go modules.
> - `configs/` holds config file templates (not Go code).

---

## Step 1 · `go.mod` + `go.sum`  — Module Definition

### File: `go.mod`

```go
module github.com/nglong14/llmgateway

go 1.25.0

require (
	github.com/go-chi/chi/v5 v5.2.5 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
)
```

#### Line-by-line explanation

| Line | Code | Purpose |
|------|------|---------|
| 1 | `module github.com/nglong14/llmgateway` | **Module path.** This is the globally-unique identifier for this Go module. Every `import` within the project and from external consumers uses this path as a prefix. It matches the intended GitHub repo URL so `go get` will work once published. |
| 3 | `go 1.25.0` | **Go version directive.** Tells the toolchain the minimum Go version required. This enables language features and standard library additions up to Go 1.25. The toolchain uses this to decide which `go` semantics to apply. |
| 5–8 | `require ( ... )` | **Dependency declarations.** Lists every direct (and in this case transitive) dependency with its exact semantic version. |
| 6 | `github.com/go-chi/chi/v5 v5.2.5` | **chi router.** A lightweight, idiomatic HTTP router for Go. We use `chi` instead of the standard `http.ServeMux` because it provides composable middleware, URL parameter routing, and route grouping — all essential for the gateway's `/v1/*` API surface. The `v5` major version is the latest stable line. |
| 7 | `gopkg.in/yaml.v3 v3.0.1` | **YAML parser.** Used by `internal/config` to deserialize `gateway.yaml` into Go structs. Version 3 supports YAML 1.2, node-level control, and better error messages compared to v2. |

#### Why `// indirect`?

Go marks dependencies as `// indirect` when they appear in `go.mod` but are **not directly imported by any `.go` file in the module root package**. Since our imports live in sub-packages (`internal/config`, `cmd/gateway`) rather than a root-level `.go` file, `go mod tidy` marks them as indirect. This is purely cosmetic — the dependencies are still fully used. Running `go mod tidy` after more packages are added will typically promote them to direct.

### File: `go.sum`

```
github.com/go-chi/chi/v5 v5.2.5 h1:Eg4myHZBjyvJmAFjFvWgrqDTXFyOzjj7YIm3L3mu6Ug=
github.com/go-chi/chi/v5 v5.2.5/go.mod h1:X7Gx4mteadT3eDOMTsXzmI4/rwUpOwBHLpAfupzFJP0=
gopkg.in/check.v1 v0.0.0-20161208181325-20d25e280405/go.mod h1:Co6ibVJAznAaIkqp8huTwlJQCZ016jof/cbN4VW5Yz0=
gopkg.in/yaml.v3 v3.0.1 h1:fxVm/GzAzEWqLHuvctI91KS9hhNmmWOoWu0XTYJS7CA=
gopkg.in/yaml.v3 v3.0.1/go.mod h1:K4uyk7z7BCEPqu6E+C64Yfv1cQ7kz7rIZviUmN+EgEM=
```

#### What is `go.sum`?

`go.sum` is a **cryptographic lockfile**. It contains the **SHA-256 hashes** of every downloaded module zip and its `go.mod` file. Its purpose is to guarantee **reproducible builds** — if someone tampers with a dependency on a proxy, `go` will refuse to build because the hash won't match.

| Entry suffix | Meaning |
|--------------|---------|
| `h1:xxxx=` | Hash of the **module zip** (the actual source code download). |
| `/go.mod h1:xxxx=` | Hash of only the module's `go.mod` file. Go fetches `go.mod` separately when resolving the dependency graph, so it has its own checksum. |

> **Key rule:** Always commit `go.sum` to version control. Never edit it manually — let `go mod tidy` manage it.

**Done when:** `go.mod` exists with the two dependencies listed. ✅

---

## Step 2 · Config Loader

### File: `internal/config/config.go`

```go
package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the top-level gateway configuration.
type Config struct {
	Server    ServerConfig              `yaml:"server"`
	Providers map[string]ProviderConfig `yaml:"providers"`
}

// ServerConfig holds HTTP server settings.
type ServerConfig struct {
	Address string `yaml:"address"`
}

// ProviderConfig holds per-provider credentials and base URL.
type ProviderConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
}

// envVarPattern matches ${ENV_VAR} placeholders.
var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

// Load reads a YAML config file from path, expands any ${ENV_VAR}
// placeholders from the environment, and returns the parsed Config.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read file %s: %w", path, err)
	}

	// Expand ${ENV_VAR} placeholders before unmarshaling.
	expanded := expandEnvVars(string(data))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("config: parse yaml: %w", err)
	}

	return &cfg, nil
}

// expandEnvVars replaces every ${VAR} in s with the corresponding
// environment variable value. Missing variables resolve to "".
func expandEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		return os.Getenv(varName)
	})
}
```

#### Section-by-section breakdown

##### 1. Package & Imports (lines 1–10)

```go
package config
```
- Lives in `internal/config/`, so its import path is `github.com/nglong14/llmgateway/internal/config`.
- The `internal` directory is a **Go access-control mechanism** — packages under `internal/` can only be imported by code within the same module. This prevents external consumers from depending on our private config structs.

```go
import (
	"fmt"      // formatted error wrapping
	"os"       // file reading + environment variable access
	"regexp"   // regex for ${ENV_VAR} pattern matching
	"strings"  // TrimPrefix / TrimSuffix for extracting var names
	"gopkg.in/yaml.v3"  // YAML deserialization
)
```

##### 2. Config Structs (lines 12–27)

```go
type Config struct {
	Server    ServerConfig              `yaml:"server"`
	Providers map[string]ProviderConfig `yaml:"providers"`
}
```

- **`Config`** is the root struct that mirrors the top-level keys in `gateway.yaml`.
- **`Server`** is a nested struct (not a map) because the server config has a known, fixed shape.
- **`Providers`** is a `map[string]ProviderConfig` because the set of providers is **open-ended** — users can add/remove providers without touching Go code. The map key (e.g., `"openai"`, `"anthropic"`) becomes the provider's identifier throughout the system.

```go
type ServerConfig struct {
	Address string `yaml:"address"`
}
```
- Houses HTTP server settings. Currently only `Address` (e.g., `":8080"`), but structured as a separate type so we can add fields like `ReadTimeout`, `TLS`, etc. later without changing `Config`.

```go
type ProviderConfig struct {
	APIKey  string `yaml:"api_key"`
	BaseURL string `yaml:"base_url"`
}
```
- **`APIKey`**: The authentication credential for cloud providers. Uses `${ENV_VAR}` syntax in YAML so secrets never appear in plaintext config files.
- **`BaseURL`**: The root API URL for each provider. Different providers have different base URLs (OpenAI uses `/v1`, Anthropic uses a different path structure, Ollama is on localhost).

> **Struct tags** — The `` `yaml:"..."` `` tags tell `yaml.Unmarshal` which YAML key maps to which Go field. Without these, the YAML parser uses case-insensitive matching of the Go field name, which would require YAML keys like `apikey` instead of the more readable `api_key`.

##### 3. Environment Variable Expansion (lines 29–30)

```go
var envVarPattern = regexp.MustCompile(`\$\{([^}]+)\}`)
```

- Pre-compiled regex that matches the pattern `${ANYTHING_HERE}`.
- `\$\{` matches the literal characters `${`.
- `([^}]+)` captures one or more characters that are **not** `}` — this is the variable name.
- `\}` matches the closing `}`.
- Using a package-level `var` with `MustCompile` means the regex is compiled **once** at program startup. If the pattern were invalid, `MustCompile` would panic immediately rather than silently failing later.

##### 4. The `Load` Function (lines 32–49)

```go
func Load(path string) (*Config, error) {
```
- **Exported** (capital `L`) so `cmd/gateway/main.go` can call `config.Load(...)`.
- Returns `(*Config, error)` following Go's idiomatic "result, error" pattern.
- Takes a `path string` to keep the function **pure** — it doesn't hardcode a file path, making it testable.

```go
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config: read file %s: %w", path, err)
	}
```
- `os.ReadFile` slurps the entire file into memory. Fine for config files (typically < 1KB).
- Error wrapping with `%w` preserves the original error for `errors.Is()` / `errors.As()` inspection by callers.
- The `"config: "` prefix is a Go convention for namespacing errors to the package that produced them.

```go
	expanded := expandEnvVars(string(data))
```
- Runs **before** YAML parsing so that `${OPENAI_API_KEY}` in the raw YAML text gets replaced with the actual environment variable value. This approach is simpler than a custom YAML unmarshaler and works regardless of where in the YAML the placeholder appears.

```go
	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, fmt.Errorf("config: parse yaml: %w", err)
	}
	return &cfg, nil
```
- Declares `cfg` as a zero-value `Config`, then lets `yaml.Unmarshal` populate it.
- Returns a **pointer** (`*Config`) to avoid copying the struct on every return. Since `Providers` is a map (reference type), this also avoids subtle aliasing bugs.

##### 5. The `expandEnvVars` Helper (lines 51–58)

```go
func expandEnvVars(s string) string {
	return envVarPattern.ReplaceAllStringFunc(s, func(match string) string {
		varName := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		return os.Getenv(varName)
	})
}
```
- **Unexported** (lowercase `e`) — this is an internal implementation detail.
- `ReplaceAllStringFunc` calls the closure for every regex match. The `match` parameter is the full matched text (e.g., `"${OPENAI_API_KEY}"`).
- `TrimPrefix` / `TrimSuffix` strips the `${` and `}` delimiters to extract just `"OPENAI_API_KEY"`.
- `os.Getenv` returns the environment variable's value, or `""` if it's not set. This means unset API keys become empty strings rather than causing errors — a deliberate choice for development (Ollama has no API key).

---

### File: `configs/gateway.yaml`

```yaml
server:
  address: ":8080"

providers:
  openai:
    api_key: "${OPENAI_API_KEY}"
    base_url: "https://api.openai.com/v1"
  anthropic:
    api_key: "${ANTHROPIC_API_KEY}"
    base_url: "https://api.anthropic.com"
  gemini:
    api_key: "${GEMINI_API_KEY}"
    base_url: "https://generativelanguage.googleapis.com"
  ollama:
    base_url: "http://localhost:11434"
```

#### Key-by-key explanation

| Key | Value | Purpose |
|-----|-------|---------|
| `server.address` | `":8080"` | The Go `net` package interprets `":PORT"` as "listen on all interfaces on this port." Using `":8080"` (not `"localhost:8080"`) allows Docker containers and remote machines to reach the server. |
| `providers.openai.api_key` | `"${OPENAI_API_KEY}"` | Placeholder that gets expanded to the real key at load time via `expandEnvVars()`. This keeps secrets **out of version control**. |
| `providers.openai.base_url` | `"https://api.openai.com/v1"` | OpenAI's API root. The `/v1` suffix is part of their path convention; our gateway will append `/chat/completions`, `/models`, etc. |
| `providers.anthropic.base_url` | `"https://api.anthropic.com"` | Anthropic's API root. Note: no `/v1` — Anthropic uses a different versioning scheme (via headers). |
| `providers.gemini.base_url` | `"https://generativelanguage.googleapis.com"` | Google's Generative Language API endpoint. Models are specified in the URL path. |
| `providers.ollama` | No `api_key` | Ollama runs locally and requires **no authentication**. The YAML key simply omits `api_key`, which becomes an empty string in the Go struct (zero value for `string`). |
| `providers.ollama.base_url` | `"http://localhost:11434"` | Ollama's default local port. Uses HTTP (not HTTPS) since it's a local service. |

> **Design note:** This file is a **template**. In production, you'd either mount a different YAML or override values with environment variables. The `${...}` syntax gives us that flexibility without needing a separate `.env` file loader.

**Done when:** `config.Load("configs/gateway.yaml")` returns a populated struct and `${ENV_VAR}` values are expanded. ✅

---

## Step 3 · HTTP Server + Stub Endpoints

### File: `cmd/gateway/main.go`

```go
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nglong14/llmgateway/internal/config"
)

func main() {
	// 1. Parse --config flag.
	configPath := flag.String("config", "configs/gateway.yaml", "path to YAML config file")
	flag.Parse()

	// 2. Load config.
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// 3. Create chi router with basic middleware.
	r := chi.NewRouter()
	r.Use(middleware.Logger)
	r.Use(middleware.Recoverer)
	r.Use(middleware.SetHeader("Content-Type", "application/json"))

	// 4. Register routes.

	// GET /health — simple liveness probe.
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	// GET /v1/models — empty stub for now.
	r.Get("/v1/models", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"object": "list",
			"data":   []interface{}{},
		})
	})

	// POST /v1/chat/completions — not-implemented stub.
	r.Post("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotImplemented)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"error": map[string]string{
				"message": "not implemented",
				"type":    "server_error",
				"code":    "not_implemented",
			},
		})
	})

	// 5. Start HTTP server.
	srv := &http.Server{
		Addr:         cfg.Server.Address,
		Handler:      r,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start serving in a goroutine.
	go func() {
		fmt.Printf("🚀 LLM Gateway listening on %s\n", cfg.Server.Address)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	// 6. Graceful shutdown on SIGINT / SIGTERM.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	fmt.Printf("\n⏳ Received %s — shutting down gracefully…\n", sig)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("forced shutdown: %v", err)
	}
	fmt.Println("✅ Server stopped.")
}
```

#### Section-by-section breakdown

##### 1. Package Declaration & Imports (lines 1–18)

```go
package main
```
- `package main` is special in Go — it signals that this package builds an **executable binary** (not a library). The `main()` function inside it becomes the program's entry point.

**Standard library imports:**

| Import | Why |
|--------|-----|
| `context` | Used to create a timeout context for graceful shutdown. `srv.Shutdown(ctx)` will wait at most 10 seconds for in-flight requests to complete. |
| `encoding/json` | Writes JSON responses via `json.NewEncoder(w).Encode(...)`. We prefer streaming to `json.Marshal` + `w.Write` because the encoder writes directly to the `ResponseWriter` without allocating an intermediate `[]byte`. |
| `flag` | Parses the `--config` CLI flag. Simple and dependency-free. |
| `fmt` | Formatted printing for startup/shutdown messages. |
| `log` | `log.Fatalf` prints an error and calls `os.Exit(1)` — used for unrecoverable errors during startup. |
| `net/http` | The HTTP server, request/response types, and status codes. |
| `os` | Provides the `os.Signal` type and exit control. |
| `os/signal` | `signal.Notify` listens for OS signals (SIGINT, SIGTERM) in a channel. |
| `syscall` | Defines `syscall.SIGINT` and `syscall.SIGTERM` constants for cross-platform signal handling. |
| `time` | Duration constants for server timeouts. |

**Third-party imports:**

| Import | Why |
|--------|-----|
| `chi/v5` | Router. `chi.NewRouter()` creates A mux that supports `r.Get()`, `r.Post()`, URL params, and middleware chaining. |
| `chi/v5/middleware` | Pre-built middleware: `Logger` (request logging), `Recoverer` (panic recovery), `SetHeader` (default headers). |
| `internal/config` | Our config loader from Step 2. |

##### 2. `--config` Flag (lines 21–23)

```go
configPath := flag.String("config", "configs/gateway.yaml", "path to YAML config file")
flag.Parse()
```

- `flag.String` registers a `--config` flag with a default value. Returns a `*string` pointer.
- `flag.Parse()` processes `os.Args` and populates the flag values.
- **Why a flag instead of hardcoding?** So you can run `gateway --config /etc/gateway/prod.yaml` in production vs. the dev default. Zero-config for development, configurable for deployment.

##### 3. Config Loading (lines 25–29)

```go
cfg, err := config.Load(*configPath)
if err != nil {
	log.Fatalf("failed to load config: %v", err)
}
```

- Dereferences the flag pointer (`*configPath`) to get the string value.
- `log.Fatalf` is appropriate here because a missing/invalid config is **fatal** — the server cannot start without knowing its address and provider credentials.

##### 4. Router Setup (lines 31–35)

```go
r := chi.NewRouter()
r.Use(middleware.Logger)
r.Use(middleware.Recoverer)
r.Use(middleware.SetHeader("Content-Type", "application/json"))
```

- **`middleware.Logger`**: Logs every request with method, path, duration, and status code. Essential for debugging during development.
- **`middleware.Recoverer`**: Catches panics in handlers and returns a 500 instead of crashing the entire server. Critical for production resilience.
- **`middleware.SetHeader("Content-Type", "application/json")`**: Sets the default `Content-Type` on all responses. Since every endpoint in the gateway returns JSON, this avoids repeating `w.Header().Set(...)` in every handler.

> **Middleware execution order matters.** `Logger` wraps `Recoverer` wraps `SetHeader` wraps the handler. So:
> 1. Logger starts timing → 2. Recoverer catches panics → 3. Header is set → 4. Handler runs.

##### 5. Route Handlers (lines 37–64)

**`GET /health`** (lines 40–43)
```go
r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
})
```
- A **liveness probe** for load balancers and Kubernetes readiness checks.
- Returns `{"status":"ok"}` with a `200 OK`.
- Uses an anonymous map literal — no need for a named struct for such a simple response.

**`GET /v1/models`** (lines 46–52)
```go
r.Get("/v1/models", func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"object": "list",
		"data":   []interface{}{},
	})
})
```
- **Mimics the OpenAI `/v1/models` response shape** (`{"object":"list","data":[...]}`).
- Currently returns an empty list (`"data": []`). Once providers are implemented, this will aggregate models from all configured providers.
- The `interface{}` type is used because the response mixes strings and arrays.

**`POST /v1/chat/completions`** (lines 55–64)
```go
r.Post("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusNotImplemented)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"error": map[string]string{
			"message": "not implemented",
			"type":    "server_error",
			"code":    "not_implemented",
		},
	})
})
```
- The **core endpoint** that will eventually proxy chat completion requests to providers.
- Returns `501 Not Implemented` with an error body that follows the **OpenAI error response format** (`{"error":{"message":"...","type":"...","code":"..."}}`). This ensures clients get a structured error rather than a silent failure.
- The endpoint exists now as a placeholder so we can build integration tests against it incrementally.

##### 6. HTTP Server Configuration (lines 66–73)

```go
srv := &http.Server{
	Addr:         cfg.Server.Address,
	Handler:      r,
	ReadTimeout:  30 * time.Second,
	WriteTimeout: 60 * time.Second,
	IdleTimeout:  120 * time.Second,
}
```

| Field | Value | Why |
|-------|-------|-----|
| `Addr` | From config (`:8080`) | Decoupled from code — changeable via YAML. |
| `Handler` | `r` (chi router) | The router is an `http.Handler`, so it plugs directly into the standard server. |
| `ReadTimeout` | 30s | Maximum time to read the **entire request** (headers + body). Prevents slow-loris attacks where a client sends data very slowly to hold connections open. |
| `WriteTimeout` | 60s | Maximum time to write the **entire response**. Set higher than `ReadTimeout` because LLM responses (especially streaming) can be large. |
| `IdleTimeout` | 120s | How long to keep an idle keep-alive connection open. Higher values reduce connection churn for chatty clients. |

> **Why not use `http.ListenAndServe()`?** Creating an explicit `http.Server` struct gives us access to `srv.Shutdown()` for graceful shutdown, and lets us configure timeouts that `ListenAndServe` doesn't expose.

##### 7. Server Start (lines 75–81)

```go
go func() {
	fmt.Printf("🚀 LLM Gateway listening on %s\n", cfg.Server.Address)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server error: %v", err)
	}
}()
```

- **Why a goroutine?** `ListenAndServe()` blocks forever. Running it in a goroutine lets the main goroutine continue to the signal-handling code below.
- **`err != http.ErrServerClosed`**: When `srv.Shutdown()` is called, `ListenAndServe` returns `ErrServerClosed` — this is **expected**, not an error. We only `log.Fatalf` on **unexpected** errors (e.g., port already in use).

##### 8. Graceful Shutdown (lines 83–95)

```go
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
sig := <-quit
```

- Creates a **buffered channel** (capacity 1) to receive OS signals.
- `signal.Notify` registers interest in `SIGINT` (Ctrl+C) and `SIGTERM` (Docker/K8s stop).
- `<-quit` **blocks** the main goroutine until a signal arrives. This is the mechanism that keeps the program alive.

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()

if err := srv.Shutdown(ctx); err != nil {
	log.Fatalf("forced shutdown: %v", err)
}
```

- `srv.Shutdown(ctx)` does the following:
  1. **Stops accepting new connections** immediately.
  2. **Waits for in-flight requests** to complete (up to the context's 10-second deadline).
  3. **Closes idle connections** after all active requests finish.
- If requests don't complete within 10 seconds, the context expires and `Shutdown` returns an error, triggering `log.Fatalf` which force-exits.
- `defer cancel()` is a Go best practice — it releases the context's resources even if `Shutdown` succeeds before the timeout.

> **Why graceful shutdown matters for an LLM gateway:** LLM requests can take 5–30 seconds. Without graceful shutdown, a deploy/restart would kill in-flight requests, causing client errors.

---

## Step 4 · Makefile

### File: `Makefile`

```makefile
.PHONY: build run test clean

build:
	go build -o bin/gateway.exe ./cmd/gateway

run:
	go run ./cmd/gateway --config configs/gateway.yaml

test:
	go test ./... -v -count=1 -race

clean:
	rm -rf bin/
```

#### Line-by-line explanation

```makefile
.PHONY: build run test clean
```
- Declares these targets as **phony** — they don't correspond to real files. Without this, if a file named `build` existed in the directory, `make build` would say "nothing to do" because the "file" is already up to date. `.PHONY` forces `make` to always run the recipe.

```makefile
build:
	go build -o bin/gateway.exe ./cmd/gateway
```
- **`go build`**: Compiles the Go source into a binary.
- **`-o bin/gateway.exe`**: Specifies the output path. Uses `.exe` since we're on Windows. The `bin/` directory keeps build artifacts out of the source tree.
- **`./cmd/gateway`**: The import path of the `main` package to compile. The `./` prefix is relative to the module root.

```makefile
run:
	go run ./cmd/gateway --config configs/gateway.yaml
```
- **`go run`**: Compiles and runs in one step. Ideal for development — no need to manually build first.
- Passes the default config path so you can just type `make run`.

```makefile
test:
	go test ./... -v -count=1 -race
```
- **`./...`**: Recursively tests **all packages** in the module.
- **`-v`**: Verbose output — shows each test function name and result.
- **`-count=1`**: Disables test caching. Go normally caches passing test results and skips re-running them. `-count=1` forces every test to run fresh, which is important when tests depend on external state (environment variables, files).
- **`-race`**: Enables Go's **race detector**, which instruments memory accesses at runtime to find data races. Essential for a concurrent HTTP server. Adds ~2x overhead but catches real bugs.

```makefile
clean:
	rm -rf bin/
```
- Removes the build output directory. Standard hygiene target.

**Done when:** `make build` produces `bin/gateway.exe` and `make run` starts the server. ✅

---

## Verification Summary

| Test | Command | Expected Output | Status |
|------|---------|-----------------|--------|
| Build compiles | `go build ./...` | Exit code 0, no errors | ✅ |
| Server starts | `go run ./cmd/gateway --config configs/gateway.yaml` | `🚀 LLM Gateway listening on :8080` | ✅ |
| Health check | `GET /health` | `{"status":"ok"}` | ✅ |
| Models stub | `GET /v1/models` | `{"object":"list","data":[]}` | ✅ |
| Chat stub | `POST /v1/chat/completions` | `{"error":{"message":"not implemented",...}}` | ✅ |

---

## What Comes Next (Not in This Plan)

After this foundation is solid, the [implementation_plan.md](file:///d:/LLMGateway/.agent/implementation_plan.md) layers on:

1. Unified model types (`request.go`, `response.go`, `errors.go`)
2. Provider interface + registry
3. OpenAI provider (first end-to-end)
4. Normalization + SSE streaming
5. Router/handlers refactor (move out of `main.go`)
6. Remaining providers (Anthropic, Gemini, Ollama)
