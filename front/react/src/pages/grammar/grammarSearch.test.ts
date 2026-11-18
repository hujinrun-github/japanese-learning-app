import { describe, expect, it } from 'vitest'
import type { GrammarSearchResult } from '@/types/api'
import { exactMatchPath, grammarSearchApiPath } from './grammarSearch'

function searchResult(exactMatchID: number): GrammarSearchResult {
  return { query: 'ていた', exact_match_id: exactMatchID, points: [] }
}

describe('exactMatchPath', () => {
  it.each([
    ['exact hit navigates straight to the detail page', searchResult(10), '/grammar/10'],
    ['no exact hit stays on the grammar list', searchResult(0), null],
    ['negative id is ignored', searchResult(-1), null],
    ['no result yet stays on the grammar list', null, null],
  ])('%s', (_name, result, want) => {
    expect(exactMatchPath(result)).toBe(want)
  })
})

describe('grammarSearchApiPath', () => {
  it.each([
    ['plain annotation', 'から', '/api/v1/grammar/search?q=%E3%81%8B%E3%82%89'],
    ['whitespace is trimmed', '  〜ている ', '/api/v1/grammar/search?q=%E3%80%9C%E3%81%A6%E3%81%84%E3%82%8B'],
    ['reserved characters are encoded', 'A&B', '/api/v1/grammar/search?q=A%26B'],
  ])('%s', (_name, query, want) => {
    expect(grammarSearchApiPath(query)).toBe(want)
  })
})
