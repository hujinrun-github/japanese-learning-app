import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import { SpeakingText, splitSpeakingSentences } from './SpeakingText'
import { toFuriganaTokens } from '@/util/furigana'

describe('splitSpeakingSentences', () => {
  it.each([
    {
      name: 'preserves numbered textbook sentences and dialogue roles across newlines',
      text: '1. 部屋に机といすがあります。\n2. 机の上に猫がいます。\n甲：その箱の中に何がありますか。\n乙：時計と眼鏡があります。',
      expected: ['1. 部屋に机といすがあります。', '2. 机の上に猫がいます。', '甲：その箱の中に何がありますか。', '乙：時計と眼鏡があります。'],
    },
    {
      name: 'splits consecutive sentences even before a number or Latin word',
      text: '1. 森さんは学生です。 2. 李さんはJC企画の社員です。JRの駅はここです。',
      expected: ['1. 森さんは学生です。', '2. 李さんはJC企画の社員です。', 'JRの駅はここです。'],
    },
    {
      name: 'keeps a short reply on its own line',
      text: '小野：ええ。JRと地下鉄の駅があります。李：そうですか。',
      expected: ['小野：ええ。', 'JRと地下鉄の駅があります。', '李：そうですか。'],
    },
    {
      name: 'keeps closing quotes and punctuation with the preceding sentence',
      text: '「本当ですか？」「はい！」次は？ いいですか? はい!',
      expected: ['「本当ですか？」', '「はい！」', '次は？', 'いいですか?', 'はい!'],
    },
    {
      name: 'honors existing lines without punctuation and ignores empty lines',
      text: '  李：あのう\r\n\r\n　小野：はい\n甲：そうです。\n ',
      expected: ['李：あのう', '小野：はい', '甲：そうです。'],
    },
    { name: 'handles empty text', text: '\n　 ', expected: [] },
  ])('$name', ({ text, expected }) => {
    const sentences = splitSpeakingSentences(text)
    expect(sentences).toEqual(expected)
    // Formatting must not drop spoken text, labels, numbers, or punctuation.
    expect(sentences.join('').replace(/\s/g, '')).toBe(text.replace(/\s/g, ''))
  })
})

describe('SpeakingText', () => {
  it.each([false, true])('renders separate complete rows with annotations=%s', async (annotated) => {
    const sentences = ['1. 部屋に机といすがあります。', '2. 机の上に猫がいます。', '甲：部屋にだれがいますか。', '乙：だれもいません。']
    const annotations = annotated ? await Promise.all(sentences.map(toFuriganaTokens)) : []
    const html = renderToStaticMarkup(<SpeakingText sentences={sentences} annotations={annotations} />)
    const rows = Array.from(html.matchAll(/<div class="[^"]*sentenceLine[^"]*">(.*?)<\/div>/g), (match) =>
      match[1].replace(/<rt>.*?<\/rt>/g, '').replace(/<[^>]*>/g, ''),
    )
    expect(rows).toEqual(sentences)
    expect(html.includes('<ruby')).toBe(annotated)
  })
})
