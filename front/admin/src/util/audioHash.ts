export async function sha256Hex(text: string): Promise<string> {
  const data = new TextEncoder().encode(text)
  const hash = await crypto.subtle.digest('SHA-256', data)
  return Array.from(new Uint8Array(hash))
    .map(b => b.toString(16).padStart(2, '0'))
    .join('')
    .slice(0, 16)
}

export function exampleAudioUrl(hash: string): string {
  return `/audio/examples/${hash}.wav`
}

export function wordAudioUrl(filename: string): string {
  return `/audio/words/${filename}`
}

// The words API already returns a normalized path (e.g. /audio/words/<hash>.wav
// or /api/v1/audio/<id>/stream). Older rows may still hold a bare filename.
// Prefixing every value with /audio/words/ produced a doubled path, which 404'd
// and silently fell back to browser TTS in AudioRegenButton.
export function wordAudioPlaybackURL(value: string | null | undefined): string | undefined {
  const raw = (value ?? '').trim()
  if (!raw) return undefined
  if (raw.startsWith('/') || /^https?:\/\//i.test(raw)) return raw
  return wordAudioUrl(raw)
}
