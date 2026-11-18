import { afterEach, describe, expect, it, vi } from 'vitest'
import type { EvaluationResult } from '@/types/shadowing'
import { buildSpeakingEvaluationFormData, evaluateSpeaking } from './speaking'

function resultFixture(): EvaluationResult {
  return {
    status: 'scoreable',
    mode: 'repeat',
    overall_score: 82,
    partial: true,
    scores: {},
    recognized_text: '',
    lag_ms: 0,
    issues: [],
    score_source: 'local_prosody_v1',
    scorer_version: 'local-prosody-1.1',
    weights_version: 'w-repeat-1.1',
    model_version: 'none',
    shadowing_version: 0,
    latency_ms: 20,
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('buildSpeakingEvaluationFormData', () => {
  it.each([
    { name: 'small material id', material_id: 1 },
    { name: 'large material id', material_id: 987654 },
  ])('$name', ({ material_id }) => {
    const form = buildSpeakingEvaluationFormData({
      audio: new Blob(['wav-bytes'], { type: 'audio/wav' }),
      material_id,
    })
    expect(form.get('material_id')).toBe(String(material_id))
    const audio = form.get('audio')
    expect(audio).toBeInstanceOf(Blob)
    expect((audio as File).name).toBe('attempt.wav')
    expect((audio as File).size).toBeGreaterThan(0)
  })
})

describe('evaluateSpeaking', () => {
  it('posts multipart to the speaking evaluation endpoint and unwraps data', async () => {
    vi.stubGlobal('localStorage', { getItem: () => null, removeItem: () => {} })
    const fetchMock = vi.fn(async (_path: string, _init?: RequestInit) => ({
      ok: true,
      status: 200,
      json: async () => ({ data: resultFixture() }),
    }))
    vi.stubGlobal('fetch', fetchMock)

    const got = await evaluateSpeaking({
      audio: new Blob(['wav-bytes'], { type: 'audio/wav' }),
      material_id: 4,
    })
    expect(got).toEqual(resultFixture())

    const [path, init] = fetchMock.mock.calls[0]
    if (!init) throw new Error('fetch was called without init')
    expect(path).toBe('/api/v1/speaking/evaluate')
    expect(init.method).toBe('POST')
    expect(init.body).toBeInstanceOf(FormData)
    expect((init.body as FormData).get('material_id')).toBe('4')
    // apiFetch must not force a Content-Type for FormData (browser sets the boundary).
    expect(init.headers).toEqual({})
  })

  it('propagates a missing-reference APIError so the page can fall back to self rating', async () => {
    vi.stubGlobal('localStorage', { getItem: () => null, removeItem: () => {} })
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: false,
      status: 422,
      json: async () => ({
        code: 'ERR_SPEAKING_REFERENCE_MISSING',
        message: 'reference audio is unavailable for this material',
      }),
    })))

    await expect(evaluateSpeaking({
      audio: new Blob(['wav-bytes'], { type: 'audio/wav' }),
      material_id: 4,
    })).rejects.toMatchObject({
      code: 'ERR_SPEAKING_REFERENCE_MISSING',
      status: 422,
    })
  })
})
