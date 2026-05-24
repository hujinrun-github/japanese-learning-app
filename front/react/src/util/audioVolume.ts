const STORAGE_KEY = 'app_audio_volume'

let listeners: Array<(v: number) => void> = []

export function getVolume(): number {
  const raw = localStorage.getItem(STORAGE_KEY)
  if (raw === null) return 1
  const v = parseFloat(raw)
  return Number.isFinite(v) ? Math.max(0, Math.min(1, v)) : 1
}

export function setVolume(v: number): void {
  const clamped = Math.max(0, Math.min(1, v))
  localStorage.setItem(STORAGE_KEY, String(clamped))
  listeners.forEach((fn) => fn(clamped))
}

export function onVolumeChange(fn: (v: number) => void): () => void {
  listeners.push(fn)
  return () => {
    listeners = listeners.filter((f) => f !== fn)
  }
}
