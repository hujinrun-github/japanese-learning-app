import { describe, expect, it } from 'vitest'
import { grammarSearchPath, translationRoundPath } from './grammarLink'

describe('grammarSearchPath', () => {
  it.each([
    ['plain annotation', 'ていた', '/translation/practice/grammar?q=%E3%81%A6%E3%81%84%E3%81%9F'],
    ['wave dash annotation', '〜から', '/translation/practice/grammar?q=%E3%80%9C%E3%81%8B%E3%82%89'],
    ['surrounding whitespace is trimmed', '  〜ている  ', '/translation/practice/grammar?q=%E3%80%9C%E3%81%A6%E3%81%84%E3%82%8B'],
    ['reserved characters are encoded', 'A&B?C/D', '/translation/practice/grammar?q=A%26B%3FC%2FD'],
    ['empty annotation falls back to the grammar list', '', '/translation/practice/grammar'],
    ['whitespace-only annotation falls back to the grammar list', '   ', '/translation/practice/grammar'],
  ])('%s', (_name, point, want) => {
    expect(grammarSearchPath(point)).toBe(want)
  })

  it('carries the source and review mode into grammar without keeping stale search filters', () => {
    expect(grammarSearchPath('のに', new URLSearchParams('mode=free&source_id=7&review=true&q=stale&level=N5')))
      .toBe('/translation/practice/grammar?mode=free&source_id=7&review=true&q=%E3%81%AE%E3%81%AB')
  })
  it('keeps the mistake-book question when opening and returning from grammar', () => {
    expect(grammarSearchPath('のに', new URLSearchParams('sentence_id=23')))
      .toBe('/translation/practice/grammar?sentence_id=23&q=%E3%81%AE%E3%81%AB')
    expect(translationRoundPath(new URLSearchParams('sentence_id=23&q=のに')))
      .toBe('/translation/practice?sentence_id=23')
  })

})
