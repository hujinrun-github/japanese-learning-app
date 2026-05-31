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
