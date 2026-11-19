import { renderToStaticMarkup } from 'react-dom/server'
import { StaticRouter } from 'react-router-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { WordReviewPage } from './WordReviewPage'

vi.mock('@/hooks/useDailyPracticeProgress', () => ({
  useDailyPracticeProgress: () => ({ progress: { status: 'loading' }, refreshProgress: () => {} }),
}))
vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))

describe('review and new word queues', () => {
  it.each([
    { query: '?level=all', title: 'word.queue.title', selected: 'word.queue.scope.review', all: true },
    { query: '?level=N2&queue=new&mode=dictation', title: 'word.queue.learnTitle', selected: 'word.queue.scope.new', all: false },
  ])('keeps $query explicit', ({ query, title, selected, all }) => {
    const html = renderToStaticMarkup(<StaticRouter location={`/words/review${query}`}><WordReviewPage /></StaticRouter>)
    expect(html).toContain(`>${title}</h1>`)
    expect(html).toMatch(new RegExp(`aria-pressed="true"[^>]*>[^<]*${selected}`))
    expect(html.includes('word.queue.allLevels')).toBe(all)
  })
})
