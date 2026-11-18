import { afterEach, describe, expect, it, vi } from 'vitest'
import { getWordAudioURL, playReferenceAudio, stopCurrent } from './exampleAudio'

afterEach(() => { vi.unstubAllGlobals() })

describe('configured reference playback', () => {
  it('plays the exact configured URL without synthesizing a replacement', async () => {
    const speak = vi.fn()
    vi.stubGlobal('speechSynthesis', { cancel: vi.fn(), speak })
    vi.stubGlobal('localStorage', { getItem: () => null })
    const urls: string[] = []
    class AudioStub {
      volume = 1
      onended: (() => void) | null = null
      onerror: (() => void) | null = null
      constructor(url: string) { urls.push(url) }
      play() { queueMicrotask(() => this.onended?.()); return Promise.resolve() }
      pause() {}
    }
    vi.stubGlobal('Audio', AudioStub)
    await playReferenceAudio('/api/v1/audio/17/stream')
    expect(urls).toEqual(['/api/v1/audio/17/stream'])
    expect(speak).not.toHaveBeenCalled()
  })

  it('reports a missing configured reference instead of silently switching voices', async () => {
    const speak = vi.fn()
    vi.stubGlobal('speechSynthesis', { cancel: vi.fn(), speak })
    vi.stubGlobal('localStorage', { getItem: () => null })
    class FailedAudio {
      volume = 1
      onerror: (() => void) | null = null
      play() { queueMicrotask(() => this.onerror?.()); return Promise.resolve() }
      pause() {}
    }
    vi.stubGlobal('Audio', FailedAudio)
    await expect(playReferenceAudio('/audio/missing.wav')).rejects.toThrow('Reference audio unavailable')
    expect(speak).not.toHaveBeenCalled()
    stopCurrent()
  })
})

describe('getWordAudioURL', () => {
  const tests = [
    { name: 'prefixes a stored filename', input: 'word.wav', want: '/audio/words/word.wav' },
    { name: 'keeps an API path', input: '/api/v1/audio/12', want: '/api/v1/audio/12' },
    { name: 'keeps an audio path', input: '/audio/words/word.wav', want: '/audio/words/word.wav' },
    { name: 'keeps an absolute URL', input: 'https://cdn.example.com/word.wav', want: 'https://cdn.example.com/word.wav' },
  ]

  for (const tt of tests) {
    it(tt.name, () => {
      expect(getWordAudioURL(tt.input)).toBe(tt.want)
    })
  }
})
