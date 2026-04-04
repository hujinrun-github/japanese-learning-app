/**
 * api.ts — Unified fetch wrapper with Bearer auth from localStorage.
 */

const TOKEN_KEY = "jla_token";

export function getToken(): string | null {
  return localStorage.getItem(TOKEN_KEY);
}

export function setToken(token: string): void {
  localStorage.setItem(TOKEN_KEY, token);
}

export function clearToken(): void {
  localStorage.removeItem(TOKEN_KEY);
}

export interface APIResponse<T> {
  data: T;
  error?: {
    code: string;
    message: string;
    detail: string;
  };
}

/**
 * apiFetch sends an authenticated JSON request to the API.
 *
 * @param path    API path, e.g. "/api/v1/words/queue"
 * @param init    Optional fetch init options (method, body, etc.)
 * @returns       The parsed `data` field from the API response envelope.
 * @throws        Error with a human-readable message on HTTP or API errors.
 */
export async function apiFetch<T>(
  path: string,
  init: RequestInit = {}
): Promise<T> {
  const headers: Record<string, string> = {
    "Content-Type": "application/json",
    ...(init.headers as Record<string, string>),
  };

  const token = getToken();
  if (token) {
    headers["Authorization"] = `Bearer ${token}`;
  }

  const response = await fetch(path, { ...init, headers });

  if (!response.ok) {
    let message = `HTTP ${response.status}`;
    try {
      const body: APIResponse<unknown> = await response.json();
      if (body.error?.message) {
        message = body.error.message;
      }
    } catch {
      // ignore JSON parse errors — use the HTTP status message
    }
    throw new Error(message);
  }

  const body: APIResponse<T> = await response.json();
  return body.data;
}
