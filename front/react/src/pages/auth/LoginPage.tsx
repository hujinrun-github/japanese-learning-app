import { useState, type FormEvent } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/contexts/AuthContext'
import { apiFetch } from '@/api/client'
import { Button } from '@/components/ui/Button'
import { EyeIcon } from './EyeIcon'
import styles from './AuthPage.module.css'

interface LoginResponse {
  token: string
  user: {
    id: number
    name: string
    email: string
    jlpt_level: 'N5' | 'N4' | 'N3' | 'N2' | 'N1'
    streak_days: number
    created_at: string
  }
}

export function LoginPage() {
  const { isAuthenticated, login } = useAuth()
  const { t } = useTranslation()
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  if (isAuthenticated) return <Navigate to="/" replace />

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      const res = await apiFetch<LoginResponse>('POST', '/api/v1/auth/login', { email, password })
      login(res.token, res.user)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('auth.login.error'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className={styles.page}>
      <div className={styles.card}>
        <div className={styles.header}>
          <span className={styles.logo}>🇯🇵</span>
          <h1 className={styles.title}>{t('common.appName')}</h1>
          <p className={styles.subtitle}>{t('auth.login.title')}</p>
        </div>

        <form onSubmit={handleSubmit} className={styles.form}>
          {error && <div className={styles.errorBox}>{error}</div>}

          <div className={styles.field}>
            <label htmlFor="email" className={styles.label}>{t('auth.login.email')}</label>
            <input
              id="email"
              type="email"
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="example@mail.com"
              required
              autoComplete="email"
            />
          </div>

          <div className={styles.field}>
            <label htmlFor="password" className={styles.label}>{t('auth.login.password')}</label>
            <div className={styles.passwordWrapper}>
              <input
                id="password"
                type={showPassword ? 'text' : 'password'}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••"
                required
                autoComplete="current-password"
              />
              <button
                type="button"
                className={styles.eyeButton}
                onClick={() => setShowPassword((v) => !v)}
                aria-label={showPassword ? t('password.hide') : t('password.show')}
              >
                <EyeIcon visible={showPassword} />
              </button>
            </div>
          </div>

          <Button type="submit" size="lg" loading={loading} className={styles.submitBtn}>
            {t('auth.login.submit')}
          </Button>

          <p className={styles.forgotLink}>
            <Link to="/forgot-password">{t('auth.login.forgotPassword')}</Link>
          </p>
        </form>

        <p className={styles.footer}>
          {t('auth.login.noAccount')}{' '}
          <Link to="/register">{t('auth.login.registerLink')}</Link>
        </p>
      </div>
    </div>
  )
}
