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

describe('单词正确答案的读音、词性与自他动词', () => {
  it.each([
    { reading: 'きょう', reading_type: '3', part_of_speech: '名詞', verb_type: '' },
    { reading: 'おわる', reading_type: '2', part_of_speech: '五段動詞', verb_type: '自动词' },
    { reading: 'おえる', reading_type: '2', part_of_speech: '一段動詞', verb_type: '他动词' },
    { reading: 'ふく', reading_type: '2', part_of_speech: '五段動詞', verb_type: '自他两用' },
  ])('在 $reading 旁显示已标注的词性和动词类型', (word) => {
    const html = renderToStaticMarkup(<WordAnswerReading word={word} />)

    expect(html).toMatch(new RegExp(`${word.reading}</div><span[^>]*>${word.part_of_speech}</span>`))
    expect(html).toContain(`>${word.reading_type}</span>`)
    if (word.verb_type) expect(html).toContain(`>${word.verb_type}</span>`)
    else expect(html).not.toMatch(/自动词|他动词|自他两用/)
  })

  it.each(['', '自动词'])('保留自他标签，词性为 "%s" 时只显示一次', (partOfSpeech) => {
    const html = renderToStaticMarkup(
      <WordAnswerReading word={{ reading: 'おわる', reading_type: '', part_of_speech: partOfSpeech, verb_type: '自动词' }} />,
    )

    expect(html.match(/>自动词<\/span>/g)).toHaveLength(1)
  })

  it.each(['2', ''])('未标注词汇不显示空标签，读音类型为 "%s"', (readingType) => {
    const html = renderToStaticMarkup(
      <WordAnswerReading word={{ reading: 'ねこ', reading_type: readingType, part_of_speech: '', verb_type: '' }} />,
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
