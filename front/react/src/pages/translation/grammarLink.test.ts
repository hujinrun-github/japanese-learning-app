import { describe, expect, it } from 'vitest'
import { grammarSearchPath } from './grammarLink'

describe('grammarSearchPath', () => {
  it.each([
    ['plain annotation', 'ていた', '/grammar?q=%E3%81%A6%E3%81%84%E3%81%9F'],
    ['wave dash annotation', '〜から', '/grammar?q=%E3%80%9C%E3%81%8B%E3%82%89'],
    ['surrounding whitespace is trimmed', '  〜ている  ', '/grammar?q=%E3%80%9C%E3%81%A6%E3%81%84%E3%82%8B'],
    ['reserved characters are encoded', 'A&B?C/D', '/grammar?q=A%26B%3FC%2FD'],
    ['empty annotation falls back to the grammar list', '', '/grammar'],
    ['whitespace-only annotation falls back to the grammar list', '   ', '/grammar'],
  ])('%s', (_name, point, want) => {
    expect(grammarSearchPath(point)).toBe(want)
  })
})
