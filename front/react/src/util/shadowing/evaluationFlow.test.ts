import { describe, expect, it } from 'vitest'
import { APIError } from '@/api/client'
import { classifyEvaluationError, recordButtonLabel } from './evaluationFlow'

describe('classifyEvaluationError', () => {
  it.each([
    {
      name: 'stale version is a refreshable failure',
      err: new APIError('ERR_SHADOWING_VERSION_STALE', 'stale', 409),
      kind: 'stale',
      message: '教材内容已更新',
    },
    {
      name: 'other API errors keep their message',
      err: new APIError('ERR_SHADOWING_AUDIO_UNSUPPORTED', 'bad audio', 422),
      kind: 'other',
      message: 'bad audio',
    },
    {
      name: 'plain errors keep their message',
      err: new Error('microphone unavailable'),
      kind: 'other',
      message: 'microphone unavailable',
    },
    {
      name: 'non-error values get a fallback',
      err: 'something odd',
      kind: 'other',
      message: '评分失败',
    },
    {
      name: 'null gets a fallback',
      err: null,
      kind: 'other',
      message: '评分失败',
    },
  ])('$name', ({ err, kind, message }) => {
    const got = classifyEvaluationError(err)
    expect(got.kind).toBe(kind)
    expect(got.message).toContain(message)
  })
})

describe('recordButtonLabel', () => {
  it.each([
    { name: 'idle', isRecording: false, isEvaluating: false, want: 'Record' },
    { name: 'recording', isRecording: true, isEvaluating: false, want: 'Stop recording' },
    { name: 'scoring', isRecording: false, isEvaluating: true, want: 'Scoring' },
    { name: 'recording wins over scoring', isRecording: true, isEvaluating: true, want: 'Stop recording' },
  ])('$name', ({ isRecording, isEvaluating, want }) => {
    expect(recordButtonLabel(isRecording, isEvaluating)).toContain(want)
  })

  it('uses common emoji instead of obscure Unicode symbols', () => {
    for (const label of [
      recordButtonLabel(false, false),
      recordButtonLabel(true, false),
      recordButtonLabel(false, true),
    ]) {
      // U+23xx (timers, stop squares) are known to be invisible on some platforms.
      expect(label).not.toMatch(/[\u2300-\u23FF]/)
    }
  })
})
