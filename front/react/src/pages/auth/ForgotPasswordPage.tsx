import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Button } from '@/components/ui/Button'
import styles from './AuthPage.module.css'

export function ForgotPasswordPage() {
  const { t } = useTranslation()
  const [email, setEmail] = useState('')
  const [error, setError] = useState('')
  const [success, setSuccess] = useState(false)
  const [loading, setLoading] = useState(false)

  async function handleSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    setLoading(true)
    try {
      await apiFetch('POST', '/api/v1/auth/forgot-password', { email })
      setSuccess(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('auth.forgot.error'))
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className={styles.page}>
      <div className={styles.card}>
        <div className={styles.header}>
          <span className={styles.logo}>🔑</span>
          <h1 className={styles.title}>{t('common.appName')}</h1>
          <p className={styles.subtitle}>{t('auth.forgot.title')}</p>
        </div>

        {success ? (
          <div className={styles.successBox}>
            {t('auth.forgot.success')}
          </div>
        ) : (
          <form onSubmit={handleSubmit} className={styles.form}>
            {error && <div className={styles.errorBox}>{error}</div>}

            <p className={styles.hint}>
              {t('auth.forgot.hint')}
            </p>

            <div className={styles.field}>
              <label htmlFor="email" className={styles.label}>{t('auth.forgot.email')}</label>
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

            <Button type="submit" size="lg" loading={loading} className={styles.submitBtn}>
              {t('auth.forgot.submit')}
            </Button>
          </form>
        )}

        <p className={styles.footer}>
          <Link to="/login">{t('auth.forgot.backToLogin')}</Link>
        </p>
      </div>
    </div>
  )
}
