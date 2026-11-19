import { useCallback, useEffect, useRef, useState } from 'react'
import { useTranslation } from 'react-i18next'
import { disconnectFlowSpace, getFlowSpaceIntegration, pairFlowSpace, retryFlowSpaceIntegration } from '@/api/flowspace'
import type { FlowSpaceIntegrationEvent, FlowSpaceIntegrationStatus } from '@/api/flowspace'
import styles from './SettingsPage.module.css'

interface ViewProps {
  status: FlowSpaceIntegrationStatus | null
  loading: boolean
  busy: boolean
  error: string
  notice: string
  code: string
  onCodeChange: (value: string) => void
  onPair: () => void
  onRefresh: () => void
  onRetry: () => void
  onDisconnect: () => void
}

function eventLabel(event: FlowSpaceIntegrationEvent) {
  if (event.status !== 'delivered') {
    if (event.status === 'failed' || event.status === 'paused' || event.status === 'needs_attention') return '需要处理'
    if (event.status === 'processing') return '正在送达'
    return '待送达'
  }
  switch (event.remote_status) {
    case 'applied': return '已处理'
    case 'already_done': return '目标任务原已完成'
    case 'stale': return '已忽略旧状态'
    case 'needs_attention': return 'FlowSpace 需要处理'
    case 'received':
    case 'processing':
    case 'retry_wait': return '已送达，待处理'
    default: return '已送达 FlowSpace'
  }
}

export function FlowSpaceIntegrationView({ status, loading, busy, error, notice, code, onCodeChange, onPair, onRefresh, onRetry, onDisconnect }: ViewProps) {
  const { t } = useTranslation()
  const text = (key: string, fallback: string) => t(`settings.flowspace.${key}`, { defaultValue: fallback })
  return (
    <section className={styles.card} aria-labelledby="flowspace-integration-heading" aria-busy={loading || busy}>
      <h2 id="flowspace-integration-heading" className={styles.cardTitle}>🔗 {text('title', 'FlowSpace 任务联动')}</h2>
      <p className={styles.syncHint}>{text('description', '日语学习任务完成后，自动完成已关联的 FlowSpace 当天任务。请先在 FlowSpace 中生成配对码，连接后再关联学习任务。')}</p>
      {loading && <p role="status" className={styles.msg}>{text('loading', '正在读取连接状态…')}</p>}
      {status && !status.available && <p role="status" className={styles.msg}>{text('unavailable', 'FlowSpace 联动尚未启用，请联系管理员配置后刷新。')}</p>}
      {status?.available && !status.connected && <form onSubmit={event => { event.preventDefault(); if (code.trim() && !busy) onPair() }}>
        <div className={styles.field}>
          <label className={styles.label} htmlFor="flowspace-pairing-code">{text('pairingCode', '配对码')}</label>
          <input id="flowspace-pairing-code" className={styles.input} type="password" autoComplete="off" spellCheck={false}
            value={code} onChange={event => onCodeChange(event.target.value)} disabled={busy} maxLength={512}
            aria-describedby="flowspace-pairing-hint" />
          <small id="flowspace-pairing-hint">{text('pairingHint', '输入 FlowSpace 生成的一次性配对码。配对码过期后请重新生成。')}</small>
        </div>
        <button className={styles.saveBtn} type="submit" disabled={busy || !code.trim()}>{text('connect', '连接 FlowSpace')}</button>
      </form>}
      {status?.connected && <>
        <p role="status" className={styles.msg}>{text('connected', '已连接')}：{status.workspace_name || status.workspace_id || 'FlowSpace'}</p>
        <p className={styles.syncHint}>{text('pending', '待送达')} {status.pending_count ?? 0} · {text('delivered', '已送达')} {status.delivered_count ?? 0}</p>
        <p className={styles.syncHint}>{text('deliveryHint', '已送达表示 FlowSpace 已收到事件，处理后才会更新任务。请在 FlowSpace 任务详情确认自动完成来源。')}</p>
        <div className={styles.buttonRow}>
          <button className={styles.secondaryBtn} type="button" onClick={onRetry} disabled={busy}>{text('retry', '重试同步')}</button>
          <button className={styles.dangerBtn} type="button" onClick={onDisconnect} disabled={busy}>{text('disconnect', '断开连接')}</button>
        </div>
        {!!status.events?.length && <div className={styles.pendingList}>
          <h3 className={styles.pendingTitle}>{text('events', '最近同步记录')}</h3>
          <ul className={styles.integrationEvents}>
            {status.events.map(event => <li key={event.event_id} className={styles.pendingItem}>
              <div className={styles.integrationEvent}>
                <span>{event.kind === 'catalog' ? text('catalog', '学习计划目录') : text('taskState', '学习任务状态')}</span>
                <span>{text(`event.${event.remote_status || event.status}`, eventLabel(event))}</span>
                <time dateTime={event.created_at}>{event.created_at}</time>
                {event.error && <span className={styles.msgError}>{event.error}</span>}
              </div>
            </li>)}
          </ul>
        </div>}
      </>}
      {(error || status?.last_error) && <p role="alert" className={`${styles.msg} ${styles.msgError}`}>{error || status?.last_error}</p>}
      {notice && <p role="status" className={`${styles.msg} ${styles.msgSuccess}`}>{notice}</p>}
      <button className={styles.secondaryBtn} type="button" onClick={onRefresh} disabled={loading || busy}>{text('refresh', '刷新连接状态')}</button>
    </section>
  )
}

export default function FlowSpaceIntegrationCard() {
  const [status, setStatus] = useState<FlowSpaceIntegrationStatus | null>(null)
  const [code, setCode] = useState('')
  const [loading, setLoading] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const request = useRef<AbortController | null>(null)

  const run = useCallback(async (operation: (signal: AbortSignal) => Promise<FlowSpaceIntegrationStatus>, message = '', initial = false) => {
    request.current?.abort()
    const controller = new AbortController()
    request.current = controller
    if (initial) setLoading(true)
    else setBusy(true)
    setError('')
    setNotice('')
    try {
      const next = await operation(controller.signal)
      if (controller.signal.aborted) return
      setStatus(next)
      setNotice(message)
      if (message) setCode('')
    } catch (err) {
      if (!controller.signal.aborted) setError(err instanceof Error ? err.message : '连接状态读取失败，请重试。')
    } finally {
      if (!controller.signal.aborted) { setLoading(false); setBusy(false) }
    }
  }, [])

  useEffect(() => {
    void run(getFlowSpaceIntegration, '', true)
    return () => request.current?.abort()
  }, [run])

  useEffect(() => {
    if (!status?.connected) return
    const timer = window.setInterval(() => {
      if (document.visibilityState === 'visible' && !request.current?.signal.aborted && !busy) void run(getFlowSpaceIntegration)
    }, 15000)
    return () => window.clearInterval(timer)
  }, [status?.connected, busy, run])

  return <FlowSpaceIntegrationView status={status} loading={loading} busy={busy} error={error} notice={notice} code={code}
    onCodeChange={setCode} onRefresh={() => { void run(getFlowSpaceIntegration) }}
    onPair={() => { void run(signal => pairFlowSpace(code.trim(), signal), '已连接 FlowSpace，请在 FlowSpace 任务详情关联学习任务。') }}
    onRetry={() => { void run(retryFlowSpaceIntegration, '已安排重试，请刷新查看送达和处理结果。') }}
    onDisconnect={() => { void run(async signal => { await disconnectFlowSpace(signal); return getFlowSpaceIntegration(signal) }, '已断开连接，FlowSpace 中已有任务状态保留。') }} />
}
