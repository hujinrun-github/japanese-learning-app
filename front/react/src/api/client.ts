import type { APIResponse } from '@/types/api'

export class APIError extends Error {
  code: string
  status: number
  details?: Record<string, unknown>

  constructor(code: string, message: string, status: number, details?: Record<string, unknown>) {
    super(message)
    this.name = 'APIError'
    this.code = code
    this.status = status
    this.details = details
  }
}

type APIErrorPayload = {
  error?: {
    code?: string
    message?: string
    details?: Record<string, unknown>
  }
  code?: string
  message?: string
  details?: Record<string, unknown>
}

function shouldRedirectToLoginOnUnauthorized(path: string): boolean {
  return path !== '/api/v1/auth/login'
}

export async function apiFetch<T>(
  method: string,
  path: string,
  body?: unknown,
  signal?: AbortSignal,
): Promise<T> {
  const token = localStorage.getItem('token')

  const headers: Record<string, string> = {}
  if (token) {
    headers['Authorization'] = `Bearer ${token}`
  }

  let bodyInit: BodyInit | undefined
  if (body instanceof FormData) {
    bodyInit = body
  } else if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
    bodyInit = JSON.stringify(body)
  }

  const res = await fetch(path, {
    method,
    headers,
    body: bodyInit,
    signal,
  })

  if (!res.ok) {
    let code = 'ERR_UNKNOWN'
    let message = `HTTP ${res.status}`
    let details: Record<string, unknown> | undefined
    try {
      const err = (await res.json()) as APIErrorPayload
      if (err.error) {
        code = err.error.code ?? code
        message = err.error.message ?? message
        details = err.error.details
      } else {
        // backend returns flat: { code, message, request_id }
        code = err.code ?? code
        message = err.message ?? message
        details = err.details
      }
    } catch {
      // ignore parse error
    }
    // Auto-redirect to login on 401 (expired or invalid token)
    if (res.status === 401 && shouldRedirectToLoginOnUnauthorized(path)) {
      localStorage.removeItem('token')
      localStorage.removeItem('user')
      if (typeof window !== 'undefined') {
        window.location.href = '/login'
      }
    }
    throw new APIError(code, message, res.status, details)
  }

  // 204 No Content
  if (res.status === 204) {
    return undefined as T
  }

  let json: APIResponse<T> = {} as APIResponse<T>
  try { json = (await res.json()) as APIResponse<T> } catch { /* empty body */ }
  return json.data
}
