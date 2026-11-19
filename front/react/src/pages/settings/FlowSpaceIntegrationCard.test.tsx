import { renderToStaticMarkup } from 'react-dom/server'
import { createInstance } from 'i18next'
import { I18nextProvider } from 'react-i18next'
import { beforeAll, describe, expect, it } from 'vitest'
import type { FlowSpaceIntegrationStatus } from '@/api/flowspace'
import { FlowSpaceIntegrationView } from './FlowSpaceIntegrationCard'

const i18n = createInstance()
beforeAll(async () => { await i18n.init({ lng: 'zh', resources: { zh: { translation: {} } } }) })
const disconnected: FlowSpaceIntegrationStatus = { available: true, connected: false, pending_count: 0, delivered_count: 0, events: [] }

function render(state: FlowSpaceIntegrationStatus | null, error = '', busy = false) {
  return renderToStaticMarkup(<I18nextProvider i18n={i18n}><FlowSpaceIntegrationView
    status={state} loading={false} busy={busy} error={error} notice="" code=""
    onCodeChange={() => {}} onPair={() => {}} onRefresh={() => {}} onRetry={() => {}} onDisconnect={() => {}}
  /></I18nextProvider>)
}

describe('FlowSpace integration settings', () => {
  it('offers a labelled pairing input and explains what becomes automatic', () => {
    const html = render(disconnected)
    expect(html).toContain('FlowSpace 任务联动')
    expect(html).toContain('配对码')
    expect(html).toContain('type="password"')
    expect(html).toContain('连接 FlowSpace')
    expect(html).toContain('当天')
  })

  it('unconfigured service does not offer a working connect action', () => {
    const html = render({ ...disconnected, available: false })
    expect(html).toContain('尚未启用')
    expect(html).not.toContain('连接 FlowSpace</button>')
  })

  it('keeps delivered and applied statuses distinct and exposes recovery controls', () => {
    const html = render({ ...disconnected, connected: true, workspace_name: '日常计划', pending_count: 1, delivered_count: 2, events: [
      { event_id: 'received', kind: 'task', status: 'delivered', remote_status: 'received', created_at: '2026-11-19T10:00:00Z' },
      { event_id: 'applied', kind: 'task', status: 'delivered', remote_status: 'applied', created_at: '2026-11-19T09:00:00Z' },
    ] })
    for (const text of ['日常计划', '待送达', '已送达', '待处理', '已处理', '重试同步', '断开连接']) expect(html).toContain(text)
    expect(html).not.toContain('2 个任务已完成')
  })

  it('shows failed reads as an error instead of an unconnected account', () => {
    const html = render(null, '连接状态读取失败')
    expect(html).toContain('role="alert"')
    expect(html).toContain('连接状态读取失败')
    expect(html).not.toContain('未连接')
  })

  it('disables actions while a mutation is being submitted', () => {
    const html = render({ ...disconnected, connected: true }, '', true)
    expect(html.match(/disabled=""/g)?.length).toBeGreaterThanOrEqual(3)
  })
})
