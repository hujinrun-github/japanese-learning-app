import { describe, it, expect, vi } from 'vitest'

// Mock kuromojin before importing the module under test
vi.mock('kuromojin', () => ({
  tokenize: vi.fn(),
}))

import { tokenize } from 'kuromojin'
import { toFuriganaTokens, charIndexToTokenIndex } from '../furigana'
import type { FuriganaToken } from '@/types/api'

function mockTokens(forms: { surface: string; reading: string }[]) {
  return forms.map(f => ({
    surface_form: f.surface,
    reading: f.reading,
    pos: '名詞',
    basic_form: f.surface,
  }))
}

describe('toFuriganaTokens', () => {
  it('returns tokens for kanji with reading', async () => {
    vi.mocked(tokenize).mockResolvedValue(mockTokens([
      { surface: '今日', reading: 'キョウ' },
    ]))

    const tokens = await toFuriganaTokens('今日')
    expect(tokens).toEqual([
      { surface: '今日', reading: 'きょう' },
    ])
  })

  it('returns tokens for mixed kana and kanji', async () => {
    vi.mocked(tokenize).mockResolvedValue(mockTokens([
      { surface: '日本語', reading: 'ニホンゴ' },
      { surface: 'を', reading: 'ヲ' },
      { surface: '勉強', reading: 'ベンキョウ' },
      { surface: 'する', reading: 'スル' },
    ]))

    const tokens = await toFuriganaTokens('日本語を勉強する')
    expect(tokens).toEqual([
      { surface: '日本語', reading: 'にほんご' },
      { surface: 'を', reading: 'を' },
      { surface: '勉強', reading: 'べんきょう' },
      { surface: 'する', reading: 'する' },
    ])
  })

  it('uses empty reading when token has no reading', async () => {
    vi.mocked(tokenize).mockResolvedValue(mockTokens([
      { surface: '、', reading: '' },
    ]))

    const tokens = await toFuriganaTokens('、')
    expect(tokens).toEqual([
      { surface: '、', reading: '' },
    ])
  })

  it('returns empty array for empty string', async () => {
    vi.mocked(tokenize).mockResolvedValue([])

    const tokens = await toFuriganaTokens('')
    expect(tokens).toEqual([])
  })

  it('converts katakana readings to hiragana', async () => {
    vi.mocked(tokenize).mockResolvedValue(mockTokens([
      { surface: '私', reading: 'ワタシ' },
      { surface: 'コーヒー', reading: 'コーヒー' },
    ]))

    const tokens = await toFuriganaTokens('私コーヒー')
    expect(tokens[0].reading).toBe('わたし')
    // コーヒー as reading stays katakana but since it's the reading field
    // and our katakanaToHiragana only converts within 30A1-30F6 range
    expect(tokens[1].reading).toBe('こーひー')
  })
})

describe('charIndexToTokenIndex', () => {
  const tokens: FuriganaToken[] = [
    { surface: '今日', reading: 'きょう' },
    { surface: 'は', reading: 'は' },
    { surface: '良い', reading: 'よい' },
    { surface: '天気', reading: 'てんき' },
    { surface: 'です', reading: 'です' },
  ]
  // surface lengths: 2 + 1 + 2 + 2 + 2 = 9 chars
  // token boundaries: [0-1], [2], [3-4], [5-6], [7-8]

  it('returns index 0 for charIndex 0', () => {
    expect(charIndexToTokenIndex(0, tokens)).toBe(0)
  })

  it('returns correct token index at start of each token', () => {
    expect(charIndexToTokenIndex(2, tokens)).toBe(1) // 'は' starts at index 2
    expect(charIndexToTokenIndex(3, tokens)).toBe(2) // '良い' starts at index 3
  })

  it('returns same token for charIndex within a multi-char token', () => {
    expect(charIndexToTokenIndex(1, tokens)).toBe(0) // second char of '今日'
    expect(charIndexToTokenIndex(6, tokens)).toBe(3) // second char of '天気'
  })

  it('returns last index for charIndex beyond text length', () => {
    expect(charIndexToTokenIndex(100, tokens)).toBe(4)
  })

  it('returns 0 for single token', () => {
    const single: FuriganaToken[] = [{ surface: '私', reading: 'わたし' }]
    expect(charIndexToTokenIndex(0, single)).toBe(0)
    expect(charIndexToTokenIndex(5, single)).toBe(0)
  })

  it('handles empty token step-through', () => {
    const items: FuriganaToken[] = [
      { surface: 'a', reading: '' },
      { surface: 'bc', reading: '' },
      { surface: 'd', reading: '' },
    ]
    expect(charIndexToTokenIndex(0, items)).toBe(0) // 'a'
    expect(charIndexToTokenIndex(1, items)).toBe(1) // 'b' (start of 'bc')
    expect(charIndexToTokenIndex(2, items)).toBe(1) // 'c' (still in 'bc')
    expect(charIndexToTokenIndex(3, items)).toBe(2) // 'd'
  })
})
