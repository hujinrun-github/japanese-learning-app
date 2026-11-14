import { describe, expect, it } from 'vitest'
import type { Sentence } from '@/types/api'
import { indexLessonText, splitHighlights } from './lessonSelection'

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
