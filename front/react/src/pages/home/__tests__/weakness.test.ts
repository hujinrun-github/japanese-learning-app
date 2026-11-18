import { describe, expect, it } from 'vitest'
import { grammarMistakePath, wordMistakePath } from '../weakness'

describe('weak grammar recommendation destinations', () => {
  it.each([
    { name: 'quiz key targets the grammar detail even when source is a list', item_key: 'grammar:25:7', source_path: '/grammar', expected: '/grammar/25' },
    { name: 'canonical key takes precedence over an inconsistent source', item_key: 'grammar:25:7', source_path: '/grammar/99', expected: '/grammar/25' },
    { name: 'legacy records retain their explicit detail destination', item_key: 'old-quiz', source_path: '/grammar/42', expected: '/grammar/42' },
    { name: 'detail source may include a query or fragment', item_key: '', source_path: '/grammar/42?from=mistakes#quiz', expected: '/grammar/42' },
    { name: 'a list is not a concrete recommendation', item_key: '', source_path: '/grammar', expected: null },
    { name: 'external destinations are rejected', item_key: '', source_path: 'https://example.com/grammar/42', expected: null },
    { name: 'protocol-relative destinations are rejected', item_key: '', source_path: '//example.com/grammar/42', expected: null },
    { name: 'zero IDs are invalid', item_key: 'grammar:0:1', source_path: '', expected: null },
    { name: 'unsafe numeric IDs are rejected', item_key: 'grammar:9007199254740993:1', source_path: '', expected: null },
  ])('$name', ({ item_key, source_path, expected }) => {
    expect(grammarMistakePath({ module: 'grammar', status: 'unmastered', item_key, source_path })).toBe(expected)
  })

  it.each([
    { module: 'word', status: 'unmastered' },
    { module: 'grammar', status: 'mastered' },
  ] as const)('does not recommend $module / $status as a weak grammar point', ({ module, status }) => {
    expect(grammarMistakePath({ module, status, item_key: 'grammar:25:7', source_path: '/grammar/25' })).toBeNull()
  })
})

describe('weak word recommendation destinations', () => {
  it.each([
    { item_key: 'word:7:reading', source_path: '/words/review', expected: '/words/review?word_id=7' },
    { item_key: 'word:82:dictation', source_path: '/words/review', expected: '/words/review?word_id=82&mode=dictation' },
    { item_key: 'word:7:reading', source_path: '/words/review?word_id=99', expected: '/words/review?word_id=7' },
    { item_key: 'legacy', source_path: '/words/review?word_id=7&mode=dictation', expected: '/words/review?word_id=7&mode=dictation' },
    { item_key: '', source_path: '/words/review', expected: null },
    { item_key: 'word:0:reading', source_path: '', expected: null },
    { item_key: 'word:9007199254740993:reading', source_path: '', expected: null },
    { item_key: '', source_path: 'https://example.com/words/review?word_id=7', expected: null },
  ])('resolves $item_key / $source_path', ({ item_key, source_path, expected }) => {
    expect(wordMistakePath({ module: 'word', status: 'unmastered', item_key, source_path })).toBe(expected)
  })

  it.each([
    { module: 'grammar', status: 'unmastered' },
    { module: 'word', status: 'mastered' },
  ] as const)('does not treat $module / $status as a weak word', ({ module, status }) => {
    expect(wordMistakePath({ module, status, item_key: 'word:7:reading', source_path: '/words/review' })).toBeNull()
  })
})
