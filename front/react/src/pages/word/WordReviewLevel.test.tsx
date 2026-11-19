import { renderToStaticMarkup } from 'react-dom/server'
import { StaticRouter } from 'react-router-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { WordReviewPage } from './WordReviewPage'
import { GrammarListPage } from '../grammar/GrammarListPage'

vi.mock('@/hooks/useDailyPracticeProgress', () => ({
  useDailyPracticeProgress: () => ({ progress: { status: 'loading' }, refreshProgress: () => {} }),
}))
vi.mock('react-i18next', () => ({
  useTranslation: () => ({ t: (key: string) => key }),
}))

describe('study plan links select the requested learning level', () => {
  it.each([
    { Page: WordReviewPage, path: '/words/review?level=N2', level: 'N2' },
    { Page: WordReviewPage, path: '/words/review?level=N3&mode=dictation', level: 'N3' },
    { Page: WordReviewPage, path: '/words/review?level=invalid', level: 'N5' },
    { Page: GrammarListPage, path: '/grammar?level=N2', level: 'N2' },
    { Page: GrammarListPage, path: '/grammar?level=N4', level: 'N4' },
    { Page: GrammarListPage, path: '/grammar?level=invalid', level: 'N5' },
  ])('$path selects $level', ({ Page, path, level }) => {
    const html = renderToStaticMarkup(<StaticRouter location={path}><Page /></StaticRouter>)
    expect(html).toMatch(new RegExp(`aria-pressed="true"[^>]*>\\s*${level}\\s*</button>`))
    expect(html.match(/aria-pressed="true"[^>]*>\s*N[1-5]\s*<\/button>/g)).toHaveLength(1)
  })
})
