import { renderToStaticMarkup } from 'react-dom/server'
import { StaticRouter } from 'react-router-dom/server'
import { describe, expect, it, vi } from 'vitest'
import { GrammarListPage } from './GrammarListPage'

vi.mock('react-i18next', () => ({ useTranslation: () => ({ t: (key: string) => key }) }))

describe('grammar list status on entry', () => {
  it.each([
    ['/grammar', 'unlearned'],
    ['/grammar?level=N2', 'unlearned'],
    ['/grammar?status=invalid', 'unlearned'],
    ['/grammar?level=N4&status=all', 'all'],
    ['/grammar?status=learning', 'learning'],
    ['/grammar?status=mastered', 'mastered'],
  ])('%s selects %s', (path, status) => {
    const html = renderToStaticMarkup(<StaticRouter location={path}><GrammarListPage /></StaticRouter>)
    expect(html).toMatch(new RegExp(`aria-pressed="true"[^>]*>grammar\\.list\\.filter\\.${status}</button>`))
    expect(html.match(/aria-pressed="true"[^>]*>grammar\.list\.filter\./g)).toHaveLength(1)
  })

  it('keeps direct grammar searches in the search view', () => {
    const html = renderToStaticMarkup(<StaticRouter location="/grammar?q=ながら"><GrammarListPage /></StaticRouter>)
    expect(html).toContain('语法搜索')
    expect(html).not.toContain('grammar.list.filter.unlearned')
  })
})
