import { getVolume } from './audioVolume'

let audioEl: HTMLAudioElement | null = null

function stopCurrent() {
  if (audioEl) {
    audioEl.pause()
    audioEl = null
  }
  speechSynthesis.cancel()
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

export interface SpeakOptions {
  onBoundary?: (charIndex: number) => void
  onEnd?: () => void
}

export async function speakExample(text: string, opts?: SpeakOptions): Promise<void> {
  stopCurrent()

  const hash = await sha256Hex(text)
  const url = getExampleAudioURL(hash)
  const vol = getVolume()

  return new Promise((resolve) => {
    const audio = new Audio(url)
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
    }

    audio.play().catch(() => {
      audioEl = null
      resolve()
    })
  })
}
