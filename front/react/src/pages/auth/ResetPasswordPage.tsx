import { useState, type FormEvent } from 'react'
import { Link, useSearchParams, Navigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Button } from '@/components/ui/Button'
import { EyeIcon } from './EyeIcon'
import styles from './AuthPage.module.css'

export function ResetPasswordPage() {
  const [searchParams] = useSearchParams()
  const token = searchParams.get('token') ?? ''
  const { t } = useTranslation()

  const [newPassword, setNewPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [showPassword, setShowPassword] = useState(false)
  const [showConfirm, setShowConfirm] = useState(false)
  const [error, setError] = useState('')
  const [success, setSuccess] = useState(false)
  const [loading, setLoading] = useState(false)

  // If no token in URL, redirect to forgot-password
  if (!token) return <Navigate to="/forgot-password" replace />

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    if (newPassword !== confirm) {
      setError(t('auth.reset.passwordMismatch'))
      return
    }
    setLoading(true)
    try {
      await apiFetch('POST', '/api/v1/auth/reset-password', { token, new_password: newPassword })
      setSuccess(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('auth.reset.error'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className={styles.page}>
      <div className={styles.card}>
        <div className={styles.header}>
          <span className={styles.logo}>🔒</span>
          <h1 className={styles.title}>{t('common.appName')}</h1>
          <p className={styles.subtitle}>{t('auth.reset.title')}</p>
        </div>

        {success ? (
          <div className={styles.successBox}>
            {t('auth.reset.success')}
            <br />
            <Link to="/login">{t('auth.reset.loginLink')}</Link>
          </div>
        ) : (
          <form onSubmit={handleSubmit} className={styles.form}>
            {error && <div className={styles.errorBox}>{error}</div>}

            <div className={styles.field}>
              <label htmlFor="new-password" className={styles.label}>{t('auth.reset.newPassword')}</label>
              <div className={styles.passwordWrapper}>
                <input
                  id="new-password"
                  type={showPassword ? 'text' : 'password'}
                  value={newPassword}
                  onChange={(e) => setNewPassword(e.target.value)}
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
              <label htmlFor="confirm-password" className={styles.label}>{t('auth.reset.confirmPassword')}</label>
              <div className={styles.passwordWrapper}>
                <input
                  id="confirm-password"
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
              {t('auth.reset.submit')}
            </Button>
          </form>
        )}

        <p className={styles.footer}>
          <Link to="/login">{t('auth.reset.backToLogin')}</Link>
        </p>
      </div>
    </div>
  )
}
