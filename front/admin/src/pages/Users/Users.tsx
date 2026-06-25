import { Fragment, useState, useEffect, useCallback } from 'react'
import type { MouseEvent } from 'react'
import { adminFetch } from '@/api/client'
import Modal from '@/components/Modal/Modal'
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

const emptyPasswordForm = { newPassword: '', confirmPassword: '' }

export default function UsersPage() {
  const [items, setItems] = useState<User[]>([])
  const [total, setTotal] = useState(0)
  const [page, setPage] = useState(1)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [expandedId, setExpandedId] = useState<number | null>(null)
  const [stats, setStats] = useState<UserStats | null>(null)
  const [statsLoading, setStatsLoading] = useState(false)
  const [deletingId, setDeletingId] = useState<number | null>(null)
  const [passwordUser, setPasswordUser] = useState<User | null>(null)
  const [passwordForm, setPasswordForm] = useState(emptyPasswordForm)
  const [passwordError, setPasswordError] = useState('')
  const [passwordNotice, setPasswordNotice] = useState('')
  const [savingPassword, setSavingPassword] = useState(false)
  const [showResetPassword, setShowResetPassword] = useState(false)

  const totalPages = Math.ceil(total / PAGE_SIZE)

  const fetchItems = useCallback(async () => {
    setLoading(true); setError('')
    try {
      const params = new URLSearchParams({ page: String(page), size: String(PAGE_SIZE) })
      const data = await adminFetch<{ items: User[]; total: number }>('GET', `/users?${params}`)
      setItems(data.items || []); setTotal(data.total || 0)
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

  async function handleDeleteUser(e: MouseEvent<HTMLButtonElement>, user: User) {
    e.stopPropagation()
    const label = user.email || `ID ${user.id}`
    if (!window.confirm(`Delete user ${label}? This cannot be undone.`)) return

    setDeletingId(user.id)
    setError('')
    try {
      await adminFetch<void>('DELETE', `/users/${user.id}`)
      if (expandedId === user.id) {
        setExpandedId(null)
        setStats(null)
      }
      setItems(prev => prev.filter(item => item.id !== user.id))
      setTotal(prev => Math.max(0, prev - 1))
      if (items.length === 1 && page > 1) {
        setPage(p => p - 1)
      } else {
        fetchItems()
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to delete user')
    } finally {
      setDeletingId(null)
    }
  }

  function openPasswordModal(e: MouseEvent<HTMLButtonElement>, user: User) {
    e.stopPropagation()
    setPasswordUser(user)
    setPasswordForm(emptyPasswordForm)
    setPasswordError('')
    setShowResetPassword(false)
  }

  function closePasswordModal() {
    if (savingPassword) return
    setPasswordUser(null)
    setPasswordForm(emptyPasswordForm)
    setPasswordError('')
    setShowResetPassword(false)
  }

  async function handleUpdatePassword() {
    if (!passwordUser) return
    const newPassword = passwordForm.newPassword.trim()
    if (!newPassword) {
      setPasswordError('New password is required')
      return
    }
    if (newPassword !== passwordForm.confirmPassword.trim()) {
      setPasswordError('Passwords do not match')
      return
    }

    setSavingPassword(true)
    setPasswordError('')
    setPasswordNotice('')
    try {
      await adminFetch<void>('PUT', `/users/${passwordUser.id}/password`, { new_password: newPassword })
      setPasswordNotice(`Password updated for ${passwordUser.email}`)
      setPasswordUser(null)
      setPasswordForm(emptyPasswordForm)
      setShowResetPassword(false)
    } catch (err) {
      setPasswordError(err instanceof Error ? err.message : 'Failed to update password')
    } finally {
      setSavingPassword(false)
    }
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
      {passwordNotice && <p className={styles.success}>{passwordNotice}</p>}
      {loading ? <p className="adm-loading">Loading users...</p> : (
        <>
          <table className="adm-table">
            <thead><tr><th>ID</th><th>Name</th><th>Email</th><th>JLPT Levels</th><th>Streak</th><th>Created</th><th>Actions</th></tr></thead>
            <tbody>
              {items.length === 0 ? <tr><td colSpan={7} className="adm-empty">No users found</td></tr> :
                items.map(u => (
                  <Fragment key={u.id}>
                    <tr className="adm-clickableRow" onClick={() => handleToggleExpand(u.id)}>
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
                      <td>
                        <div className="adm-actions">
                          <button
                            type="button"
                            title={`Reset password for ${u.email}`}
                            onClick={(e) => openPasswordModal(e, u)}
                          >
                            Reset Password
                          </button>
                          <button
                            type="button"
                            className="adm-btnDanger"
                            disabled={deletingId === u.id}
                            title={`Delete ${u.email}`}
                            onClick={(e) => handleDeleteUser(e, u)}
                          >
                            {deletingId === u.id ? 'Deleting...' : 'Delete'}
                          </button>
                        </div>
                      </td>
                    </tr>
                    {expandedId === u.id && (
                      <tr className="adm-expandedRow" key={`${u.id}-stats`}>
                        <td colSpan={7}>
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
                  </Fragment>
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
      <Modal
        open={passwordUser !== null}
        title={passwordUser ? `Reset Password: ${passwordUser.email}` : 'Reset Password'}
        onClose={closePasswordModal}
      >
        <div className="adm-form">
          <p className={styles.passwordHint}>
            Set a new learner login password. The user can sign in with this password immediately.
          </p>
          {passwordError && <p className="adm-error">{passwordError}</p>}
          <div className={styles.passwordTools}>
            <button
              type="button"
              className={styles.passwordToggle}
              onClick={() => setShowResetPassword((visible) => !visible)}
              aria-pressed={showResetPassword}
              aria-label={showResetPassword ? 'Hide reset password' : 'Show reset password'}
            >
              {showResetPassword ? 'Hide Password' : 'Show Password'}
            </button>
          </div>
          <label>
            New Password
            <input
              type={showResetPassword ? 'text' : 'password'}
              value={passwordForm.newPassword}
              onChange={(e) => setPasswordForm((current) => ({ ...current, newPassword: e.target.value }))}
              autoComplete="new-password"
            />
          </label>
          <label>
            Confirm Password
            <input
              type={showResetPassword ? 'text' : 'password'}
              value={passwordForm.confirmPassword}
              onChange={(e) => setPasswordForm((current) => ({ ...current, confirmPassword: e.target.value }))}
              autoComplete="new-password"
            />
          </label>
          <button className="adm-saveBtn" onClick={handleUpdatePassword} disabled={savingPassword}>
            {savingPassword ? 'Saving...' : 'Update Password'}
          </button>
        </div>
      </Modal>
    </div>
  )
}
