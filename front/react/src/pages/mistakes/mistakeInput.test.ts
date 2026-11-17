import { describe, expect, it } from 'vitest'
import type { QuizItemResult, QuizQuestion, Word } from '@/types/api'
import { buildGrammarMistakes, buildWordDictationMistake, buildWordReadingMistake } from './mistakeInput'

const word: Word = {
  id: 7,
  kanji_form: '猫',
  reading: 'ねこ',
  part_of_speech: '名词',
  meaning: '猫',
  jlpt_level: 'N5',
  examples: [],
  reading_type: '',
  verb_type: '',
}

describe('buildWordReadingMistake', () => {
  it('records the typed reading and the correct one', () => {
    expect(buildWordReadingMistake(word, ' ネコ ')).toEqual({
      module: 'word',
      item_key: 'word:7:reading',
      prompt: '猫',
      user_answer: 'ネコ',
      correct_answer: 'ねこ',
      explanation: '猫',
      source_path: '/words/review',
    })
  })

  it('keeps a non-empty prompt when kanji_form is missing', () => {
    const result = buildWordReadingMistake({ ...word, kanji_form: '' }, 'ねここ')
    expect(result.prompt).toBe('ねこ')
  })
})

describe('buildWordDictationMistake', () => {
  const cases = [
    {
      name: 'kanji words collect both spelling and reading',
      answer: { spelling: '猫 ', reading: ' ネコ ', requiresKanji: true },
      wantUser: '猫 / ネコ',
      wantCorrect: '猫 / ねこ',
    },
    {
      name: 'kana-only words collect spelling only',
      answer: { spelling: 'ねここ', reading: '', requiresKanji: false },
      wantUser: 'ねここ',
      wantCorrect: '猫',
    },
  ]

  for (const tc of cases) {
    it(tc.name, () => {
      const result = buildWordDictationMistake(word, tc.answer)
      expect(result).toEqual({
        module: 'word',
        item_key: 'word:7:dictation',
        prompt: '猫',
        user_answer: tc.wantUser,
        correct_answer: tc.wantCorrect,
        explanation: '猫（ねこ）',
        source_path: '/words/review',
      })
    })
  }

  it('falls back to a non-empty prompt when meaning is missing', () => {
    const result = buildWordDictationMistake({ ...word, meaning: '' }, {
      spelling: 'ねここ', reading: '', requiresKanji: false,
    })
    expect(result.prompt).toBe('猫')
  })
})

const questions: QuizQuestion[] = [
  { id: 1, type: 'fill_blank', prompt: '＿＿＿に行きます。', explanation: '目的地用に' },
  { id: 2, type: 'multi_choice', prompt: 'これは＿＿＿です。', options: ['を', 'に'], explanation: '' },
]

const results: QuizItemResult[] = [
  { question_id: 1, correct: false, user_answer: 'を', expected: 'に', explanation: '目的地用に' },
  { question_id: 2, correct: true, user_answer: 'に', expected: 'に', explanation: '' },
]

describe('buildGrammarMistakes', () => {
  it('keeps only wrong answers and resolves the question prompt', () => {
    expect(buildGrammarMistakes(3, questions, results)).toEqual([{
      module: 'grammar',
      item_key: 'grammar:3:1',
      prompt: '＿＿＿に行きます。',
      user_answer: 'を',
      correct_answer: 'に',
      explanation: '目的地用に',
      source_path: '/grammar/3',
    }])
  })

  for (const tc of [
    { name: 'all correct yields no mistakes', items: results.slice(1), want: 0 },
    { name: 'empty results yield no mistakes', items: [], want: 0 },
  ]) {
    it(tc.name, () => {
      expect(buildGrammarMistakes(3, questions, tc.items)).toHaveLength(tc.want)
    })
  }

  it('keeps a non-empty prompt when the question is missing', () => {
    const mistakes = buildGrammarMistakes(3, [], results.slice(0, 1))
    expect(mistakes).toHaveLength(1)
    expect(mistakes[0].prompt).not.toBe('')
  })
})
