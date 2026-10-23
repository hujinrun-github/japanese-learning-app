import { describe, expect, it } from 'vitest'
import { evaluateDictationAnswer, normalizeDictationAnswer, wordRequiresKanji } from './dictationAnswer'

describe('dictation answer evaluation', () => {
  const tests = [
    {
      name: 'accepts correct kanji and reading',
      word: { kanji_form: '勉強', reading: 'べんきょう' },
      spelling: '勉強',
      reading: 'べんきょう',
      want: { spellingCorrect: true, readingCorrect: true, allCorrect: true },
    },
    {
      name: 'normalizes width and surrounding whitespace',
      word: { kanji_form: 'アクセサリー', reading: 'アクセサリー' },
      spelling: '  ｱｸｾｻﾘｰ  ',
      reading: '',
      want: { spellingCorrect: true, readingCorrect: true, allCorrect: true },
    },
    {
      name: 'reports an incorrect written form separately',
      word: { kanji_form: '帰る', reading: 'かえる' },
      spelling: '変える',
      reading: 'かえる',
      want: { spellingCorrect: false, readingCorrect: true, allCorrect: false },
    },
    {
      name: 'reports an incorrect reading separately',
      word: { kanji_form: '学校', reading: 'がっこう' },
      spelling: '学校',
      reading: 'がこう',
      want: { spellingCorrect: true, readingCorrect: false, allCorrect: false },
    },
    {
      name: 'requires only the official spelling for a kana-only word',
      word: { kanji_form: 'だいぶ', reading: 'だいぶ' },
      spelling: 'だいぶ',
      reading: '',
      want: { spellingCorrect: true, readingCorrect: true, allCorrect: true },
    },
    {
      name: 'keeps hiragana and katakana distinct',
      word: { kanji_form: 'テレビ', reading: 'テレビ' },
      spelling: 'てれび',
      reading: '',
      want: { spellingCorrect: false, readingCorrect: true, allCorrect: false },
    },
  ]

  for (const tt of tests) {
    it(tt.name, () => {
      expect(evaluateDictationAnswer(tt.word, tt.spelling, tt.reading)).toMatchObject(tt.want)
    })
  }
})

describe('dictation normalization', () => {
  it('normalizes Unicode width and trims the edges', () => {
    expect(normalizeDictationAnswer('  ｶﾀｶﾅ  ')).toBe('カタカナ')
  })

  it('detects words containing kanji', () => {
    expect(wordRequiresKanji('お風呂')).toBe(true)
    expect(wordRequiresKanji('アクセサリー')).toBe(false)
  })
})
