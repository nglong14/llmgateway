const GRAFANA_URL = 'http://localhost:3000/d/llm-gateway/llm-gateway?orgId=1&refresh=10s&kiosk'

export function MetricsPage() {
  return (
    <>
      <div className="page-heading">
        <div><div className="eyebrow">Observability</div><h1>Gateway metrics</h1><p className="muted">Live request, provider latency, and token usage telemetry.</p></div>
        <a className="secondary button-link" href={GRAFANA_URL} target="_blank" rel="noreferrer">Open in Grafana ↗</a>
      </div>
      <section className="grafana-card">
        <iframe title="LLM Gateway Grafana dashboard" src={GRAFANA_URL} allowFullScreen />
      </section>
      <p className="hint">Dashboard unavailable? Start or restart Docker Compose so Grafana picks up anonymous embedding settings.</p>
    </>
  )
}
