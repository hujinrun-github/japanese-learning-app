// WAV helpers for browser recordings. The backend scoring endpoint only accepts
// 16-bit PCM WAV, so MediaRecorder output must be decoded and re-encoded here.

export const WAV_SAMPLE_RATE = 16000

const WAV_HEADER_BYTES = 44
const RECORDER_MIME_CANDIDATES = ['audio/webm', 'audio/mp4', 'audio/ogg'] as const

// Convert one float sample in [-1, 1] to a signed 16-bit PCM value.
// Out-of-range and non-finite values are clamped instead of wrapping.
export function floatToPCM16(value: number): number {
  if (!Number.isFinite(value)) return 0
  const clamped = Math.max(-1, Math.min(1, value))
  const quantized = Math.round(clamped * 32768)
  return Math.max(-32768, Math.min(32767, quantized))
}

// Average all channels into one mono track. Uneven channel lengths fall back
// to the shortest one (should not happen for a real AudioBuffer).
export function downmixToMono(channels: ArrayLike<number>[]): Float32Array {
  if (channels.length === 0) return new Float32Array(0)
  const length = Math.min(...channels.map((channel) => channel.length))
  const out = new Float32Array(length)
  for (let i = 0; i < length; i++) {
    let sum = 0
    for (const channel of channels) sum += channel[i]
    out[i] = sum / channels.length
  }
  return out
}

// Build a canonical 44-byte-header mono 16-bit PCM WAV. The explicit
// ArrayBuffer type parameter keeps the result assignable to BlobPart.
export function encodeWAVBytes(samples: ArrayLike<number>, sampleRate: number): Uint8Array<ArrayBuffer> {
  if (!Number.isInteger(sampleRate) || sampleRate <= 0) {
    throw new Error(`invalid WAV sample rate: ${sampleRate}`)
  }
  const dataSize = samples.length * 2
  const bytes = new Uint8Array(WAV_HEADER_BYTES + dataSize)
  const view = new DataView(bytes.buffer)

  const writeASCII = (offset: number, text: string) => {
    for (let i = 0; i < text.length; i++) bytes[offset + i] = text.charCodeAt(i)
  }

  writeASCII(0, 'RIFF')
  view.setUint32(4, 36 + dataSize, true)
  writeASCII(8, 'WAVE')
  writeASCII(12, 'fmt ')
  view.setUint32(16, 16, true) // fmt chunk size
  view.setUint16(20, 1, true) // PCM
  view.setUint16(22, 1, true) // mono
  view.setUint32(24, sampleRate, true)
  view.setUint32(28, sampleRate * 2, true) // byte rate
  view.setUint16(32, 2, true) // block align
  view.setUint16(34, 16, true) // bits per sample
  writeASCII(36, 'data')
  view.setUint32(40, dataSize, true)

  for (let i = 0; i < samples.length; i++) {
    view.setInt16(WAV_HEADER_BYTES + i * 2, floatToPCM16(samples[i]), true)
  }
  return bytes
}

export function encodeWAVBlob(samples: ArrayLike<number>, sampleRate: number): Blob {
  return new Blob([encodeWAVBytes(samples, sampleRate)], { type: 'audio/wav' })
}

// Prefer webm, fall back to mp4 for Safari, then ogg. Returning null lets the
// caller show a clear error instead of silently uploading an unsupported blob.
export function pickRecorderMimeType(isTypeSupported: (type: string) => boolean): string | null {
  for (const candidate of RECORDER_MIME_CANDIDATES) {
    if (isTypeSupported(candidate)) return candidate
  }
  return null
}

export function wavDurationMS(sampleCount: number, sampleRate: number): number {
  if (sampleRate <= 0) return 0
  return Math.round((sampleCount * 1000) / sampleRate)
}
