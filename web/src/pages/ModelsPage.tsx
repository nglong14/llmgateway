import { type FormEvent, useCallback, useEffect, useRef, useState } from 'react'
import { ApiError, apiRequest, getActiveApiKey, setActiveApiKey, streamChatCompletion } from '../api/client'

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
  const [temperature, setTemperature] = useState('')
  const [stream, setStream] = useState(true)
  const [reply, setReply] = useState<ChatResponse | null>(null)
  const [error, setError] = useState('')
  const [tryError, setTryError] = useState('')
  const [loading, setLoading] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [streaming, setStreaming] = useState(false)
  const [streamedText, setStreamedText] = useState('')
  const [streamFinish, setStreamFinish] = useState('')
  const [streamDone, setStreamDone] = useState(false)
  const abortRef = useRef<AbortController | null>(null)
  const tryPanelRef = useRef<HTMLElement | null>(null)

  useEffect(() => {
    if (selected) {
      tryPanelRef.current?.scrollIntoView({ behavior: 'smooth', block: 'start' })
    }
  }, [selected])

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
    setStreamedText('')
    setStreamFinish('')
    setStreamDone(false)
  }

  function stopStreaming() {
    abortRef.current?.abort()
  }

  async function tryModel(event: FormEvent) {
    event.preventDefault()
    if (!selected || streaming) return
    const request: Record<string, unknown> = {
      model: selected.id,
      messages: [{ role: 'user', content: prompt }],
      max_tokens: maxTokens,
      stream,
    }
    if (temperature.trim() !== '') {
      const value = Number(temperature)
      if (!Number.isNaN(value)) request.temperature = value
    }

    setTryError('')
    setReply(null)
    setStreamedText('')
    setStreamFinish('')
    setStreamDone(false)

    if (stream) {
      const controller = new AbortController()
      abortRef.current = controller
      setStreaming(true)
      try {
        for await (const chunk of streamChatCompletion('/v1/chat/completions', {
          method: 'POST',
          body: JSON.stringify(request),
        }, apiKey, controller.signal)) {
          const content = chunk.choices?.[0]?.delta?.content ?? ''
          if (content) setStreamedText((previous) => previous + content)
          const finish = chunk.choices?.[0]?.finish_reason
          if (finish) setStreamFinish(finish)
        }
      } catch (requestError) {
        if (!controller.signal.aborted) setTryError(friendlyError(requestError))
      } finally {
        abortRef.current = null
        setStreaming(false)
        setStreamDone(true)
      }
      return
    }

    setSubmitting(true)
    try {
      const result = await apiRequest<ChatResponse>('/v1/chat/completions', {
        method: 'POST',
        body: JSON.stringify(request),
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
        <section className="card try-panel" ref={tryPanelRef}>
          <div className="section-heading"><div><div className="eyebrow">Try it</div><h2>{selected.id}</h2></div><button className="icon-button" onClick={() => setSelected(null)} aria-label="Close">×</button></div>
          <form onSubmit={tryModel}>
            <label>Prompt<textarea value={prompt} onChange={(event) => setPrompt(event.target.value)} rows={5} placeholder="Ask this model anything…" required /></label>
            <div className="try-actions">
              <div className="try-fields">
                <label className="token-field">Max tokens<input type="number" min={1} max={8192} value={maxTokens} onChange={(event) => setMaxTokens(Number(event.target.value))} /></label>
                <label className="token-field">Temperature<input type="number" min={0} max={2} step={0.1} value={temperature} onChange={(event) => setTemperature(event.target.value)} placeholder="auto" /></label>
                <label className="check-field"><input type="checkbox" checked={stream} onChange={(event) => setStream(event.target.checked)} />Stream</label>
              </div>
              {streaming
                ? <button className="primary stop" type="button" onClick={stopStreaming}>Stop</button>
                : <button className="primary" disabled={submitting}>{submitting ? 'Sending…' : 'Send request'}</button>}
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
          {!reply && (streamedText || streaming || streamDone) && (
            <div className="response">
              <div className="response-label">Assistant response</div>
              <p>{streamedText || (streaming ? 'Waiting for response…' : 'No content returned.')}</p>
              <div className="response-meta">
                {streaming ? <span>Streaming…</span> : <span>Finish: {streamFinish || 'stream'}</span>}
              </div>
            </div>
          )}
        </section>
      )}
    </>
  )
}
