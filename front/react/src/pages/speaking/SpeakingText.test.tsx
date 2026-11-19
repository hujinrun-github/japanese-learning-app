import { describe, expect, it } from 'vitest'
import { renderToStaticMarkup } from 'react-dom/server'
import i18next from 'i18next'
import { initReactI18next } from 'react-i18next'
import { SpeakingText, splitSpeakingTurns } from './SpeakingText'
import { toFuriganaTokens } from '@/util/furigana'

void i18next.use(initReactI18next).init({ lng: 'zh', resources: {} })

describe('splitSpeakingTurns', () => {
  it.each([
    {
      name: 'preserves numbered textbook sentences and dialogue roles across newlines',
      text: '1. 部屋に机といすがあります。\n2. 机の上に猫がいます。\n甲：その箱の中に何がありますか。\n乙：時計と眼鏡があります。',
      expected: ['1. 部屋に机といすがあります。', '2. 机の上に猫がいます。', '甲：その箱の中に何がありますか。', '乙：時計と眼鏡があります。'],
    },
    {
      name: 'keeps a speaker turn together across several sentences',
      text: '小野：ええ。JRと地下鉄の駅があります。JRの駅はここです。\n李：地下鉄の駅はここですね。',
      expected: ['小野：ええ。JRと地下鉄の駅があります。JRの駅はここです。', '李：地下鉄の駅はここですね。'],
    },
    {
      name: 'keeps short replies and follow-up sentences in the same turn',
      text: '甲：わたしは李です。小野さんですか。\n乙：はい、そうです。小野です。',
      expected: ['甲：わたしは李です。小野さんですか。', '乙：はい、そうです。小野です。'],
    },
    {
      name: 'preserves quotes and punctuation inside a turn',
      text: '甲：「本当ですか？」と聞きました。どうですか？\n乙：はい！いいですよ。',
      expected: ['甲：「本当ですか？」と聞きました。どうですか？', '乙：はい！いいですよ。'],
    },
    {
      name: 'preserves an unlabelled paragraph as one utterance',
      text: 'すみません。駅はどこですか。',
      expected: ['すみません。駅はどこですか。'],
    },
    {
      name: 'honors existing lines without punctuation and ignores empty lines',
      text: '  李：あのう\r\n\r\n　小野：はい\n甲：そうです。\n ',
      expected: ['李：あのう', '小野：はい', '甲：そうです。'],
    },
    { name: 'handles empty text', text: '\n　 ', expected: [] },
  ])('$name', ({ text, expected }) => {
    const lines = splitSpeakingTurns(text)
    expect(lines).toEqual(expected)
    // Formatting must not drop spoken text, labels, numbers, or punctuation.
    expect(lines.join('').replace(/\s/g, '')).toBe(text.replace(/\s/g, ''))
  })
})

describe('SpeakingText', () => {
  it.each([false, true])('renders separate complete rows with annotations=%s', async (annotated) => {
    const lines = ['1. 部屋に机といすがあります。', '2. 机の上に猫がいます。', '甲：わたしは李です。小野さんですか。', '乙：はい、そうです。小野です。']
    const annotations = annotated ? await Promise.all(lines.map(toFuriganaTokens)) : []
    const html = renderToStaticMarkup(<SpeakingText lines={lines} annotations={annotations} />)
    const rows = Array.from(html.matchAll(/<div class="[^"]*sentenceLine[^"]*"[^>]*>(.*?)<\/div>/g), (match) =>
      match[1].replace(/<rt>.*?<\/rt>/g, '').replace(/<[^>]*>/g, ''),
    )
    expect(rows).toEqual(lines)
    expect(html.includes('<ruby')).toBe(annotated)
  })
})
