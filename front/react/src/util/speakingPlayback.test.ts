import { describe, expect, it } from 'vitest'
import { estimateSentenceCues, sentenceAtTime, splitSpeakingSentences } from './speakingPlayback'

describe('splitSpeakingSentences', () => {
  it.each([
    { text: '甲：はい。駅はここです。\n乙：ありがとうございます！', want: ['甲：はい。', '駅はここです。', '乙：ありがとうございます！'] },
    { text: '1. 部屋に机があります。\n2. 猫がいます。', want: ['1. 部屋に机があります。', '2. 猫がいます。'] },
    { text: '  😀：はい！　いいですか？\r\n\n乙：はい', want: ['😀：はい！', 'いいですか？', '乙：はい'] },
    { text: '甲：「本当ですか？」と聞きました。どうですか？', want: ['甲：「本当ですか？」と聞きました。', 'どうですか？'] },
    { text: 'あのう\nはい', want: ['あのう', 'はい'] },
    { text: '\n　 ', want: [] },
  ])('preserves sentences and codepoint offsets: $text', ({ text, want }) => {
    const sentences = splitSpeakingSentences(text)
    expect(sentences.map(sentence => sentence.text)).toEqual(want)
    for (const sentence of sentences) {
      expect(Array.from(text).slice(sentence.start, sentence.end).join('')).toBe(sentence.text)
    }
  })
})

describe('estimated sentence timing', () => {
  it('uses audible text lengths and excludes dialogue labels and numbering', () => {
    const cues = estimateSentenceCues(['1. 甲：はい。', '長い文です。'], 9)
    expect(cues).toEqual([{ index: 0, start: 0, end: 18 / 7 }, { index: 1, start: 18 / 7, end: 9 }])
    expect(sentenceAtTime(cues, 0)).toBe(0)
    expect(sentenceAtTime(cues, cues[0].end)).toBe(1)
    expect(sentenceAtTime(cues, 9)).toBeNull()
    expect(sentenceAtTime(cues, -1)).toBeNull()
  })

  it.each([0, -1, Infinity, NaN])('does not invent timing for invalid duration %s', duration => {
    expect(estimateSentenceCues(['はい。'], duration)).toEqual([])
  })

  it('returns no active sentence when there are no cues', () => {
    expect(estimateSentenceCues([], 4)).toEqual([])
    expect(sentenceAtTime([], 1)).toBeNull()
  })
})
