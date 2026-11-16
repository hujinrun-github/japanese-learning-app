import { describe, expect, it } from 'vitest'
import type { Sentence } from '@/types/api'
import { indexLessonText, splitHighlights, tapSelection } from './lessonSelection'

describe('lesson text positions', () => {
  it('counts Unicode characters across tokens and sentences, without including readings', () => {
    const sentences: Sentence[] = [
      { index: 0, tokens: [{ surface: '𠮷野', reading: 'よしの' }, { surface: 'です。', reading: '' }], chinese: '吉野', start_ms: 0, end_ms: 0 },
      { index: 1, tokens: [{ surface: '散歩', reading: 'さんぽ' }], chinese: '散步', start_ms: 0, end_ms: 0 },
    ]
    expect(indexLessonText(sentences).map(({ tokens }) => tokens.map(({ start }) => start))).toEqual([[0, 2], [5]])
  })

  it.each([
    { name: 'partial ruby base', surface: '𠮷野', start: 0, ranges: [{start: 1, end: 2}], expected: [['𠮷', []], ['野', [{start: 1, end: 2}]]] },
    { name: 'overlapping annotations', surface: 'abc', start: 2, ranges: [{start: 1, end: 4}, {start: 3, end: 5}], expected: [['a', [{start: 1, end: 4}]], ['b', [{start: 1, end: 4}, {start: 3, end: 5}]], ['c', [{start: 3, end: 5}]]] },
  ])('splits $name', ({ surface, start, ranges, expected }) => {
    expect(splitHighlights(surface, start, ranges).map(({ text, highlights }) => [text, highlights])).toEqual(expected)
  })
})

describe('tap selection for touch screens', () => {
  const chars = Array.from('朝です。散歩します。')

  it.each([
    {
      name: 'first tap selects the tapped token',
      chars,
      anchor: null,
      target: { start: 0, end: 1 },
      expected: { start: 0, end: 1, quote: '朝' },
      nextAnchor: { start: 0, end: 1 },
    },
    {
      name: 'second tap extends the range forwards',
      chars,
      anchor: { start: 0, end: 1 },
      target: { start: 4, end: 6 },
      expected: { start: 0, end: 6, quote: '朝です。散歩' },
      nextAnchor: null,
    },
    {
      name: 'second tap extends the range backwards',
      chars,
      anchor: { start: 6, end: 10 },
      target: { start: 0, end: 1 },
      expected: { start: 0, end: 10, quote: '朝です。散歩します。' },
      nextAnchor: null,
    },
    {
      name: 'tapping the same token twice keeps a single token',
      chars,
      anchor: { start: 0, end: 1 },
      target: { start: 0, end: 1 },
      expected: { start: 0, end: 1, quote: '朝' },
      nextAnchor: null,
    },
    {
      name: 'counts surrogate pairs as one character',
      chars: Array.from('𠮷野です'),
      anchor: null,
      target: { start: 0, end: 2 },
      expected: { start: 0, end: 2, quote: '𠮷野' },
      nextAnchor: { start: 0, end: 2 },
    },
    {
      name: 'clamps a range past the end of the text',
      chars,
      anchor: null,
      target: { start: 0, end: 99 },
      expected: { start: 0, end: 10, quote: '朝です。散歩します。' },
      nextAnchor: { start: 0, end: 10 },
    },
    {
      name: 'never returns an invalid range for empty text',
      chars: [],
      anchor: null,
      target: { start: 0, end: 1 },
      expected: { start: 0, end: 0, quote: '' },
      nextAnchor: { start: 0, end: 0 },
    },
  ])('$name', ({ chars, anchor, target, expected, nextAnchor }) => {
    expect(tapSelection(chars, anchor, target)).toEqual({ selection: expected, anchor: nextAnchor })
  })
})
