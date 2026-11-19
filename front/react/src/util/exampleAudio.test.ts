import { afterEach, describe, expect, it, vi } from 'vitest'
import { getWordAudioURL, playReferenceAudio, speakExample, speakWord, stopCurrent } from './exampleAudio'

afterEach(() => { vi.unstubAllGlobals(); vi.restoreAllMocks() })

describe('configured reference playback', () => {
  it('seeks and stops at the estimated sentence clip using metadata', async () => {
    vi.stubGlobal('speechSynthesis', { cancel: vi.fn() })
    vi.stubGlobal('localStorage', { getItem: () => null })
    let audio: ClipAudio
    class ClipAudio {
      duration = 20
      currentTime = 0
      volume = 1
      onloadedmetadata: (() => void) | null = null
      ontimeupdate: (() => void) | null = null
      onended: (() => void) | null = null
      onerror: (() => void) | null = null
      pause = vi.fn()
      play = vi.fn(() => Promise.resolve())
      constructor() { audio = this }
      load() { queueMicrotask(() => this.onloadedmetadata?.()) }
    }
    vi.stubGlobal('Audio', ClipAudio)
    const progress = vi.fn()
    const done = playReferenceAudio('/audio/original.wav', { startFraction: 0.25, endFraction: 0.5, onProgress: progress })
    await Promise.resolve()
    expect(audio!.currentTime).toBe(5)
    expect(audio!.play).toHaveBeenCalledOnce()
    audio!.currentTime = 10
    audio!.ontimeupdate?.()
    await done
    expect(audio!.pause).toHaveBeenCalled()
    expect(progress).toHaveBeenCalledWith(5, 20)
    expect(progress).toHaveBeenCalledWith(10, 20)
  })

  it('settles a stopped reference and ignores late metadata from it', async () => {
    vi.stubGlobal('speechSynthesis', { cancel: vi.fn() })
    vi.stubGlobal('localStorage', { getItem: () => null })
    const instances: DeferredAudio[] = []
    class DeferredAudio {
      duration = 10
      currentTime = 0
      volume = 1
      onloadedmetadata: (() => void) | null = null
      ontimeupdate: (() => void) | null = null
      onended: (() => void) | null = null
      onerror: (() => void) | null = null
      play = vi.fn(() => Promise.resolve())
      pause = vi.fn()
      constructor() { instances.push(this) }
      load() {}
    }
    vi.stubGlobal('Audio', DeferredAudio)
    const old = playReferenceAudio('/audio/old.wav', { startFraction: 0.5 })
    const lateMetadata = instances[0].onloadedmetadata
    const next = playReferenceAudio('/audio/new.wav')
    await old
    lateMetadata?.()
    expect(instances[0].play).not.toHaveBeenCalled()
    expect(instances[1].play).toHaveBeenCalledOnce()
    stopCurrent()
    await next
  })

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

describe('regenerated local audio bypasses stale cache', () => {
  it.each([
    { name: 'reference', play: () => playReferenceAudio('/audio/examples/ref.wav?version=2'), path: '/audio/examples/ref.wav?version=2&t=1234' },
    { name: 'word', play: () => speakWord('/audio/words/word.wav', 'こうじょう'), path: '/audio/words/word.wav?t=1234' },
    { name: 'example', play: () => speakExample('町に工場があります。'), path: '/audio/examples/' },
  ])('$name', async ({ play, path, name }) => {
    vi.spyOn(Date, 'now').mockReturnValue(1234)
    vi.stubGlobal('speechSynthesis', { cancel: vi.fn() })
    vi.stubGlobal('localStorage', { getItem: () => null })
    const urls: string[] = []
    class AudioStub {
      volume = 1
      onended: (() => void) | null = null
      constructor(url: string) { urls.push(url) }
      play() { queueMicrotask(() => this.onended?.()); return Promise.resolve() }
      pause() {}
    }
    vi.stubGlobal('Audio', AudioStub)
    await play()
    if (name === 'example') expect(urls[0]).toMatch(/^\/audio\/examples\/[a-f0-9]{16}\.wav\?t=1234$/)
    else expect(urls).toEqual([path])
    vi.restoreAllMocks()
  })
})
