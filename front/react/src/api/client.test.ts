import { afterEach, describe, expect, test, vi } from 'vitest'

import { APIError, apiFetch } from './client'

describe('apiFetch', () => {
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  test('preserves details from flat API error responses', async () => {
    vi.stubGlobal('localStorage', {
      getItem: vi.fn(() => null),
      removeItem: vi.fn(),
    })
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: false,
      status: 409,
      json: async () => ({
        code: 'ERR_SHADOWING_VERSION_STALE',
        message: 'shadowing version is stale',
        request_id: '',
        details: {
          current_shadowing_version: 2,
        },
      }),
    })))

    let caught: unknown
    try {
      await apiFetch('POST', '/api/shadowing', { shadowing_version: 1 })
    } catch (err) {
      caught = err
    }

    expect(caught).toBeInstanceOf(APIError)
    expect(caught).toMatchObject({
      code: 'ERR_SHADOWING_VERSION_STALE',
      status: 409,
      details: {
        current_shadowing_version: 2,
      },
    })
  })

  test('does not redirect on login 401 so the login form can keep its email', async () => {
    vi.stubGlobal('localStorage', {
      getItem: vi.fn(() => null),
      removeItem: vi.fn(),
    })
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: false,
      status: 401,
      json: async () => ({
        code: 'ERR_INVALID_CREDENTIALS',
        message: 'invalid email or password',
      }),
    })))

    const location = { href: 'http://localhost/login?email=learner@example.com' }
    vi.stubGlobal('window', { location })

    await expect(apiFetch('POST', '/api/v1/auth/login', {
      email: 'learner@example.com',
      password: 'wrong-password',
    })).rejects.toMatchObject({
      code: 'ERR_INVALID_CREDENTIALS',
      status: 401,
    })

    expect(location.href).toBe('http://localhost/login?email=learner@example.com')
  })
})
