import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/contexts/AuthContext'
import { Card } from '@/components/ui/Card'
import { Badge } from '@/components/ui/Badge'
import { ProgressBar } from '@/components/ui/ProgressBar'
import type { JLPTLevel } from '@/types/api'
import styles from './HomePage.module.css'

// Mock data — will be replaced with useApi when backend is connected
const MOCK_STATS: {
  streak_days: number
  modules: Record<string, { due_count: number; mastered_count: number; total_count: number }>
} = {
  streak_days: 7,
  modules: {
    word:    { due_count: 12, mastered_count: 48, total_count: 200 },
    grammar: { due_count: 3,  mastered_count: 15, total_count: 100 },
    speaking:{ due_count: 0,  mastered_count: 8,  total_count: 50  },
    writing: { due_count: 5,  mastered_count: 20, total_count: 80  },
  },
}

const MODULE_CONFIG = [
  { key: 'word',     labelKey: 'home.modules.word',     icon: '📖', to: '/words/review', color: 'var(--color-n5)' },
  { key: 'grammar',  labelKey: 'home.modules.grammar',  icon: '📝', to: '/grammar',       color: 'var(--color-n4)' },
  { key: 'speaking', labelKey: 'home.modules.speaking', icon: '🎙️', to: '/speaking',      color: 'var(--color-n3)' },
  { key: 'writing',  labelKey: 'home.modules.writing',  icon: '✏️', to: '/writing',       color: 'var(--color-n2)' },
]

export function HomePage() {
  const { user } = useAuth()
  const { t } = useTranslation()
  const stats = MOCK_STATS
  const jlptLevel = (user?.jlpt_level ?? 'N5') as JLPTLevel

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
        <div className={styles.moduleGrid}>
          {MODULE_CONFIG.map((mod) => {
            const s = stats.modules[mod.key]
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
