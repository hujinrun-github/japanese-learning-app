import { afterEach, describe, expect, it, vi } from 'vitest'
import type { EvaluationResult } from '@/types/shadowing'
import { buildEvaluationFormData, evaluateShadowing } from './shadowing'

function resultFixture(): EvaluationResult {
  return {
    status: 'scoreable',
    mode: 'shadow',
    overall_score: 88,
    partial: true,
    scores: {},
    recognized_text: '',
    lag_ms: 600,
    issues: [],
    score_source: 'local_prosody_v1',
    scorer_version: 'local-prosody-1.1',
    weights_version: 'w-shadow-1.1',
    model_version: 'none',
    shadowing_version: 1,
    latency_ms: 30,
  }
}

function requestFixture() {
  return {
    audio: new Blob(['wav-bytes'], { type: 'audio/wav' }),
    mode: 'shadow' as const,
    sentence_index: 0,
    shadowing_version: 1,
  }
}

afterEach(() => {
  vi.unstubAllGlobals()
})

describe('buildEvaluationFormData', () => {
  it('sends the exact multipart fields the backend handler reads', () => {
    const form = buildEvaluationFormData(requestFixture())
    expect(form.get('mode')).toBe('shadow')
    expect(form.get('sentence_index')).toBe('0')
    expect(form.get('shadowing_version')).toBe('1')
  })

  it('attaches the WAV as a named file part', () => {
    const form = buildEvaluationFormData(requestFixture())
    const audio = form.get('audio')
    expect(audio).toBeInstanceOf(Blob)
    expect((audio as File).name).toBe('attempt.wav')
    expect((audio as File).size).toBeGreaterThan(0)
  })

  it.each([
    { name: 'shadow mode', mode: 'shadow' as const, sentence: 0, version: 1 },
    { name: 'repeat mode', mode: 'repeat' as const, sentence: 5, version: 2 },
    { name: 'larger indices', mode: 'shadow' as const, sentence: 42, version: 9 },
  ])('$name', ({ mode, sentence, version }) => {
    const form = buildEvaluationFormData({
      audio: new Blob(['x'], { type: 'audio/wav' }),
      mode,
      sentence_index: sentence,
      shadowing_version: version,
    })
    expect(form.get('mode')).toBe(mode)
    expect(form.get('sentence_index')).toBe(String(sentence))
    expect(form.get('shadowing_version')).toBe(String(version))
  })
})

describe('evaluateShadowing', () => {
  it('posts multipart to the evaluation endpoint and unwraps data', async () => {
    vi.stubGlobal('localStorage', { getItem: () => null, removeItem: () => {} })
    const fetchMock = vi.fn(async (_path: string, _init?: RequestInit) => ({
      ok: true,
      status: 200,
      json: async () => ({ data: resultFixture() }),
    }))
    vi.stubGlobal('fetch', fetchMock)

    const got = await evaluateShadowing(7, requestFixture())
    expect(got).toEqual(resultFixture())

    const [path, init] = fetchMock.mock.calls[0]
    if (!init) throw new Error('fetch was called without init')
    expect(path).toBe('/api/v1/lessons/7/shadowing/evaluations')
    expect(init.method).toBe('POST')
    expect(init.body).toBeInstanceOf(FormData)
    expect((init.body as FormData).get('sentence_index')).toBe('0')
    // apiFetch must not force a Content-Type for FormData (browser sets the boundary).
    expect(init.headers).toEqual({})
  })

  it('passes through the measured evidence and dimension on advice items', async () => {
    vi.stubGlobal('localStorage', { getItem: () => null, removeItem: () => {} })
    const fixture = resultFixture()
    fixture.issues = [{
      type: 'sync_lag_late',
      confidence: 0.9,
      dimension: 'sync',
      evidence_zh: '起步延迟 1500 ms，目标约 600 ms，晚了 900 ms',
      message_zh: '开口比原音晚了不少',
      action_zh: '听到第一个音就立刻开口',
    }]
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: true,
      status: 200,
      json: async () => ({ data: fixture }),
    })))

    const got = await evaluateShadowing(7, requestFixture())
    expect(got.issues[0]).toEqual(fixture.issues[0])
  })

  it('propagates a stale-version APIError', async () => {
    vi.stubGlobal('localStorage', { getItem: () => null, removeItem: () => {} })
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: false,
      status: 409,
      json: async () => ({
        code: 'ERR_SHADOWING_VERSION_STALE',
        message: 'shadowing version is stale',
        details: { current_shadowing_version: 2 },
      }),
    })))

    await expect(evaluateShadowing(7, requestFixture())).rejects.toMatchObject({
      code: 'ERR_SHADOWING_VERSION_STALE',
      status: 409,
    })
  })
})
