import { type FormEvent, useCallback, useEffect, useState } from 'react'
import { apiRequest, setActiveApiKey } from '../api/client'
import { useAuth } from '../auth/AuthContext'

interface APIKey {
  id: string
  key_prefix: string
  name: string
  last_used_at?: string
  created_at: string
  revoked_at?: string
}

interface CreatedKey {
  id: string
  name: string
  key_prefix: string
  api_key: string
  created_at: string
}

function formatDate(value?: string) {
  return value ? new Date(value).toLocaleString() : 'Never'
}

export function HomePage() {
  const { sessionRequest } = useAuth()
  const [keys, setKeys] = useState<APIKey[]>([])
  const [health, setHealth] = useState<'checking' | 'healthy' | 'unavailable'>('checking')
  const [name, setName] = useState('')
  const [newKey, setNewKey] = useState('')
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(true)
  const [creating, setCreating] = useState(false)

  const loadKeys = useCallback(async () => {
    try {
      const result = await sessionRequest<{ keys: APIKey[] }>('/auth/keys')
      setKeys(result.keys)
      setError('')
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to load API keys.')
    } finally {
      setLoading(false)
    }
  }, [sessionRequest])

  useEffect(() => {
    void loadKeys()
    apiRequest<unknown>('/health')
      .then(() => setHealth('healthy'))
      .catch(() => setHealth('unavailable'))
  }, [loadKeys])

  async function createKey(event: FormEvent) {
    event.preventDefault()
    setCreating(true)
    setError('')
    try {
      const created = await sessionRequest<CreatedKey>('/auth/keys', {
        method: 'POST',
        body: JSON.stringify({ name: name || undefined }),
      })
      setNewKey(created.api_key)
      setActiveApiKey(created.api_key)
      setName('')
      await loadKeys()
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to create API key.')
    } finally {
      setCreating(false)
    }
  }

  async function revokeKey(key: APIKey) {
    if (!window.confirm(`Revoke “${key.name}”? Requests using it will stop working.`)) return
    try {
      await sessionRequest<void>(`/auth/keys/${key.id}`, { method: 'DELETE' })
      await loadKeys()
    } catch (requestError) {
      setError(requestError instanceof Error ? requestError.message : 'Unable to revoke API key.')
    }
  }

  async function copyKey() {
    await navigator.clipboard.writeText(newKey)
    setCopied(true)
  }

  return (
    <>
      <div className="page-heading">
        <div><div className="eyebrow">Overview</div><h1>Gateway home</h1><p className="muted">Manage credentials used to call the LLM Gateway.</p></div>
        <span className={`status ${health}`}><i /> Gateway {health}</span>
      </div>

      <section className="card">
        <div className="section-heading">
          <div><h2>API keys</h2><p className="muted">Use these keys as Bearer tokens for models and chat completions.</p></div>
          <form className="inline-form" onSubmit={createKey}>
            <input value={name} onChange={(event) => setName(event.target.value)} placeholder="Key name" aria-label="Key name" />
            <button className="primary" disabled={creating}>{creating ? 'Creating…' : 'Create key'}</button>
          </form>
        </div>
        {error && <div className="alert error">{error}</div>}
        {loading ? <p className="empty">Loading keys…</p> : keys.length === 0 ? <p className="empty">No API keys yet.</p> : (
          <div className="table-wrap">
            <table>
              <thead><tr><th>Name</th><th>Prefix</th><th>Created</th><th>Last used</th><th>Status</th><th /></tr></thead>
              <tbody>{keys.map((key) => (
                <tr key={key.id}>
                  <td className="strong">{key.name}</td>
                  <td><code>{key.key_prefix}</code></td>
                  <td>{formatDate(key.created_at)}</td>
                  <td>{formatDate(key.last_used_at)}</td>
                  <td><span className={`badge ${key.revoked_at ? 'revoked' : 'active'}`}>{key.revoked_at ? 'Revoked' : 'Active'}</span></td>
                  <td><button className="danger-link" disabled={Boolean(key.revoked_at)} onClick={() => revokeKey(key)}>Revoke</button></td>
                </tr>
              ))}</tbody>
            </table>
          </div>
        )}
      </section>

      {newKey && (
        <div className="modal-backdrop" role="presentation">
          <section className="modal" role="dialog" aria-modal="true" aria-labelledby="new-key-title">
            <h2 id="new-key-title">Your new API key</h2>
            <p className="muted">Copy it now. It will not be shown again and has been set as the active model key.</p>
            <code className="secret">{newKey}</code>
            <div className="modal-actions">
              <button className="secondary" onClick={copyKey}>{copied ? 'Copied' : 'Copy key'}</button>
              <button className="primary" onClick={() => { setNewKey(''); setCopied(false) }}>Done</button>
            </div>
          </section>
        </div>
      )}
    </>
  )
}
