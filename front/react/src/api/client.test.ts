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
})
