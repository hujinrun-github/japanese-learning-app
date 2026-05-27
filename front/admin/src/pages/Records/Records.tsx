import { useState, useEffect, useCallback } from 'react'
import { adminFetch } from '@/api/client'
import styles from './Records.module.css'

type RecordItem = Record<string, unknown>

const MODULES = ['word', 'grammar', 'speaking', 'writing'] as const
const PAGE_SIZE = 20

// Large JSON fields that are not useful in table view
const SKIP_COLUMNS = new Set(['review_history', 'quiz_history', 'ai_feedback'])

export default function RecordsPage() {
  const [module, setModule] = useState('word')
  const [userId, setUserId] = useState('')
  const [items, setItems] = useState<RecordItem[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')

  const totalPages = Math.ceil(total / PAGE_SIZE)

  const fetchItems = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const params = new URLSearchParams({ page: String(page), size: String(PAGE_SIZE) })
      if (userId) params.set('user_id', userId)
      const data = await adminFetch<{ items: RecordItem[]; total: number }>('GET', `/records/${module}?${params}`)
      setItems(data.items)
      setTotal(data.total)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }, [module, userId, page])

  useEffect(() => {
    fetchItems()
  }, [fetchItems])

  function handleModuleChange(newModule: string) {
    setModule(newModule)
    setPage(1)
    setItems([])
    setTotal(0)
  }

  function handleSearch() {
    setPage(1)
  }

  // Derive columns from the first record's keys, skipping large JSON fields
  const columns = items.length > 0
    ? Object.keys(items[0])
        .filter((k) => !SKIP_COLUMNS.has(k))
        .map((k) => ({
          key: k,
          label: k.replace(/_/g, ' ').replace(/\b\w/g, (c) => c.toUpperCase()),
        }))
    : []

  function renderCell(val: unknown): React.ReactNode {
    if (val === null || val === undefined) return '-'
    if (typeof val === 'string' && /^\d{4}-\d{2}-\d{2}T/.test(val)) {
      return val.slice(0, 19).replace('T', ' ')
    }
    if (typeof val === 'object') return JSON.stringify(val)
    return String(val)
  }

  return (
    <div className={styles.page}>
      <h2 className={styles.title}>Records</h2>

      <div className={styles.toolbar}>
        <select value={module} onChange={(e) => handleModuleChange(e.target.value)}>
          {MODULES.map((m) => (
            <option key={m} value={m}>
              {m.charAt(0).toUpperCase() + m.slice(1)}
            </option>
          ))}
        </select>
        <input
          placeholder="User ID (optional)..."
          value={userId}
          onChange={(e) => setUserId(e.target.value)}
          onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
        />
      </div>

      {error && <p className={styles.error}>{error}</p>}

      {loading ? (
        <p className={styles.loading}>Loading...</p>
      ) : (
        <table className={styles.table}>
          <thead>
            <tr>
              {columns.map((c) => (
                <th key={c.key}>{c.label}</th>
              ))}
            </tr>
          </thead>
          <tbody>
            {items.length === 0 ? (
              <tr>
                <td colSpan={columns.length || 1} className={styles.empty}>
                  No data
                </td>
              </tr>
            ) : (
              items.map((item, idx) => (
                <tr key={(item.id as number) ?? idx}>
                  {columns.map((c) => (
                    <td key={c.key}>{renderCell(item[c.key])}</td>
                  ))}
                </tr>
              ))
            )}
          </tbody>
        </table>
      )}

      <div className={styles.pagination}>
        <button disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
          Prev
        </button>
        <span>
          Page {page} / {totalPages || 1} (Total: {total})
        </span>
        <button disabled={page >= totalPages} onClick={() => setPage((p) => p + 1)}>
          Next
        </button>
      </div>
    </div>
  )
}
