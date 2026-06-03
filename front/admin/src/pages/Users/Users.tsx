import { useState, useEffect, useCallback } from 'react'
import { adminFetch } from '@/api/client'
import styles from './Users.module.css'

interface User {
  id: number; name: string; email: string; jlpt_levels: string[]; streak_days: number; created_at: string
}

interface ModuleStat {
  due_count: number; mastered_count: number; total_count: number; today_completed: number; daily_goal: number
}

interface UserStats {
  streak_days: number; modules: Record<string, ModuleStat>
}

const PAGE_SIZE = 20

const MODULE_LABELS: Record<string, string> = {
  word: 'Words', grammar: 'Grammar', speaking: 'Speaking', writing: 'Writing',
}

const MODULE_ICONS: Record<string, string> = {
  word: '📝', grammar: '📐', speaking: '🎙', writing: '✍️',
}

const LVL_CSS: Record<string, string> = { N5: 'adm-lvlN5', N4: 'adm-lvlN4', N3: 'adm-lvlN3', N2: 'adm-lvlN2', N1: 'adm-lvlN1' }

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
    setLoading(true); setError('')
    try {
      const params = new URLSearchParams({ page: String(page), size: String(PAGE_SIZE) })
      const data = await adminFetch<{ items: User[]; total: number }>('GET', `/users?${params}`)
      setItems(data.items); setTotal(data.total)
    } catch (err) { setError(err instanceof Error ? err.message : 'Failed to load') }
    finally { setLoading(false) }
  }, [page])

  useEffect(() => { fetchItems() }, [fetchItems])

  async function handleToggleExpand(userId: number) {
    if (expandedId === userId) { setExpandedId(null); setStats(null); return }
    setExpandedId(userId); setStatsLoading(true); setError('')
    try {
      const data = await adminFetch<UserStats>('GET', `/users/${userId}/stats`)
      setStats(data)
    } catch (err) { setError(err instanceof Error ? err.message : 'Failed to load stats') }
    finally { setStatsLoading(false) }
  }

  return (
    <div className="adm-page">
      <div className="adm-header">
        <div className="adm-titleRow">
          <h2 className={`adm-title ${styles.title}`}>Users</h2>
          <span className="adm-count">{total} users</span>
        </div>
      </div>
      {error && <p className="adm-error">{error}</p>}
      {loading ? <p className="adm-loading">Loading users...</p> : (
        <>
          <table className="adm-table">
            <thead><tr><th>ID</th><th>Name</th><th>Email</th><th>JLPT Levels</th><th>Streak</th><th>Created</th></tr></thead>
            <tbody>
              {items.length === 0 ? <tr><td colSpan={6} className="adm-empty">No users found</td></tr> :
                items.map(u => (
                  <>
                    <tr key={u.id} className="adm-clickableRow" onClick={() => handleToggleExpand(u.id)}>
                      <td className="adm-id">{u.id}</td>
                      <td className="adm-userName">{u.name}</td>
                      <td className="adm-userEmail">{u.email}</td>
                      <td>
                        <span className="adm-levelList">
                          {(Array.isArray(u.jlpt_levels) ? u.jlpt_levels : []).map((lvl: string) => (
                            <span key={lvl} className={`adm-levelBadge ${LVL_CSS[lvl] || ''}`}>{lvl}</span>
                          ))}
                        </span>
                      </td>
                      <td>🔥 {u.streak_days}</td>
                      <td className="adm-dateCell">{typeof u.created_at === 'string' ? u.created_at.slice(0, 10) : String(u.created_at)}</td>
                    </tr>
                    {expandedId === u.id && (
                      <tr className="adm-expandedRow" key={`${u.id}-stats`}>
                        <td colSpan={6}>
                          {statsLoading ? <p className="adm-loading">Loading stats...</p> : stats ? (
                            <div>
                              <div className="adm-streakBadge">🔥 Streak: {stats.streak_days} days</div>
                              <div className="adm-statsGrid">
                                {Object.entries(stats.modules).map(([mod, stat]) => (
                                  <div key={mod} className="adm-statCard">
                                    <h4>{MODULE_ICONS[mod] || ''} {MODULE_LABELS[mod] || mod}</h4>
                                    <div className="adm-statRow"><span>Due</span><span>{stat.due_count}</span></div>
                                    <div className="adm-statRow"><span>Mastered</span><span>{stat.mastered_count}</span></div>
                                    <div className="adm-statRow"><span>Total</span><span>{stat.total_count}</span></div>
                                    <div className="adm-statRow"><span>Today</span><span>{stat.today_completed} / {stat.daily_goal}</span></div>
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
              }
            </tbody>
          </table>
          <div className="adm-pagination">
            <button disabled={page <= 1} onClick={() => setPage(p => p - 1)}>← Prev</button>
            <span className="adm-pageInfo">Page <strong>{page}</strong> / <strong>{totalPages || 1}</strong> <span style={{marginLeft:8,color:'#b8b0a8'}}>·</span> Total <strong>{total}</strong></span>
            <button disabled={page >= totalPages} onClick={() => setPage(p => p + 1)}>Next →</button>
          </div>
        </>
      )}
    </div>
  )
}
