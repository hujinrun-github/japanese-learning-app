import { describe, expect, it } from 'vitest'
import {
  WAV_SAMPLE_RATE,
  downmixToMono,
  encodeWAVBlob,
  encodeWAVBytes,
  floatToPCM16,
  pickRecorderMimeType,
  wavDurationMS,
} from '../wav'

function dataView(bytes: Uint8Array): DataView {
  return new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength)
}

function ascii(bytes: Uint8Array, start: number, length: number): string {
  return String.fromCharCode(...bytes.slice(start, start + length))
}

describe('floatToPCM16', () => {
  it.each([
    { name: 'silence stays zero', input: 0, want: 0 },
    { name: 'positive full scale clamps to 32767', input: 1, want: 32767 },
    { name: 'negative full scale is -32768', input: -1, want: -32768 },
    { name: 'positive half scale', input: 0.5, want: 16384 },
    { name: 'negative half scale', input: -0.5, want: -16384 },
    { name: 'above full scale is clamped', input: 2, want: 32767 },
    { name: 'below full scale is clamped', input: -2, want: -32768 },
    { name: 'NaN becomes silence', input: Number.NaN, want: 0 },
  ])('$name', ({ input, want }) => {
    expect(floatToPCM16(input)).toBe(want)
  })
})

describe('encodeWAVBytes', () => {
  const samples = [0, 0.5, -0.5, 1, -1, 2, -2]

  it('writes a canonical 44 byte mono PCM header', () => {
    const wav = encodeWAVBytes(samples, WAV_SAMPLE_RATE)
    const view = dataView(wav)

    expect(ascii(wav, 0, 4)).toBe('RIFF')
    expect(view.getUint32(4, true)).toBe(36 + samples.length * 2)
    expect(ascii(wav, 8, 4)).toBe('WAVE')
    expect(ascii(wav, 12, 4)).toBe('fmt ')
    expect(view.getUint32(16, true)).toBe(16)
    expect(view.getUint16(20, true)).toBe(1)
    expect(view.getUint16(22, true)).toBe(1)
    expect(view.getUint32(24, true)).toBe(WAV_SAMPLE_RATE)
    expect(view.getUint32(28, true)).toBe(WAV_SAMPLE_RATE * 2)
    expect(view.getUint16(32, true)).toBe(2)
    expect(view.getUint16(34, true)).toBe(16)
    expect(ascii(wav, 36, 4)).toBe('data')
    expect(view.getUint32(40, true)).toBe(samples.length * 2)
    expect(wav.byteLength).toBe(44 + samples.length * 2)
  })

  it('writes little-endian int16 samples', () => {
    const wav = encodeWAVBytes(samples, WAV_SAMPLE_RATE)
    const view = dataView(wav)
    const got = samples.map((_, i) => view.getInt16(44 + i * 2, true))
    expect(got).toEqual([0, 16384, -16384, 32767, -32768, 32767, -32768])
  })

  it.each([
    { name: '16 kHz', rate: 16000 },
    { name: '8 kHz reference rate', rate: 8000 },
    { name: '44.1 kHz', rate: 44100 },
  ])('honours the requested sample rate: $name', ({ rate }) => {
    const wav = encodeWAVBytes([0], rate)
    expect(dataView(wav).getUint32(24, true)).toBe(rate)
    expect(dataView(wav).getUint32(28, true)).toBe(rate * 2)
  })

  it('produces a header-only file for no samples', () => {
    const wav = encodeWAVBytes([], WAV_SAMPLE_RATE)
    expect(wav.byteLength).toBe(44)
    expect(dataView(wav).getUint32(40, true)).toBe(0)
  })
})

describe('encodeWAVBlob', () => {
  it('wraps the same bytes in an audio/wav blob', async () => {
    const samples = [0, 0.25, -0.25]
    const blob = encodeWAVBlob(samples, WAV_SAMPLE_RATE)
    expect(blob.type).toBe('audio/wav')
    expect(blob.size).toBe(44 + samples.length * 2)
    const bytes = new Uint8Array(await blob.arrayBuffer())
    expect(Array.from(bytes)).toEqual(Array.from(encodeWAVBytes(samples, WAV_SAMPLE_RATE)))
  })
})

describe('downmixToMono', () => {
  it.each([
    {
      name: 'single channel is passed through',
      channels: [[0, 0.5, -1]],
      want: [0, 0.5, -1],
    },
    {
      name: 'stereo is averaged',
      channels: [[1, 3], [3, 5]],
      want: [2, 4],
    },
    {
      name: 'three channels are averaged',
      channels: [[1, 1], [2, 2], [3, 3]],
      want: [2, 2],
    },
    {
      name: 'no channels yields no samples',
      channels: [],
      want: [],
    },
    {
      name: 'uneven channels use the shortest length',
      channels: [[1, 1, 1], [3, 3]],
      want: [2, 2],
    },
  ])('$name', ({ channels, want }) => {
    const got = downmixToMono(channels.map((channel) => Float32Array.from(channel)))
    expect(Array.from(got)).toEqual(want)
  })
})

describe('pickRecorderMimeType', () => {
  it.each([
    { name: 'webm preferred when available', supported: (t: string) => t === 'audio/webm', want: 'audio/webm' },
    { name: 'mp4 fallback for Safari', supported: (t: string) => t === 'audio/mp4', want: 'audio/mp4' },
    { name: 'ogg last resort', supported: (t: string) => t === 'audio/ogg', want: 'audio/ogg' },
    { name: 'null when no container can be recorded', supported: () => false, want: null },
  ])('$name', ({ supported, want }) => {
    expect(pickRecorderMimeType(supported)).toBe(want)
  })
})

describe('wavDurationMS', () => {
  it.each([
    { name: 'three seconds at 16 kHz', count: 48000, rate: 16000, want: 3000 },
    { name: 'one second', count: 16000, rate: 16000, want: 1000 },
    { name: 'empty', count: 0, rate: 16000, want: 0 },
  ])('$name', ({ count, rate, want }) => {
    expect(wavDurationMS(count, rate)).toBe(want)
  })
})
