export interface ApiErrorBody {
  error?: {
    message?: string
    type?: string
    code?: string
  }
}

export class ApiError extends Error {
  status: number
  code?: string

  constructor(status: number, body: ApiErrorBody) {
    super(body.error?.message || `Request failed with status ${status}`)
    this.name = 'ApiError'
    this.status = status
    this.code = body.error?.code
  }
}

export async function apiRequest<T>(
  path: string,
  init: RequestInit = {},
  bearerToken?: string,
): Promise<T> {
  const headers = new Headers(init.headers)
  if (init.body && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  if (bearerToken) {
    headers.set('Authorization', `Bearer ${bearerToken}`)
  }

  const response = await fetch(path, { ...init, headers })
  if (!response.ok) {
    let body: ApiErrorBody = {}
    try {
      body = await response.json()
    } catch {
      // Keep the status-based fallback message for non-JSON errors.
    }
    throw new ApiError(response.status, body)
  }

  if (response.status === 204) {
    return undefined as T
  }
  return response.json() as Promise<T>
}

export const ACTIVE_API_KEY_STORAGE = 'llmgateway.activeApiKey'

export function getActiveApiKey(): string {
  return localStorage.getItem(ACTIVE_API_KEY_STORAGE) || ''
}

export function setActiveApiKey(apiKey: string): void {
  if (apiKey) {
    localStorage.setItem(ACTIVE_API_KEY_STORAGE, apiKey)
  } else {
    localStorage.removeItem(ACTIVE_API_KEY_STORAGE)
  }
}
