import { getVolume } from './audioVolume'

let audioEl: HTMLAudioElement | null = null
let stopReference: (() => void) | null = null

// Local files can be replaced after a correction while retaining their text hash.
function freshAudioURL(url: string): string {
  if (!url.startsWith('/audio/')) return url
  const [path, fragment] = url.split('#', 2)
  return `${path}${path.includes('?') ? '&' : '?'}t=${Date.now()}${fragment ? `#${fragment}` : ''}`
}

export function stopCurrent() {
  stopReference?.()
  stopReference = null
  if (audioEl) {
    audioEl.pause()
    audioEl = null
  }
  speechSynthesis.cancel()
}

// Configured speaking references must be played verbatim. Falling back to a
// different synthesizer would change the audio used as the scoring baseline.
export interface ReferencePlaybackOptions {
  /** Fractions refer to the same original audio, never a synthesized replacement. */
  startFraction?: number
  endFraction?: number
  onProgress?: (currentTime: number, duration: number) => void
}

export function playReferenceAudio(url: string, opts?: ReferencePlaybackOptions): Promise<void> {
  stopCurrent()
  return new Promise((resolve, reject) => {
    const audio = new Audio(freshAudioURL(url))
    audio.volume = getVolume()
    audioEl = audio
    let settled = false
    let endTime = Infinity
    const finish = (err?: Error) => {
      if (settled) return
      settled = true
      audio.onended = null
      audio.onerror = null
      audio.onloadedmetadata = null
      audio.ontimeupdate = null
      audio.pause()
      if (audioEl === audio) audioEl = null
      if (stopReference === cancel) stopReference = null
      if (err) reject(err)
      else resolve()
    }
    const cancel = () => finish()
    stopReference = cancel
    const reportProgress = () => {
      if (settled) return
      opts?.onProgress?.(audio.currentTime, audio.duration)
      if (audio.currentTime >= endTime) finish()
    }
    const start = () => {
      if (settled) return
      audio.play().catch(err => finish(err instanceof Error ? err : new Error('Reference audio unavailable')))
    }
    const clipped = opts?.startFraction !== undefined || opts?.endFraction !== undefined
    audio.onloadedmetadata = () => {
      if (settled) return
      if (clipped) {
        if (!Number.isFinite(audio.duration) || audio.duration <= 0) {
          finish(new Error('Reference audio duration unavailable'))
          return
        }
        const startFraction = Math.max(0, Math.min(1, opts?.startFraction ?? 0))
        const endFraction = Math.max(startFraction, Math.min(1, opts?.endFraction ?? 1))
        audio.currentTime = startFraction * audio.duration
        endTime = endFraction * audio.duration
        reportProgress()
        start()
      } else reportProgress()
    }
    audio.ontimeupdate = reportProgress
    audio.onended = () => finish()
    audio.onerror = () => finish(new Error('Reference audio unavailable'))
    if (clipped) {
      audio.preload = 'auto'
      audio.load()
    } else start()
  })
}

async function sha256Hex(text: string): Promise<string> {
  const data = new TextEncoder().encode(text)
  const hash = await crypto.subtle.digest('SHA-256', data)
  return Array.from(new Uint8Array(hash))
    .map(b => b.toString(16).padStart(2, '0'))
    .join('')
    .slice(0, 16)
}

export function getExampleAudioURL(hash: string): string {
  return `/audio/examples/${hash}.wav`
}

export async function getSpeakingReferenceURL(text: string, audioURL?: string): Promise<string> {
  return audioURL?.trim() || getExampleAudioURL(await sha256Hex(text))
}

export function getWordAudioURL(audioURL: string): string {
  if (audioURL.startsWith('/') || /^[a-z][a-z\d+.-]*:/i.test(audioURL)) {
    return audioURL
  }
  return `/audio/words/${audioURL}`
}

export interface SpeakOptions {
  onBoundary?: (charIndex: number) => void
  onEnd?: () => void
}

function fallbackTTS(text: string, opts?: SpeakOptions): Promise<void> {
  return new Promise((resolve) => {
    speechSynthesis.cancel()
    const u = new SpeechSynthesisUtterance(text)
    u.lang = 'ja-JP'
    u.rate = 0.9
    u.volume = getVolume()
    const voices = speechSynthesis.getVoices()
    const jaVoice = voices.find(v => v.lang.startsWith('ja'))
    if (jaVoice) u.voice = jaVoice
    if (opts?.onBoundary) {
      u.onboundary = (e) => opts.onBoundary!(e.charIndex)
    }
    u.onend = () => {
      opts?.onEnd?.()
      resolve()
    }
    u.onerror = () => resolve()
    speechSynthesis.speak(u)
  })
}

export async function speakExample(text: string, opts?: SpeakOptions): Promise<void> {
  stopCurrent()

  const hash = await sha256Hex(text)
  const url = getExampleAudioURL(hash)
  const vol = getVolume()

  return new Promise((resolve) => {
    const audio = new Audio(freshAudioURL(url))
    audio.volume = vol
    audioEl = audio

    audio.onended = () => {
      audioEl = null
      opts?.onEnd?.()
      resolve()
    }
    audio.onerror = () => {
      // Fallback to browser SpeechSynthesis with boundary events
      audioEl = null
      speechSynthesis.cancel()
      fallbackTTS(text, opts).then(resolve)
    }

    audio.play().catch(() => {
      audioEl = null
      resolve()
    })
  })
}

export async function speakWord(audioURL: string | undefined | null, text: string, opts?: SpeakOptions): Promise<void> {
  stopCurrent()

  if (audioURL) {
    const url = getWordAudioURL(audioURL)
    const vol = getVolume()

    return new Promise((resolve) => {
      const audio = new Audio(freshAudioURL(url))
      audio.volume = vol
      audioEl = audio

      audio.onended = () => {
        audioEl = null
        opts?.onEnd?.()
        resolve()
      }
      audio.onerror = () => {
        audioEl = null
        fallbackTTS(text, opts).then(resolve)
      }

      audio.play().catch(() => {
        audioEl = null
        fallbackTTS(text, opts).then(resolve)
      })
    })
  }

  return fallbackTTS(text, opts)
}
