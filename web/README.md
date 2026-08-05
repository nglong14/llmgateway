# LLM Gateway Web

React + Vite frontend for account access, API key management, model discovery/testing, and gateway metrics.

## Run locally

Start the backend stack and apply database migrations from the repository root:

```bash
make docker-up
make migrate-up
```

Install and run the frontend:

```bash
cd web
npm install
npm run dev
```

Open `http://localhost:5173`. The Vite server proxies gateway requests to `http://localhost:8080`.

You can also start an already-installed frontend from the repository root with `make web`.

## Grafana

The Metrics page embeds the provisioned `LLM Gateway` dashboard from `http://localhost:3000`. Docker Compose enables anonymous Viewer access so the dashboard can load in an iframe. Restart the Grafana container after changing these settings.

## Build

```bash
npm run build
```
