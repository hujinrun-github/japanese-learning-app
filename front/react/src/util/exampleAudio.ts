import { getVolume } from './audioVolume'

let audioEl: HTMLAudioElement | null = null

// Local files can be replaced after a correction while retaining their text hash.
function freshAudioURL(url: string): string {
  if (!url.startsWith('/audio/')) return url
  const [path, fragment] = url.split('#', 2)
  return `${path}${path.includes('?') ? '&' : '?'}t=${Date.now()}${fragment ? `#${fragment}` : ''}`
}

export function stopCurrent() {
  if (audioEl) {
    audioEl.pause()
    audioEl = null
  }
  speechSynthesis.cancel()
}

// Configured speaking references must be played verbatim. Falling back to a
// different synthesizer would change the audio used as the scoring baseline.
export function playReferenceAudio(url: string): Promise<void> {
  stopCurrent()
  return new Promise((resolve, reject) => {
    const audio = new Audio(freshAudioURL(url))
    audio.volume = getVolume()
    audioEl = audio
    audio.onended = () => { if (audioEl === audio) audioEl = null; resolve() }
    audio.onerror = () => { if (audioEl === audio) audioEl = null; reject(new Error('Reference audio unavailable')) }
    audio.play().catch(reject)
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
