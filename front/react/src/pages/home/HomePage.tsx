import { useState, useEffect } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/contexts/AuthContext'
import { apiFetch } from '@/api/client'
import { Card } from '@/components/ui/Card'
import { Badge } from '@/components/ui/Badge'
import { ProgressBar } from '@/components/ui/ProgressBar'
import { Spinner } from '@/components/ui/Spinner'
import type { JLPTLevel, UserStats } from '@/types/api'
import styles from './HomePage.module.css'

const MODULE_CONFIG = [
  { key: 'word',     labelKey: 'home.modules.word',     icon: '📖', to: '/words/review' },
  { key: 'grammar',  labelKey: 'home.modules.grammar',  icon: '📝', to: '/grammar' },
  { key: 'speaking', labelKey: 'home.modules.speaking', icon: '🎙️', to: '/speaking' },
  { key: 'writing',  labelKey: 'home.modules.writing',  icon: '✏️', to: '/writing' },
]

const EMPTY_STATS: UserStats = {
  streak_days: 0,
  modules: {},
}

export function HomePage() {
  const { user } = useAuth()
  const { t } = useTranslation()
  const [stats, setStats] = useState<UserStats>(EMPTY_STATS)
  const [loading, setLoading] = useState(true)
  const jlptLevel = (user?.jlpt_level ?? 'N5') as JLPTLevel

  useEffect(() => {
    let cancelled = false
    apiFetch<UserStats>('GET', '/api/v1/users/stats').then((s) => {
      if (!cancelled) setStats(s ?? EMPTY_STATS)
    }).catch(() => {}).finally(() => {
      if (!cancelled) setLoading(false)
    })
    return () => { cancelled = true }
  }, [])

  return (
    <div className={styles.page}>
      {/* Header */}
      <section className={styles.hero}>
        <div className={styles.heroLeft}>
          <p className={styles.greeting}>{t('home.greeting')}</p>
          <h1 className={styles.heroName}>{user?.name ?? t('home.guest')} さん</h1>
          <div className={styles.heroBadges}>
            <Badge level={jlptLevel} size="md" />
            <span className={styles.streak}>🔥 {t('home.streakDays', { count: stats.streak_days })}</span>
          </div>
        </div>
      </section>

      {/* Today's tasks */}
      <section className={styles.section}>
        <h2 className={styles.sectionTitle}>{t('home.todaysTasks')}</h2>
        {loading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: 'var(--space-8) 0' }}>
            <Spinner size="md" />
          </div>
        ) : (
          <div className={styles.moduleGrid}>
            {MODULE_CONFIG.map((mod) => {
              const s = stats.modules[mod.key] ?? { due_count: 0, mastered_count: 0, total_count: 0 }
              const pct = s.total_count > 0 ? Math.round((s.mastered_count / s.total_count) * 100) : 0
              return (
                <Link key={mod.key} to={mod.to} className={styles.moduleLink}>
                  <Card hoverable padding="md" className={styles.moduleCard}>
                    <div className={styles.moduleHeader}>
                      <span className={styles.moduleIcon}>{mod.icon}</span>
                      <span className={styles.moduleLabel}>{t(mod.labelKey)}</span>
                      {s.due_count > 0 && (
                        <span className={styles.dueBadge}>{s.due_count}</span>
                      )}
                    </div>
                    <ProgressBar value={pct} label={t('home.mastered', { mastered: s.mastered_count, total: s.total_count })} />
                  </Card>
                </Link>
              )
            })}
          </div>
        )}
      </section>

      {/* Quick tips */}
      <section className={styles.section}>
        <h2 className={styles.sectionTitle}>{t('home.tips')}</h2>
        <Card padding="md" className={styles.tipCard}>
          <p className={styles.tipText}>
            {t('home.tipText')}
          </p>
        </Card>
      </section>
    </div>
  )
}
