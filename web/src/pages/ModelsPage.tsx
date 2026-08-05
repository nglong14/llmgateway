import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { ApiError, apiRequest, getActiveApiKey, setActiveApiKey } from '../api/client'

interface Model {
  id: string
  object: string
  owned_by: string
}

interface ChatResponse {
  choices: Array<{
    message: { role: string; content: string }
    finish_reason: string
  }>
  usage?: {
    prompt_tokens: number
    completion_tokens: number
    total_tokens: number
  }
}

function friendlyError(error: unknown) {
  if (!(error instanceof ApiError)) return error instanceof Error ? error.message : 'Request failed.'
  if (error.status === 401) return 'This API key is missing or invalid.'
  if (error.status === 404) return 'The selected model could not be found.'
  if (error.status === 429) return 'Rate limit reached. Wait a moment and try again.'
  if (error.status === 502) return 'The upstream model provider is unavailable.'
  return error.message
}

export function ModelsPage() {
  const [apiKey, setApiKey] = useState(getActiveApiKey)
  const [keyInput, setKeyInput] = useState(apiKey)
  const [models, setModels] = useState<Model[]>([])
  const [selected, setSelected] = useState<Model | null>(null)
  const [prompt, setPrompt] = useState('')
  const [maxTokens, setMaxTokens] = useState(512)
  const [reply, setReply] = useState<ChatResponse | null>(null)
  const [error, setError] = useState('')
  const [tryError, setTryError] = useState('')
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)

  const loadModels = useCallback(async (key: string) => {
    if (!key) return
    setLoading(true)
    setError('')
    try {
      const result = await apiRequest<{ data: Model[] }>('/v1/models', {}, key)
      setModels(result.data)
    } catch (requestError) {
      setModels([])
      setError(friendlyError(requestError))
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    void loadModels(apiKey)
  }, [apiKey, loadModels])

  function saveKey(event: FormEvent) {
    event.preventDefault()
    const next = keyInput.trim()
    setActiveApiKey(next)
    setApiKey(next)
  }

  function openTryIt(model: Model) {
    setSelected(model)
    setPrompt('')
    setReply(null)
    setTryError('')
  }

  async function tryModel(event: FormEvent) {
    event.preventDefault()
    if (!selected) return
    setSubmitting(true)
    setTryError('')
    setReply(null)
    try {
      const result = await apiRequest<ChatResponse>('/v1/chat/completions', {
        method: 'POST',
        body: JSON.stringify({
          model: selected.id,
          messages: [{ role: 'user', content: prompt }],
          max_tokens: maxTokens,
          stream: false,
        }),
      }, apiKey)
      setReply(result)
    } catch (requestError) {
      setTryError(friendlyError(requestError))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <>
      <div className="page-heading">
        <div><div className="eyebrow">Catalog</div><h1>Models</h1><p className="muted">Browse available providers and send a quick test request.</p></div>
      </div>

      <section className="card key-card">
        <form className="key-form" onSubmit={saveKey}>
          <label>Active gateway API key<input type="password" value={keyInput} onChange={(event) => setKeyInput(event.target.value)} placeholder="gw-…" /></label>
          <button className="secondary">Save and reload</button>
        </form>
        {!apiKey && <p className="hint">Create a key on Home or paste one here to load models.</p>}
      </section>

      <section className="card">
        <div className="section-heading"><div><h2>Available models</h2><p className="muted">{models.length} models across {new Set(models.map((model) => model.owned_by)).size} providers</p></div>{apiKey && <button className="secondary" onClick={() => loadModels(apiKey)}>Refresh</button>}</div>
        {error && <div className="alert error">{error}</div>}
        {loading ? <p className="empty">Loading models…</p> : models.length === 0 ? <p className="empty">No models to display.</p> : (
          <div className="table-wrap">
            <table>
              <thead><tr><th>Model</th><th>Provider</th><th /></tr></thead>
              <tbody>{models.map((model) => (
                <tr key={`${model.owned_by}-${model.id}`}>
                  <td className="strong">{model.id}</td>
                  <td><span className="provider">{model.owned_by}</span></td>
                  <td><button className="primary small" onClick={() => openTryIt(model)}>Try it</button></td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        )}
      </section>

      {selected && (
        <section className="card try-panel">
          <div className="section-heading"><div><div className="eyebrow">Try it</div><h2>{selected.id}</h2></div><button className="icon-button" onClick={() => setSelected(null)} aria-label="Close">×</button></div>
          <form onSubmit={tryModel}>
            <label>Prompt<textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} rows={5} placeholder="Ask this model anything…" required /></label>
            <div className="try-actions">
              <label className="token-field">Max tokens<input type="number" min={1} max={8192} value={maxTokens} onChange={(event) => setMaxTokens(Number(event.target.value))} /></label>
              <button className="primary" disabled={submitting}>{submitting ? 'Sending…' : 'Send request'}</button>
            </div>
          </form>
          {tryError && <div className="alert error">{tryError}</div>}
          {reply && (
            <div className="response">
              <div className="response-label">Assistant response</div>
              <p>{reply.choices[0]?.message.content || 'No content returned.'}</p>
              <div className="response-meta">
                <span>Finish: {reply.choices[0]?.finish_reason || 'unknown'}</span>
                {reply.usage && <span>Tokens: {reply.usage.prompt_tokens} prompt + {reply.usage.completion_tokens} completion = {reply.usage.total_tokens}</span>}
              </div>
            </div>
          )}
        </section>
      )}
    </>
  )
}
