import { renderToStaticMarkup } from 'react-dom/server'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { beforeAll, describe, expect, it } from 'vitest'
import zh from '@/i18n/locales/zh'
import type { ErrataReport } from './errataEditRules'
import { ErrataReportCard } from './ErrataHistoryPage'

const i18n = createInstance()
beforeAll(async () => {
  await i18n.init({ lng: 'zh', resources: { zh: { translation: zh } } })
})

const base: ErrataReport = {
  id: 1,
  module: 'grammar',
  item_key: '3:quiz:12',
  item_label: '原文摘要',
  category: 'prompt',
  description: '原始描述',
  status: 'pending',
  created_at: '2026-11-18T00:00:00Z',
}

function render(report: ErrataReport) {
  return renderToStaticMarkup(
    <I18nextProvider i18n={i18n}>
      <ErrataReportCard report={report} onChanged={() => {}} />
    </I18nextProvider>,
  )
}

describe('ErrataReportCard 按题型与状态展示操作按钮', () => {
  it.each([
    { name: '待处理的文本类同时显示编辑和删除', overrides: {}, edit: true, remove: true },
    { name: '待处理的答案类同时显示编辑和删除', overrides: { category: 'answer' }, edit: true, remove: true },
    { name: '待处理的音视频类只显示编辑', overrides: { category: 'audio' }, edit: true, remove: false },
    { name: '待处理的其他类同时显示编辑和删除', overrides: { category: 'other' }, edit: true, remove: true },
    { name: '已处理不显示任何操作按钮', overrides: { status: 'resolved' }, edit: false, remove: false },
    { name: '已处理的音视频类不显示任何操作按钮', overrides: { status: 'resolved', category: 'audio' }, edit: false, remove: false },
  ])('$name', ({ overrides, edit, remove }) => {
    const html = render({ ...base, ...overrides } as ErrataReport)
    expect(html.includes('编辑')).toBe(edit)
    expect(html.includes('删除')).toBe(remove)
  })

  it('卡片仍展示状态、题面与说明', () => {
    const html = render(base)
    expect(html).toContain('原文摘要')
    expect(html).toContain('原始描述')
    expect(html).toContain('待处理')
  })
})
