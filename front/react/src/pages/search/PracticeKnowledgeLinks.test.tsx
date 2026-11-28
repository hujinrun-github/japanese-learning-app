import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { renderToStaticMarkup } from 'react-dom/server'
import { beforeAll, describe, expect, it } from 'vitest'
import zh from '@/i18n/locales/zh'
import type { PracticeKnowledgeLink } from '@/types/api'
import { PracticeKnowledgeLinks } from './PracticeKnowledgeLinks'

const i18n = createInstance()
beforeAll(async () => {
  await i18n.init({ lng: 'zh', resources: { zh: { translation: zh } } })
})

describe('PracticeKnowledgeLinks', () => {
  it.each<{ links?: PracticeKnowledgeLink[]; count: number }>([
    { count: 0 },
    { links: [], count: 0 },
    { links: [
      { kind: 'word', id: 41, title: '勉強', reading: 'べんきょう', meaning: '学习', jlpt_level: 'N5' },
      { kind: 'grammar', id: 41, title: '〜ています', meaning: '正在', jlpt_level: 'N5' },
    ], count: 2 },
  ])('renders $count direct lookup buttons without navigating away', ({ links, count }) => {
    const html = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <PracticeKnowledgeLinks links={links} />
      </I18nextProvider>,
    )
    expect(html.match(/aria-haspopup="dialog"/g) ?? []).toHaveLength(count)
    expect(html).not.toContain('href=')
    if (count) {
      expect(html).toContain('关联单词与语法')
      expect(html).toContain('べんきょう')
      expect(html).toContain('〜ています')
    } else {
      expect(html).toBe('')
    }
  })
})
