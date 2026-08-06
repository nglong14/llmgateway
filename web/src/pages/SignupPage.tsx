import { type FormEvent, useState } from 'react'
import { Link, Navigate, useNavigate } from 'react-router-dom'
import { ApiError } from '../api/client'
import { useAuth } from '../auth/AuthContext'

export function SignupPage() {
  const { authenticated, signup } = useAuth()
  const navigate = useNavigate()
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [apiKey, setApiKey] = useState('')
  const [copied, setCopied] = useState(false)
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)

  if (authenticated && !apiKey) return <Navigate to="/" replace />

  async function handleSubmit(event: FormEvent) {
    event.preventDefault()
    setError('')
    setSubmitting(true)
    try {
      setApiKey(await signup(email, password, name))
    } catch (requestError) {
      if (requestError instanceof ApiError && requestError.status === 503) {
        setError('Authentication is unavailable. Check that PostgreSQL is running and migrated.')
      } else {
        setError(requestError instanceof Error ? requestError.message : 'Unable to create your account.')
      }
    } finally {
      setSubmitting(false)
    }
  }

  async function copyKey() {
    await navigator.clipboard.writeText(apiKey)
    setCopied(true)
  }

  return (
    <main className="auth-page">
      <section className="auth-card">
        <div className="eyebrow">LLM Gateway</div>
        <h1>Create account</h1>
        <p className="muted">Your first gateway API key is created automatically.</p>
        <form onSubmit={handleSubmit}>
          <label>Name <span className="optional">(optional)</span><input value={name} onChange={(event) => setName(event.target.value)} autoComplete="name" /></label>
          <label>Email<input type="email" value={email} onChange={(event) => setEmail(event.target.value)} required autoComplete="email" /></label>
          <label>Password<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} required autoComplete="new-password" minLength={8} /></label>
          {error && <div className="alert error">{error}</div>}
          <button className="primary full" disabled={submitting}>{submitting ? 'Creating account…' : 'Create account'}</button>
        </form>
        <p className="auth-switch">Already registered? <Link to="/login">Sign in</Link></p>
      </section>

      {apiKey && (
        <div className="modal-backdrop" role="presentation">
          <section className="modal" role="dialog" aria-modal="true" aria-labelledby="key-title">
            <div className="success-mark">✓</div>
            <h2 id="key-title">Save your API key</h2>
            <p className="muted">This key is only shown once. It is already set as your active key for model requests.</p>
            <code className="secret">{apiKey}</code>
            <div className="modal-actions">
              <button className="secondary" onClick={copyKey}>{copied ? 'Copied' : 'Copy key'}</button>
              <button className="primary" onClick={() => navigate('/')}>I saved it</button>
            </div>
          </section>
        </div>
      )}
    </main>
  )
}
