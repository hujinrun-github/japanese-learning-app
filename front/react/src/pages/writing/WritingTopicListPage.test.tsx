import { renderToStaticMarkup } from 'react-dom/server'
import type { ComponentProps } from 'react'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { StaticRouter } from 'react-router-dom/server'
import { beforeAll, describe, expect, it } from 'vitest'
import zh from '@/i18n/locales/zh'
import type { WritingTopic } from '@/types/api'
import { WritingTopicListView } from './WritingTopicListPage'

const i18n = createInstance()
beforeAll(async () => {
  await i18n.init({ lng: 'zh', resources: { zh: { translation: zh } } })
})

const topics: WritingTopic[] = [
  { id: '1', title: '自己紹介', jlpt_level: 'N5', essay_prompt: '自己紹介を書いてください', sentence_prompt: '〜ですを使う', practiced: false },
  { id: '2', title: '趣味', jlpt_level: 'N4', essay_prompt: '趣味について', sentence_prompt: '〜が好き', practiced: true },
  { id: '3', title: '旅行', jlpt_level: 'N5', essay_prompt: '旅行の思い出', sentence_prompt: '〜に行きたい', practiced: true },
]

type ViewProps = ComponentProps<typeof WritingTopicListView>

const base: ViewProps = {
  topics,
  loading: false,
  error: '',
  level: 'all',
  status: 'all',
  onLevelChange: () => {},
  onStatusChange: () => {},
  onRetry: () => {},
}

function render(overrides: Partial<ViewProps> = {}) {
  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <StaticRouter location="/writing">
        <WritingTopicListView {...base} {...overrides} />
      </StaticRouter>
    </I18nextProvider>,
  )
}

describe('WritingTopicListView 展示主题列表三态与筛选结果', () => {
  const cases: {
    name: string
    overrides: Partial<ViewProps>
    titles: string[]
    wantLoading: boolean
    wantError: boolean
    wantEmpty: boolean
  }[] = [
    { name: '加载中不渲染主题卡片', overrides: { loading: true }, titles: [], wantLoading: true, wantError: false, wantEmpty: false },
    { name: '加载失败显示错误与重试', overrides: { error: '加载失败' }, titles: [], wantLoading: false, wantError: true, wantEmpty: false },
    { name: '没有主题显示空态', overrides: { topics: [] }, titles: [], wantLoading: false, wantError: false, wantEmpty: true },
    { name: '默认渲染全部三张卡片', overrides: {}, titles: ['自己紹介', '趣味', '旅行'], wantLoading: false, wantError: false, wantEmpty: false },
    { name: '按 N5 筛选', overrides: { level: 'N5' }, titles: ['自己紹介', '旅行'], wantLoading: false, wantError: false, wantEmpty: false },
    { name: '只看已练习', overrides: { status: 'practiced' }, titles: ['趣味', '旅行'], wantLoading: false, wantError: false, wantEmpty: false },
    { name: '筛选无结果显示空态', overrides: { level: 'N1' }, titles: [], wantLoading: false, wantError: false, wantEmpty: true },
  ]

  it.each(cases)('$name', ({ overrides, titles, wantLoading, wantError, wantEmpty }) => {
    const html = render(overrides)
    for (const title of topics.map(topic => topic.title)) {
      expect(html.includes(title)).toBe(titles.includes(title))
    }
    const cardLinks = html.match(/href="\/writing\/practice\?topic_id=/g)?.length ?? 0
    expect(cardLinks).toBe(titles.length)
    expect(html.includes('aria-label="正在加载主题"')).toBe(wantLoading)
    expect(html.includes('加载失败')).toBe(wantError)
    expect(html.includes('没有符合筛选条件的主题')).toBe(wantEmpty)
  })

  it('每张卡片带级别、练习状态与可见的练习入口', () => {
    const html = render()
    expect(html).toContain('N5')
    expect(html).toContain('N4')
    expect(html).toContain('data-status="unpracticed"')
    expect(html).toContain('data-status="practiced"')
    expect(html).toContain('href="/writing/practice?topic_id=1"')
    expect(html).toContain('开始练习')
  })

  it('顶部提供直接进入练习页的入口和进度统计', () => {
    const html = render()
    expect(html).toContain('href="/writing/practice"')
    expect(html).toContain('href="/writing/questions"')
    expect(html).toContain('共 3 个主题')
    expect(html).toContain('已练习 2 个')
  })
})
