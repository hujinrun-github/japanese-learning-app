import { useState, useEffect, useCallback } from 'react'
import { adminFetch } from '@/api/client'
import styles from './Users.module.css'

interface User {
  id: number
  name: string
  email: string
  jlpt_levels: string[]
  streak_days: number
  created_at: string
}

interface ModuleStat {
  due_count: number
  mastered_count: number
  total_count: number
  today_completed: number
  daily_goal: number
}

interface UserStats {
  streak_days: number
  modules: Record<string, ModuleStat>
}

const PAGE_SIZE = 20

const MODULE_LABELS: Record<string, string> = {
  word: 'Words',
  grammar: 'Grammar',
  speaking: 'Speaking',
  writing: 'Writing',
}

export default function UsersPage() {
  const [items, setItems] = useState<User[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [expandedId, setExpandedId] = useState<number | null>(null)
  const [stats, setStats] = useState<UserStats | null>(null)
  const [statsLoading, setStatsLoading] = useState(false)

  const totalPages = Math.ceil(total / PAGE_SIZE)

  const fetchItems = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      const params = new URLSearchParams({ page: String(page), size: String(PAGE_SIZE) })
      const data = await adminFetch<{ items: User[]; total: number }>('GET', `/users?${params}`)
      setItems(data.items)
      setTotal(data.total)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }, [page])

  useEffect(() => {
    fetchItems()
  }, [fetchItems])

  async function handleToggleExpand(userId: number) {
    if (expandedId === userId) {
      setExpandedId(null)
      setStats(null)
      return
    }
    setExpandedId(userId)
    setStatsLoading(true)
    setError('')
    try {
      const data = await adminFetch<UserStats>('GET', `/users/${userId}/stats`)
      setStats(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load stats')
    } finally {
      setStatsLoading(false)
    }
  }

  const columns = [
    { key: 'id', label: 'ID' },
    { key: 'name', label: 'Name' },
    { key: 'email', label: 'Email' },
    { key: 'jlpt_levels', label: 'JLPT Levels', render: (v: string[]) => (Array.isArray(v) ? v.join(', ') : String(v)) },
    { key: 'streak_days', label: 'Streak' },
    { key: 'created_at', label: 'Created', render: (v: string) => (typeof v === 'string' ? v.slice(0, 10) : String(v)) },
  ]

  return (
    <div className={styles.page}>
      <h2 className={styles.title}>Users</h2>

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
                <td colSpan={columns.length} className={styles.empty}>
                  No data
                </td>
              </tr>
            ) : (
              items.map((u) => (
                <>
                  <tr
                    key={u.id}
                    className={styles.clickableRow}
                    onClick={() => handleToggleExpand(u.id)}
                  >
                    {columns.map((c) => {
                      const val = (u as Record<string, unknown>)[c.key]
                      return (
                        <td key={c.key}>
                          {c.render ? c.render(val as never) : (val as React.ReactNode)}
                        </td>
                      )
                    })}
                  </tr>
                  {expandedId === u.id && (
                    <tr className={styles.expandedRow} key={`${u.id}-stats`}>
                      <td colSpan={columns.length}>
                        {statsLoading ? (
                          <p className={styles.loading}>Loading stats...</p>
                        ) : stats ? (
                          <div>
                            <div className={styles.streakBadge}>
                              Streak: {stats.streak_days} days
                            </div>
                            <div className={styles.statsGrid}>
                              {Object.entries(stats.modules).map(([mod, stat]) => (
                                <div key={mod} className={styles.statCard}>
                                  <h4>{MODULE_LABELS[mod] || mod}</h4>
                                  <div className={styles.statRow}>
                                    <span>Due</span>
                                    <span>{stat.due_count}</span>
                                  </div>
                                  <div className={styles.statRow}>
                                    <span>Mastered</span>
                                    <span>{stat.mastered_count}</span>
                                  </div>
                                  <div className={styles.statRow}>
                                    <span>Total</span>
                                    <span>{stat.total_count}</span>
                                  </div>
                                  <div className={styles.statRow}>
                                    <span>Today</span>
                                    <span>
                                      {stat.today_completed} / {stat.daily_goal}
                                    </span>
                                  </div>
                                </div>
                              ))}
                            </div>
                          </div>
                        ) : null}
                      </td>
                    </tr>
                  )}
                </>
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
