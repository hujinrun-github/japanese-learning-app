import { useEffect, useState } from 'react'
import { adminFetch } from '@/api/client'
import styles from './Errata.module.css'

type Status = 'pending' | 'resolved'

type Report = {
  id: number
  user_id: number
  module: string
  item_key: string
  item_label: string
  category: string
  description: string
  status: Status
  created_at: string
}

type ReportPage = { items: Report[]; has_more: boolean }

const modules: Record<string, string> = {
  word: '单词',
  grammar: '语法',
  lesson: '课文',
  speaking: '口语',
  shadowing: '影子跟读',
  writing: '写作',
  translation: '翻译',
}

const categories: Record<string, string> = {
  prompt: '题目或原文',
  answer: '答案或译文',
  explanation: '解析或注音',
  audio: '音频或视频',
  other: '其他',
}

const statusLabels: Record<Status, string> = { pending: '待处理', resolved: '已处理' }

export default function ErrataPage() {
  const [filter, setFilter] = useState<Status | ''>('pending')
  const [page, setPage] = useState(1)
  const [data, setData] = useState<ReportPage>({ items: [], has_more: false })
  const [refresh, setRefresh] = useState(0)
  const [loading, setLoading] = useState(true)
  const [updating, setUpdating] = useState<number | null>(null)
  const [error, setError] = useState('')

  useEffect(() => {
    let ignore = false
    async function loadReports() {
      setLoading(true)
      setError('')
      try {
        const params = new URLSearchParams({ status: filter, page: String(page) })
        const result = await adminFetch<ReportPage>('GET', `/errata?${params}`)
        if (!ignore) setData(result)
      } catch (err) {
        if (!ignore) setError(err instanceof Error ? err.message : '加载勘误失败')
      } finally {
        if (!ignore) setLoading(false)
      }
    }
    void loadReports()
    return () => { ignore = true }
  }, [filter, page, refresh])

  async function changeStatus(report: Report) {
    if (updating !== null) return
    const status = report.status === 'pending' ? 'resolved' : 'pending'
    setUpdating(report.id)
    setError('')
    try {
      await adminFetch('PATCH', `/errata/${report.id}`, { status })
      if (filter && data.items.length === 1 && page > 1) {
        setPage(page - 1)
      } else {
        setRefresh(value => value + 1)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : '更新勘误失败')
    } finally {
      setUpdating(null)
    }
  }

  return (
    <div className="adm-page">
      <div className="adm-header">
        <h2 className="adm-title">练习勘误</h2>
      </div>
      <div className="adm-toolbar">
        <label htmlFor="errata-status">处理状态</label>
        <select
          id="errata-status"
          value={filter}
          disabled={updating !== null}
          onChange={(event) => { setFilter(event.target.value as Status | ''); setPage(1) }}
        >
          <option value="pending">待处理</option>
          <option value="resolved">已处理</option>
          <option value="">全部</option>
        </select>
      </div>
      {error && <p className="adm-error" role="alert">{error}</p>}
      {loading ? (
        <p className="adm-loading" role="status">正在加载勘误…</p>
      ) : (
        <>
          {data.items.length === 0 ? (
            <p className="adm-empty">暂无符合条件的勘误。</p>
          ) : (
            <ul className={styles.list}>
              {data.items.map(report => (
                <li key={report.id} className={styles.report}>
                  <div className={styles.meta}>
                    <strong>{modules[report.module] ?? report.module} · {categories[report.category] ?? report.category}</strong>
                    <span>{statusLabels[report.status]}</span>
                  </div>
                  <h3 className={styles.item}>{report.item_label}</h3>
                  <p className={styles.description}>{report.description}</p>
                  <div className={styles.footer}>
                    <div className={styles.meta}>
                      <span>报告 #{report.id}</span>
                      <span>题目 {report.item_key}</span>
                      <span>用户 #{report.user_id}</span>
                      <time dateTime={report.created_at}>{new Date(report.created_at).toLocaleString()}</time>
                    </div>
                    <button className="adm-btn" disabled={updating !== null} onClick={() => { void changeStatus(report) }}>
                      {updating === report.id ? '处理中…' : report.status === 'pending' ? '标记已处理' : '重新打开'}
                    </button>
                  </div>
                </li>
              ))}
            </ul>
          )}
          {(page > 1 || data.has_more) && (
            <div className="adm-pagination">
              <button disabled={page === 1 || updating !== null} onClick={() => setPage(value => value - 1)}>上一页</button>
              <span className="adm-pageInfo">第 {page} 页</span>
              <button disabled={!data.has_more || updating !== null} onClick={() => setPage(value => value + 1)}>下一页</button>
            </div>
          )}
        </>
      )}
    </div>
  )
}
