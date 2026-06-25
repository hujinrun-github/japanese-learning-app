import { type FormEvent, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import styles from './Login.module.css'

export default function LoginPage() {
  const [password, setPassword] = useState('')
  const [showToken, setShowToken] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)
  const navigate = useNavigate()

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!password.trim()) {
      setError('Password is required')
      return
    }
    setLoading(true)
    setError('')
    try {
      const res = await fetch('/api/admin/words?size=1', {
        headers: { 'Authorization': `Bearer ${password}` },
      })
      if (!res.ok) {
        setError('Invalid token')
        return
      }
      sessionStorage.setItem('admin_token', password)
      navigate('/words')
    } catch {
      setError('Cannot connect to admin server')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className={styles.wrapper}>
      <form className={styles.card} onSubmit={handleSubmit}>
        <h1 className={styles.title}>Admin Login</h1>
        {error && <p className={styles.error}>{error}</p>}
        <div className={styles.tokenField}>
          <input
            className={styles.input}
            type={showToken ? 'text' : 'password'}
            placeholder="Enter admin token"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
          <button
            className={styles.tokenToggle}
            type="button"
            onClick={() => setShowToken((current) => !current)}
          >
            {showToken ? 'Hide' : 'Show'}
          </button>
        </div>
        <button className={styles.submitBtn} type="submit" disabled={loading}>
          {loading ? 'Verifying...' : 'Login'}
        </button>
      </form>
    </div>
  )
}
