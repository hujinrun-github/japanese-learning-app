import { renderToStaticMarkup } from 'react-dom/server'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { StaticRouter } from 'react-router-dom/server'
import { beforeAll, describe, expect, it } from 'vitest'
import { AuthProvider } from '@/contexts/AuthContext'
import zh from '@/i18n/locales/zh'
import { PageShell } from './PageShell'

const i18n = createInstance()
beforeAll(async () => {
  await i18n.init({ lng: 'zh', resources: { zh: { translation: zh } } })
})

describe('global lookup navigation', () => {
  it.each([
    ['/words/review', 'header'],
    ['/translation/practice', 'header'],
    ['/lesson/1', 'nav'],
    ['/notes/new', 'nav'],
  ])('shows a lookup dialog button in the %s shell’s %s', (path, section) => {
    const html = renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <StaticRouter location={path}>
          <AuthProvider>
            <PageShell><p>Learning content</p></PageShell>
          </AuthProvider>
        </StaticRouter>
      </I18nextProvider>,
    )
    const navigation = section === 'header' ? html.split('</header>')[0] : html.split('</main>')[1]

    expect(navigation).toMatch(/<button\b[^>]*aria-haspopup="dialog"[^>]*>(?:(?!<\/button>).)*搜索<\/[^>]+>(?:<\/button>)?/)
    expect(navigation).not.toContain('href="/search"')
    expect(html).toContain('<main')
    expect(html).toContain('Learning content')
  })
})
