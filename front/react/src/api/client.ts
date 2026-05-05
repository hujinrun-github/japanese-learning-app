import type { APIResponse } from '@/types/api'

export class APIError extends Error {
  code: string
  status: number

  constructor(code: string, message: string, status: number) {
    super(message)
    this.name = 'APIError'
    this.code = code
    this.status = status
  }
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
    try {
      const err = await res.json()
      if (err.error) {
        code = err.error.code ?? code
        message = err.error.message ?? message
      } else {
        // backend returns flat: { code, message, request_id }
        code = err.code ?? code
        message = err.message ?? message
      }
    } catch {
      // ignore parse error
    }
    // Auto-redirect to login on 401 (expired or invalid token)
    if (res.status === 401) {
      localStorage.removeItem('token')
      localStorage.removeItem('user')
      window.location.href = '/login'
    }
    throw new APIError(code, message, res.status)
  }

  // 204 No Content
  if (res.status === 204) {
    return undefined as T
  }

  const json = (await res.json()) as APIResponse<T>
  return json.data
}
