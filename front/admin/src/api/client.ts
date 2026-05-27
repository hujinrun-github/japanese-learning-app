const BASE = '/api/admin'

function token(): string {
  return sessionStorage.getItem('admin_token') || ''
}

export async function adminFetch<T>(method: string, path: string, body?: unknown): Promise<T> {
  const headers: Record<string, string> = {
    'Authorization': `Bearer ${token()}`,
  }
  let bodyInit: BodyInit | undefined
  if (body instanceof FormData) {
    bodyInit = body
  } else if (body !== undefined) {
    headers['Content-Type'] = 'application/json'
    bodyInit = JSON.stringify(body)
  }
  const res = await fetch(BASE + path, { method, headers, body: bodyInit })
  if (res.status === 204) return undefined as T
  const json = await res.json()
  if (!res.ok) throw new Error(json.error || `HTTP ${res.status}`)
  return json as T
}
