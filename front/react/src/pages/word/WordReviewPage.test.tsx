import { renderToStaticMarkup } from 'react-dom/server'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { beforeAll, describe, expect, it } from 'vitest'
import zh from '@/i18n/locales/zh'
import { WordAnswerReading, WordRatingActions } from './WordReviewPage'

const i18n = createInstance()
beforeAll(async () => {
  await i18n.init({ lng: 'zh', resources: { zh: { translation: zh } } })
})

describe('单词正确答案的读音与自他动词标记', () => {
  it.each([
    { reading: 'おわる', reading_type: '2', verb_type: '自动词' },
    { reading: 'おえる', reading_type: '2', verb_type: '他动词' },
    { reading: 'ふく', reading_type: '2', verb_type: '自他两用' },
  ])('在 $reading 旁显示 $verb_type', (word) => {
    const html = renderToStaticMarkup(<WordAnswerReading word={word} />)

    expect(html).toMatch(new RegExp(`${word.reading}</div><span[^>]*>${word.verb_type}</span>`))
    expect(html).toContain(`>${word.reading_type}</span>`)
  })

  it.each(['2', ''])('未标注词汇不显示空标签，读音类型为 "%s"', (readingType) => {
    const html = renderToStaticMarkup(
      <WordAnswerReading word={{ reading: 'ねこ', reading_type: readingType, verb_type: '' }} />,
    )

    expect(html).toContain('ねこ</div></div>')
    expect(html).not.toMatch(/<span[^>]*><\/span>/)
  })
})

describe('单词复习自评', () => {
  it.each([false, true])('只展示两档掌握程度，提交中为 %s', (disabled) => {
    const html = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <WordRatingActions onRate={() => {}} disabled={disabled} />
      </I18nextProvider>,
    )

    expect(html.match(/<button\b/g)).toHaveLength(2)
    expect(html).toContain('有印象')
    expect(html).toContain('已完全掌握')
    expect(html).toContain('明天再复习')
    expect(html).toContain('至少 7 天后再复习')
    expect(html.match(/disabled=""/g) ?? []).toHaveLength(disabled ? 2 : 0)
  })
})
