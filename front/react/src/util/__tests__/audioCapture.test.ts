import { describe, expect, it } from 'vitest'
import { scoringAudioConstraints } from '../audioCapture'

describe('scoringAudioConstraints', () => {
  it('disables every browser audio-processing feature that would reshape the envelope', () => {
    expect(scoringAudioConstraints()).toEqual({
      echoCancellation: false,
      noiseSuppression: false,
      autoGainControl: false,
    })
  })

  it('never enables a processing feature, even if new fields are added later', () => {
    const constraints = scoringAudioConstraints() as Record<string, unknown>
    for (const [key, value] of Object.entries(constraints)) {
      expect(value, `${key} must not be enabled`).not.toBe(true)
    }
  })

  it('returns a fresh object so callers cannot mutate shared defaults', () => {
    const first = scoringAudioConstraints() as Record<string, unknown>
    first.autoGainControl = true
    expect(scoringAudioConstraints().autoGainControl).toBe(false)
  })
})
