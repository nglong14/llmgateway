import {
  createContext,
  type PropsWithChildren,
  useCallback,
  useContext,
  useMemo,
  useState,
} from 'react'
import { apiRequest, setActiveApiKey } from '../api/client'

interface TokenResponse {
  access_token: string
  refresh_token: string
  token_type: string
  expires_in: number
}

interface StoredSession extends TokenResponse {
  expires_at: number
}

interface SignupResponse {
  user_id: string
  email: string
  api_key: string
}

interface AuthContextValue {
  authenticated: boolean
  login: (email: string, password: string) => Promise<void>
  signup: (email: string, password: string, name: string) => Promise<string>
  logout: () => void
  sessionRequest: <T>(path: string, init?: RequestInit) => Promise<T>
}

const SESSION_STORAGE = 'llmgateway.session'
const AuthContext = createContext<AuthContextValue | null>(null)

function readSession(): StoredSession | null {
  const value = localStorage.getItem(SESSION_STORAGE)
  if (!value) return null
  try {
    return JSON.parse(value) as StoredSession
  } catch {
    localStorage.removeItem(SESSION_STORAGE)
    return null
  }
}

function withExpiry(tokens: TokenResponse): StoredSession {
  return {
    ...tokens,
    expires_at: Date.now() + tokens.expires_in * 1000,
  }
}

export function AuthProvider({ children }: PropsWithChildren) {
  const [session, setSession] = useState<StoredSession | null>(readSession)

  const saveSession = useCallback((tokens: TokenResponse) => {
    const next = withExpiry(tokens)
    localStorage.setItem(SESSION_STORAGE, JSON.stringify(next))
    setSession(next)
    return next
  }, [])

  const logout = useCallback(() => {
    localStorage.removeItem(SESSION_STORAGE)
    setSession(null)
  }, [])

  const refresh = useCallback(async () => {
    if (!session?.refresh_token) {
      logout()
      throw new Error('Your session has expired. Please sign in again.')
    }
    try {
      const tokens = await apiRequest<TokenResponse>('/auth/refresh', {
        method: 'POST',
        body: JSON.stringify({ refresh_token: session.refresh_token }),
      })
      return saveSession(tokens)
    } catch (error) {
      logout()
      throw error
    }
  }, [logout, saveSession, session])

  const login = useCallback(async (email: string, password: string) => {
    const tokens = await apiRequest<TokenResponse>('/auth/token', {
      method: 'POST',
      body: JSON.stringify({ email, password }),
    })
    saveSession(tokens)
  }, [saveSession])

  const signup = useCallback(async (email: string, password: string, name: string) => {
    const account = await apiRequest<SignupResponse>('/auth/signup', {
      method: 'POST',
      body: JSON.stringify({ email, password, name: name || undefined }),
    })
    setActiveApiKey(account.api_key)
    await login(email, password)
    return account.api_key
  }, [login])

  const sessionRequest = useCallback(async <T,>(path: string, init: RequestInit = {}) => {
    let current = session
    if (!current) {
      throw new Error('Please sign in to continue.')
    }
    if (current.expires_at <= Date.now() + 30_000) {
      current = await refresh()
    }
    try {
      return await apiRequest<T>(path, init, current.access_token)
    } catch (error) {
      if (error instanceof Error && 'status' in error && error.status === 401) {
        current = await refresh()
        return apiRequest<T>(path, init, current.access_token)
      }
      throw error
    }
  }, [refresh, session])

  const value = useMemo<AuthContextValue>(() => ({
    authenticated: Boolean(session),
    login,
    signup,
    logout,
    sessionRequest,
  }), [login, logout, session, sessionRequest, signup])

  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>
}

// oxlint-disable-next-line react/only-export-components -- The hook is the provider's public API.
export function useAuth(): AuthContextValue {
  const context = useContext(AuthContext)
  if (!context) throw new Error('useAuth must be used inside AuthProvider')
  return context
}
