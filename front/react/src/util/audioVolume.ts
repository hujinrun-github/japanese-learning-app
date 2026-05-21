const STORAGE_KEY = 'app_audio_volume'

export function getVolume(): number {
  const raw = localStorage.getItem(STORAGE_KEY)
  if (raw === null) return 1
  const v = parseFloat(raw)
  if (isNaN(v) || v < 0 || v > 1) return 1
  return v
}

export function setVolume(volume: number): void {
  const clamped = Math.max(0, Math.min(1, volume))
  localStorage.setItem(STORAGE_KEY, String(clamped))
}
