import { renderToStaticMarkup } from 'react-dom/server'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import zh from '@/i18n/locales/zh'
import type { Sentence } from '@/types/api'
import { LessonReading } from './LessonReading'

const i18n = createInstance()

beforeAll(async () => {
  await i18n.init({ lng: 'zh', resources: { zh: { translation: zh } } })
})

afterEach(() => {
  vi.unstubAllGlobals()
})

function renderLesson() {
  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <LessonReading lessonId={1} sentences={sentences} />
    </I18nextProvider>,
  )
}

const sentences: Sentence[] = [
  { index: 0, tokens: [{ surface: '朝', reading: 'あさ' }, { surface: 'です。', reading: '' }], chinese: '早上了。', start_ms: 0, end_ms: 0 },
  { index: 1, tokens: [{ surface: '散歩', reading: 'さんぽ' }, { surface: 'します。', reading: '' }], chinese: '我去散步。', start_ms: 0, end_ms: 0 },
]

describe('LessonReading', () => {
  it.each([
    { name: 'a lesson with text', sentences, expected: '竖排阅读' },
    { name: 'a lesson without text', sentences: [], expected: '暂无课文正文' },
  ])('renders $name', ({ sentences, expected }) => {
    const html = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <LessonReading lessonId={1} sentences={sentences} />
      </I18nextProvider>,
    )

    expect(html).toContain(expected)
    if (sentences.length > 0) {
      expect(html).toMatch(/<p[^>]*><ruby>.*朝.*<rt>あさ<\/rt><\/ruby>.*です。.*<ruby>.*散歩.*<rt>さんぽ<\/rt><\/ruby>.*します。.*<\/p>/)
      expect(html).toMatch(/aria-pressed="true"[^>]*>竖排阅读<\/button>/)
      expect(html).not.toContain('早上了。')
      expect(html).toContain('点选模式')
      expect(html).toMatch(/data-lesson-end="\d+"/)
      expect(html).toContain('data-tap-mode="off"')
      expect(html).toContain('从右侧第一列开始，自上往下读。')
      expect(html).not.toContain('轻点文字选中')
    }
  })

  it('turns on tap selection by default on touch screens', () => {
    vi.stubGlobal('window', { matchMedia: () => ({ matches: true }) })
    const html = renderLesson()

    expect(html).toContain('data-tap-mode="on"')
    expect(html).toContain('轻点文字选中')
    expect(html).toContain('从右侧第一列开始，自上往下读。')
  })
})
