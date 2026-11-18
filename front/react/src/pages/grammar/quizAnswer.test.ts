import { describe, expect, it } from 'vitest'
import type { QuizType } from '@/types/api'
import { answerPartsFor, hasFullAnswer } from './quizAnswer'

describe('answerPartsFor', () => {
  it.each([
    [
      'half-width blank keeps the whole sentence around the answer',
      'fill_blank',
      '電車に乗り遅れ___。（赶不上电车了，真糟糕）',
      { before: '電車に乗り遅れ', after: '。（赶不上电车了，真糟糕）' },
    ],
    [
      'full-width blank is supported',
      'fill_blank',
      '私は日本語＿＿＿好きです。',
      { before: '私は日本語', after: '好きです。' },
    ],
    [
      'two underscores count as a blank',
      'fill_blank',
      '私__学生です。',
      { before: '私', after: '学生です。' },
    ],
    [
      'blank at the beginning',
      'fill_blank',
      '___に行きます。',
      { before: '', after: 'に行きます。' },
    ],
    [
      'no blank falls back to answer only',
      'fill_blank',
      '邀请对方一起吃饭，应说：',
      { before: '', after: '' },
    ],
    [
      'multiple blanks fall back to answer only',
      'fill_blank',
      '「A___B___大きい」（A比B大）正确语序是：',
      { before: '', after: '' },
    ],
    [
      'empty prompt falls back to answer only',
      'fill_blank',
      '',
      { before: '', after: '' },
    ],
    [
      'multi choice shows the answer option only',
      'multi_choice',
      '「宿題をし___」（把作业做完了）表示完成，空白处填：',
      { before: '', after: '' },
    ],
  ] as [string, QuizType, string, { before: string; after: string }][])(
    '%s',
    (_name, type, prompt, want) => {
      expect(answerPartsFor(type, prompt)).toEqual(want)
    },
  )
})

describe('hasFullAnswer', () => {
  it.each([
    ['backfilled explanation already contains the full answer', '正确答案：私は学生です。｜「は」是主题助词', true],
    ['legacy explanation only explains the grammar', '「〜てしまう」表示遗憾或意外发生', false],
    ['empty explanation', '', false],
    ['prefix not at the beginning does not count', '解析：正确答案：私は学生です。', false],
  ])('%s', (_name, explanation, want) => {
    expect(hasFullAnswer(explanation)).toBe(want)
  })
})
