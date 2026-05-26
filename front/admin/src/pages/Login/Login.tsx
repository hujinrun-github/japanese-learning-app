import { type FormEvent, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import styles from './Login.module.css'

export default function LoginPage() {
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const navigate = useNavigate()

  function handleSubmit(e: FormEvent) {
    e.preventDefault()
    if (!password.trim()) {
      setError('Password is required')
      return
    }
    sessionStorage.setItem('admin_token', password)
    navigate('/words')
  }

  return (
    <div className={styles.wrapper}>
      <form className={styles.card} onSubmit={handleSubmit}>
        <h1 className={styles.title}>Admin Login</h1>
        {error && <p className={styles.error}>{error}</p>}
        <input
          className={styles.input}
          type="password"
          placeholder="Enter admin token"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
        />
        <button className={styles.submitBtn} type="submit">
          Login
        </button>
      </form>
    </div>
  )
}
