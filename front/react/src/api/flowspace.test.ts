import { afterEach, describe, expect, it, vi } from 'vitest'
import { disconnectFlowSpace, getFlowSpaceIntegration, pairFlowSpace, retryFlowSpaceIntegration } from './flowspace'

describe('FlowSpace connection API', () => {
  afterEach(() => vi.unstubAllGlobals())

  function fetchResult(data: unknown, status = 200) {
    vi.stubGlobal('localStorage', { getItem: () => 'learner-session', removeItem: vi.fn() })
    const fetch = vi.fn(async () => ({ ok: status < 400, status, json: async () => data }))
    vi.stubGlobal('fetch', fetch)
    return fetch
  }

  it('reads authenticated connection status without sending integration credentials', async () => {
    const state = { available: true, connected: false, pending_count: 0, delivered_count: 0, events: [] }
    const fetch = fetchResult({ data: state })
    expect(await getFlowSpaceIntegration()).toEqual(state)
    expect(fetch).toHaveBeenCalledWith('/api/v1/integrations/flowspace', expect.objectContaining({
      method: 'GET', headers: { Authorization: 'Bearer learner-session' },
    }))
  })

  it('sends only the one-time pairing code to the learner backend', async () => {
    const fetch = fetchResult({ data: { connected: true } })
    await pairFlowSpace('temporary-code')
    expect(fetch).toHaveBeenCalledWith('/api/v1/integrations/flowspace/pair', expect.objectContaining({
      method: 'POST', body: JSON.stringify({ pairing_code: 'temporary-code' }),
    }))
  })

  it('preserves pairing errors for the form instead of pretending to connect', async () => {
    fetchResult({ code: 'ERR_PAIRING_EXPIRED', message: '配对码已过期，请重新生成' }, 409)
    await expect(pairFlowSpace('expired')).rejects.toMatchObject({
      code: 'ERR_PAIRING_EXPIRED', message: '配对码已过期，请重新生成', status: 409,
    })
  })

  it('supports retry and disconnect without exposing connection secrets', async () => {
    const fetch = fetchResult({ data: { available: true, connected: false } })
    await retryFlowSpaceIntegration()
    await disconnectFlowSpace()
    expect(fetch).toHaveBeenNthCalledWith(1, '/api/v1/integrations/flowspace/retry', expect.objectContaining({ method: 'POST' }))
    expect(fetch).toHaveBeenNthCalledWith(2, '/api/v1/integrations/flowspace', expect.objectContaining({ method: 'DELETE' }))
  })
})
