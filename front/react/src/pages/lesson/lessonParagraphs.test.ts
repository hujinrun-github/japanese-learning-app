import { describe, expect, it } from 'vitest'
import type { FuriganaToken, Sentence } from '@/types/api'
import { indexLessonText } from './lessonSelection'
import { groupSentencesByParagraph, toArticleParagraphs } from './lessonParagraphs'

function token(surface: string, reading = ''): FuriganaToken {
  return { surface, reading }
}

function sentence(index: number, tokens: FuriganaToken[], chinese: string, paragraph?: number): Sentence {
  return {
    index,
    tokens,
    chinese,
    start_ms: index * 3000,
    end_ms: (index + 1) * 3000,
    ...(paragraph === undefined ? {} : { paragraph }),
  }
}

describe('toArticleParagraphs', () => {
  const tests = [
    {
      name: 'empty lesson has no paragraphs',
      input: [] as Sentence[],
      want: [],
    },
    {
      name: 'one sentence becomes one paragraph with its tokens and translation',
      input: [sentence(0, [token('毎朝', 'まいあさ'), token('。')], '每天早上。')],
      want: [{
        tokens: [token('毎朝', 'まいあさ'), token('。')],
        chinese: '每天早上。',
      }],
    },
    {
      name: 'unmarked sentences merge into one running paragraph without separators',
      input: [
        sentence(0, [token('毎朝'), token('。')], '每天早上。'),
        sentence(1, [token('起きます'), token('。')], '起床。'),
        sentence(2, [token('寝ます'), token('。')], '睡觉。'),
      ],
      want: [{
        tokens: [
          token('毎朝'), token('。'),
          token('起きます'), token('。'),
          token('寝ます'), token('。'),
        ],
        chinese: '每天早上。起床。睡觉。',
      }],
    },
    {
      name: 'sentences without translations are skipped in the Chinese paragraph',
      input: [
        sentence(0, [token('毎朝')], '每天早上。'),
        sentence(1, [token('起きます')], ''),
      ],
      want: [{
        tokens: [token('毎朝'), token('起きます')],
        chinese: '每天早上。',
      }],
    },
    {
      name: 'furigana readings survive the merge',
      input: [
        sentence(0, [token('七時', 'しちじ')], '七点。'),
        sentence(1, [token('学校', 'がっこう')], '学校。'),
      ],
      want: [{
        tokens: [token('七時', 'しちじ'), token('学校', 'がっこう')],
        chinese: '七点。学校。',
      }],
    },
    {
      name: 'different paragraph numbers split the article into separate paragraphs',
      input: [
        sentence(0, [token('毎朝'), token('。')], '每天早上。', 0),
        sentence(1, [token('起きます'), token('。')], '起床。', 0),
        sentence(2, [token('午後'), token('。')], '下午。', 1),
        sentence(3, [token('寝ます'), token('。')], '睡觉。', 1),
      ],
      want: [
        {
          tokens: [token('毎朝'), token('。'), token('起きます'), token('。')],
          chinese: '每天早上。起床。',
        },
        {
          tokens: [token('午後'), token('。'), token('寝ます'), token('。')],
          chinese: '下午。睡觉。',
        },
      ],
    },
    {
      name: 'unmarked sentences fall back to the first paragraph',
      input: [
        sentence(0, [token('前文')], '前文。'),
        sentence(1, [token('後文')], '后文。', 1),
      ],
      want: [
        { tokens: [token('前文')], chinese: '前文。' },
        { tokens: [token('後文')], chinese: '后文。' },
      ],
    },
    {
      name: 'non-consecutive paragraph numbers still split on every change',
      input: [
        sentence(0, [token('一')], '一。', 0),
        sentence(1, [token('二')], '二。', 2),
        sentence(2, [token('三')], '三。', 5),
      ],
      want: [
        { tokens: [token('一')], chinese: '一。' },
        { tokens: [token('二')], chinese: '二。' },
        { tokens: [token('三')], chinese: '三。' },
      ],
    },
    {
      name: 'a paragraph number reused later starts a fresh paragraph',
      input: [
        sentence(0, [token('一')], '一。', 0),
        sentence(1, [token('二')], '二。', 1),
        sentence(2, [token('三')], '三。', 0),
      ],
      want: [
        { tokens: [token('一')], chinese: '一。' },
        { tokens: [token('二')], chinese: '二。' },
        { tokens: [token('三')], chinese: '三。' },
      ],
    },
    {
      name: 'each paragraph keeps furigana and skips empty translations independently',
      input: [
        sentence(0, [token('七時', 'しちじ')], '七点。', 0),
        sentence(1, [token('起きます')], '', 0),
        sentence(2, [token('学校', 'がっこう')], '学校。', 1),
        sentence(3, [token('行きます')], '', 1),
      ],
      want: [
        { tokens: [token('七時', 'しちじ'), token('起きます')], chinese: '七点。' },
        { tokens: [token('学校', 'がっこう'), token('行きます')], chinese: '学校。' },
      ],
    },
  ]

  for (const tt of tests) {
    it(tt.name, () => {
      expect(toArticleParagraphs(tt.input)).toEqual(tt.want)
    })
  }
})

describe('groupSentencesByParagraph', () => {
  it('returns no paragraphs for an empty lesson', () => {
    expect(groupSentencesByParagraph([])).toEqual([])
  })

  it('keeps unmarked legacy sentences in one group', () => {
    const input = [
      sentence(0, [token('毎朝')], '每天早上。'),
      sentence(1, [token('起きます')], '起床。'),
    ]
    expect(groupSentencesByParagraph(input)).toEqual([{ sentences: input, chinese: '每天早上。起床。' }])
  })

  it('splits on paragraph changes while keeping sentence boundaries and order', () => {
    const input = [
      sentence(0, [token('一')], '一。', 0),
      sentence(1, [token('二')], '二。', 0),
      sentence(2, [token('三')], '三。', 1),
    ]
    expect(groupSentencesByParagraph(input)).toEqual([
      { sentences: [input[0], input[1]], chinese: '一。二。' },
      { sentences: [input[2]], chinese: '三。' },
    ])
  })

  it('does not reset indexed token offsets at a paragraph boundary', () => {
    const input = [
      sentence(0, [token('朝'), token('です。')], '早上了。', 0),
      sentence(1, [token('散歩')], '散步。', 1),
    ]
    const groups = groupSentencesByParagraph(indexLessonText(input))

    expect(groups.map((group) => group.sentences.flatMap((item) => item.tokens.map((t) => t.start)))).toEqual([
      [0, 1],
      [4],
    ])
  })
})
