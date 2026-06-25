import { describe, expect, it } from 'vitest'
import type { Sentence } from '@/types/api'
import { getCurrentSentenceIndex } from './currentSentence'

const sentences: Sentence[] = [
  { index: 0, tokens: [], chinese: 'one', start_ms: 1000, end_ms: 2000 },
  { index: 1, tokens: [], chinese: 'two', start_ms: 3000, end_ms: 4000 },
  { index: 2, tokens: [], chinese: 'three', start_ms: 4000, end_ms: 5000 },
]

describe('getCurrentSentenceIndex', () => {
  it('returns null for empty list', () => {
    expect(getCurrentSentenceIndex([], 1000)).toBeNull()
  })

  it('uses first sentence before first start', () => {
    expect(getCurrentSentenceIndex(sentences, 0)).toBe(0)
  })

  it('keeps previous sentence in gaps', () => {
    expect(getCurrentSentenceIndex(sentences, 2500)).toBe(0)
  })

  it('uses next sentence at exact start boundary', () => {
    expect(getCurrentSentenceIndex(sentences, 3000)).toBe(1)
  })

  it('keeps last sentence after media ends', () => {
    expect(getCurrentSentenceIndex(sentences, 6000)).toBe(2)
  })

  it('skips invalid sentence timings', () => {
    const invalidLeading: Sentence[] = [
      { index: 9, tokens: [], chinese: 'bad', start_ms: 0, end_ms: 0 },
      { index: 10, tokens: [], chinese: 'ok', start_ms: 1000, end_ms: 2000 },
    ]

    expect(getCurrentSentenceIndex(invalidLeading, 500)).toBe(10)
  })
})
