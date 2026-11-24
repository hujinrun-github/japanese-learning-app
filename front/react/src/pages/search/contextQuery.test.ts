import { describe, expect, it } from 'vitest'
import { selectionKeyword, dictionaryTerms } from './contextQuery'

describe('dictionary lookup from study text', () => {
  it.each([
    ['  「勉強。」 ', '勉強'],
    ['〜てもいい。', '〜てもいい'],
    ['朝\n散歩', '朝散歩'],
    ['　。　', ''],
    ['語'.repeat(101), ''],
  ])('prepares %j for a search', (text, expected) => {
    expect(selectionKeyword(text)).toBe(expected)
  })

  it('keeps visible terms in sentence order and searches inflected terms by dictionary form', () => {
    expect(dictionaryTerms([
      { surface: '日本語' }, { surface: 'を' }, { surface: '勉強' },
      { surface: 'し', base: 'する' }, { surface: '。' }, { surface: '日本語' },
    ])).toEqual([
      { surface: '日本語', keyword: '日本語' }, { surface: 'を', keyword: 'を' },
      { surface: '勉強', keyword: '勉強' }, { surface: 'し', keyword: 'する' },
    ])
  })
})
