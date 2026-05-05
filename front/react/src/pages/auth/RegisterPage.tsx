import { useState, type FormEvent } from 'react'
import { Link, Navigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/contexts/AuthContext'
import { apiFetch } from '@/api/client'
import { Button } from '@/components/ui/Button'
import { EyeIcon } from './EyeIcon'
import styles from './AuthPage.module.css'

interface RegisterResponse {
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

export function RegisterPage() {
  const { isAuthenticated, login } = useAuth()
  const { t } = useTranslation()
  const [name, setName] = useState('')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [showConfirm, setShowConfirm] = useState(false)
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  if (isAuthenticated) return <Navigate to="/" replace />

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    if (password !== confirm) {
      setError(t('auth.register.passwordMismatch'))
      return
    }
    setLoading(true)
    try {
      const res = await apiFetch<RegisterResponse>('POST', '/api/v1/auth/register', { name, email, password })
      login(res.token, res.user)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('auth.register.error'))
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
          <p className={styles.subtitle}>{t('auth.register.title')}</p>
        </div>

        <form onSubmit={handleSubmit} className={styles.form}>
          {error && <div className={styles.errorBox}>{error}</div>}

          <div className={styles.field}>
            <label htmlFor="name" className={styles.label}>{t('auth.register.name')}</label>
            <input
              id="name"
              type="text"
              value={name}
              onChange={(e) => setName(e.target.value)}
              placeholder={t('auth.register.namePlaceholder')}
              required
            />
          </div>

          <div className={styles.field}>
            <label htmlFor="email" className={styles.label}>{t('auth.register.email')}</label>
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
            <label htmlFor="password" className={styles.label}>{t('auth.register.password')}</label>
            <div className={styles.passwordWrapper}>
              <input
                id="password"
                type={showPassword ? 'text' : 'password'}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
                placeholder="••••••••"
                required
                minLength={8}
                autoComplete="new-password"
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

          <div className={styles.field}>
            <label htmlFor="confirm" className={styles.label}>{t('auth.register.confirmPassword')}</label>
            <div className={styles.passwordWrapper}>
              <input
                id="confirm"
                type={showConfirm ? 'text' : 'password'}
                value={confirm}
                onChange={(e) => setConfirm(e.target.value)}
                placeholder="••••••••"
                required
                autoComplete="new-password"
              />
              <button
                type="button"
                className={styles.eyeButton}
                onClick={() => setShowConfirm((v) => !v)}
                aria-label={showConfirm ? t('password.hide') : t('password.show')}
              >
                <EyeIcon visible={showConfirm} />
              </button>
            </div>
          </div>

          <Button type="submit" size="lg" loading={loading} className={styles.submitBtn}>
            {t('auth.register.submit')}
          </Button>
        </form>

        <p className={styles.footer}>
          {t('auth.register.hasAccount')}{' '}
          <Link to="/login">{t('auth.register.loginLink')}</Link>
        </p>
      </div>
    </div>
  )
}
