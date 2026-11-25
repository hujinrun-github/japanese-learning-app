import { renderToStaticMarkup } from 'react-dom/server'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { beforeAll, describe, expect, it } from 'vitest'
import zh from '@/i18n/locales/zh'
import { DailyPracticeProgress, type DailyPracticeState } from './DailyPracticeProgress'

const i18n = createInstance()
beforeAll(async () => {
  await i18n.init({ lng: 'zh', resources: { zh: { translation: zh } }, interpolation: { escapeValue: false } })
})

describe('DailyPracticeProgress', () => {
  it.each([
    { progress: { status: 'ready', completed: 4, goal: 5 }, expected: '今日 4/5' },
    { progress: { status: 'ready', completed: 0, goal: 5 }, expected: '今日 0/5' },
    { progress: { status: 'ready', completed: 0, goal: 5, scope: 'N2 / N3' }, expected: '今日 0/5 · N2 / N3' },
    { progress: { status: 'ready', completed: 7, goal: 5 }, expected: '今日 7/5' },
    { progress: { status: 'error' }, expected: '今日进度暂不可用' },
    { progress: { status: 'loading' }, expected: '正在加载今日进度' },
  ] satisfies { progress: DailyPracticeState; expected: string }[])('displays $expected', ({ progress, expected }) => {
    const html = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}><DailyPracticeProgress progress={progress} /></I18nextProvider>,
    )
    expect(html).toContain(expected)
  })
})
