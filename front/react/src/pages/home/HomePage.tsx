import { useEffect, useState, type CSSProperties } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { useAuth } from '@/contexts/AuthContext'
import { apiFetch } from '@/api/client'
import { Badge } from '@/components/ui/Badge'
import { ProgressBar } from '@/components/ui/ProgressBar'
import { Spinner } from '@/components/ui/Spinner'
import type { JLPTLevel, ModuleStat, UserStats } from '@/types/api'
import { getDailyProgress, getMasteryProgress, hasDailyGoal, percent } from './progress'
import styles from './HomePage.module.css'

const MODULE_CONFIG = [
  { key: 'word', labelKey: 'home.modules.word', descKey: 'home.moduleDesc.word', icon: '📖', to: '/words/review', tone: 'blue' },
  { key: 'grammar', labelKey: 'home.modules.grammar', descKey: 'home.moduleDesc.grammar', icon: '📗', to: '/grammar', tone: 'green' },
  { key: 'lesson', labelKey: 'home.modules.lesson', descKey: 'home.moduleDesc.lesson', icon: '📚', to: '/lesson', tone: 'coral' },
  { key: 'speaking', labelKey: 'home.modules.speaking', descKey: 'home.moduleDesc.speaking', icon: '🎙', to: '/speaking', tone: 'violet' },
  { key: 'writing', labelKey: 'home.modules.writing', descKey: 'home.moduleDesc.writing', icon: '✍️', to: '/writing', tone: 'amber' },
  { key: 'translation', labelKey: 'home.modules.translation', descKey: 'home.moduleDesc.translation', icon: '文', to: '/translation', tone: 'teal' },
  { key: 'note', labelKey: 'home.modules.notes', descKey: 'home.moduleDesc.notes', icon: '📝', to: '/notes', tone: 'blue' },
] as const

const REVIEW_ITEMS = [
  { key: 'word', icon: 'A', to: '/words/review', labelKey: 'home.reviewQueue.word', detailKey: 'home.reviewQueue.wordDetail' },
  { key: 'grammar', icon: '文', to: '/grammar', labelKey: 'home.reviewQueue.grammar', detailKey: 'home.reviewQueue.grammarDetail' },
  { key: 'writing', icon: '✍️', to: '/writing', labelKey: 'home.reviewQueue.writing', detailKey: 'home.reviewQueue.writingDetail' },
  { key: 'speaking', icon: '🎧', to: '/speaking', labelKey: 'home.reviewQueue.speaking', detailKey: 'home.reviewQueue.speakingDetail' },
] as const

const EMPTY_STATS: UserStats = {
  streak_days: 0,
  modules: {},
}

const EMPTY_MODULE_STAT: ModuleStat = {
  due_count: 0,
  mastered_count: 0,
  total_count: 0,
  today_completed: 0,
  daily_goal: 0,
}

const VALID_JLPT_LEVELS: JLPTLevel[] = ['N5', 'N4', 'N3', 'N2', 'N1']

function getModuleStat(stats: UserStats, key: string): ModuleStat {
  return stats.modules[key] ?? EMPTY_MODULE_STAT
}

function primaryJLPTLevel(levels: string[] | undefined): JLPTLevel {
  const level = levels?.find(item => VALID_JLPT_LEVELS.includes(item as JLPTLevel))
  return (level ?? 'N5') as JLPTLevel
}

export function HomePage() {
  const { user } = useAuth()
  const { t } = useTranslation()
  const [stats, setStats] = useState<UserStats>(EMPTY_STATS)
  const [loading, setLoading] = useState(true)
  const jlptLevel = primaryJLPTLevel(user?.jlpt_levels)
  const primaryStats = ['word', 'grammar', 'speaking', 'writing'].map(key => getModuleStat(stats, key))
  const totalGoal = primaryStats.reduce((sum, item) => sum + item.daily_goal, 0)
  const totalCompleted = primaryStats.reduce((sum, item) => sum + item.today_completed, 0)
  const totalDue = primaryStats.reduce((sum, item) => sum + item.due_count, 0)
  const totalMastered = primaryStats.reduce((sum, item) => sum + item.mastered_count, 0)
  const totalKnown = primaryStats.reduce((sum, item) => sum + item.total_count, 0)
  const todayPercent = percent(totalCompleted, totalGoal)
  const jlptPercent = percent(totalMastered, totalKnown)
  const wordStat = getModuleStat(stats, 'word')
  const nextModule = totalDue > 0 ? MODULE_CONFIG[0] : MODULE_CONFIG[2]
  const weakModule = MODULE_CONFIG
    .slice(0, 4)
    .map(item => ({ ...item, stat: getModuleStat(stats, item.key) }))
    .filter(item => item.stat.total_count > 0)
    .sort((a, b) => percent(a.stat.mastered_count, a.stat.total_count) - percent(b.stat.mastered_count, b.stat.total_count))[0]
    ?? MODULE_CONFIG[1]

  useEffect(() => {
    let cancelled = false
    apiFetch<UserStats>('GET', '/api/v1/users/stats')
      .then(s => { if (!cancelled) setStats(s ?? EMPTY_STATS) })
      .catch(() => {})
      .finally(() => { if (!cancelled) setLoading(false) })
    return () => { cancelled = true }
  }, [])

  return (
    <div className={styles.page}>
      <section className={styles.dashboard}>
        <div className={styles.mainColumn}>
          <section className={styles.hero}>
            <div className={styles.heroIntro}>
              <p className={styles.greeting}>{t('home.greeting')}</p>
              <h1 className={styles.heroName}>{t('home.welcomeName', { name: user?.name ?? t('home.guest') })}</h1>
              <p className={styles.heroSub}>{t('home.heroSub')}</p>
            </div>

            <div className={styles.statStrip}>
              <div className={styles.statTile}>
                <span className={styles.statIcon}>目</span>
                <span className={styles.statLabel}>{t('home.currentLevel')}</span>
                <strong><Badge level={jlptLevel} size="md" /></strong>
              </div>
              <div className={styles.statTile}>
                <span className={styles.statIcon}>🔥</span>
                <span className={styles.statLabel}>{t('home.studyStreak')}</span>
                <strong>{t('home.streakDays', { count: stats.streak_days })}</strong>
              </div>
              <div className={styles.statTile}>
                <span className={styles.statIcon}>🎯</span>
                <span className={styles.statLabel}>{t('home.todayGoal')}</span>
                <strong>{t('home.wordsCount', { count: totalGoal || wordStat.daily_goal || 0 })}</strong>
              </div>
            </div>

            <div className={styles.primaryAction}>
              <div>
                <h2>{t('home.startReview')}</h2>
                <p>{t('home.dueWords', { count: wordStat.due_count })}</p>
                <div className={styles.heroProgress} aria-hidden="true">
                  <span style={{ width: `${todayPercent}%` }} />
                </div>
              </div>
              <Link to={nextModule.to} className={styles.primaryButton}>
                {t('home.startNow')} <span>→</span>
              </Link>
            </div>
          </section>

          <section className={styles.section}>
            <div className={styles.sectionHeader}>
              <h2>{t('home.learningMap')}</h2>
              <span>{t('home.todayProgress', { completed: totalCompleted, goal: totalGoal })}</span>
            </div>
            {loading ? (
              <div className={styles.loading}>
                <Spinner size="md" />
              </div>
            ) : (
              <div className={styles.moduleGrid}>
                {MODULE_CONFIG.map(mod => {
                  const s = getModuleStat(stats, mod.key)
                  const dailyProgress = getDailyProgress(s)
                  const masteryProgress = getMasteryProgress(s)
                  const progress = hasDailyGoal(s) ? dailyProgress : masteryProgress
                  const secondaryMeta = s.due_count > 0
                    ? t('home.dueShort', { count: s.due_count })
                    : s.total_count > 0
                      ? `${masteryProgress}%`
                      : ''
                  return (
                    <Link key={mod.key} to={mod.to} className={`${styles.moduleCard} ${styles[mod.tone]}`}>
                      <span className={styles.moduleIcon}>{mod.icon}</span>
                      <div className={styles.moduleContent}>
                        <strong>{t(mod.labelKey)}</strong>
                        <span>{t(mod.descKey)}</span>
                      </div>
                      <div className={styles.moduleMeta}>
                        <strong>{t('home.todayProgress', { completed: s.today_completed, goal: s.daily_goal })}</strong>
                        {secondaryMeta ? <span>{secondaryMeta}</span> : null}
                      </div>
                      <div
                        className={styles.moduleProgress}
                        aria-label={t('home.todayProgress', { completed: s.today_completed, goal: s.daily_goal })}
                      >
                        <span style={{ width: `${progress}%` }} />
                      </div>
                    </Link>
                  )
                })}
              </div>
            )}
          </section>

          <section className={styles.practicePanel}>
            <div>
              <div className={styles.sectionHeader}>
                <h2>{t('home.readingDrill')}</h2>
                <Badge level={jlptLevel} size="sm" />
              </div>
              <p className={styles.japaneseLine}>
                この<ruby>本<rt>ほん</rt></ruby>はとてもおもしろくて、
                <ruby>一日<rt>いちにち</rt></ruby>で
                <ruby>全部<rt>ぜんぶ</rt></ruby>
                <ruby>読み<rt>よみ</rt></ruby>
                <ruby>終えました<rt>おえました</rt></ruby>。
              </p>
              <p className={styles.translation}>{t('home.readingTranslation')}</p>
            </div>
            <div className={styles.practiceActions}>
              <Link to="/lesson" className={styles.secondaryButton}>{t('home.showAnalysis')}</Link>
              <Link to="/notes/new" className={styles.secondaryButton}>{t('home.addToNotes')}</Link>
            </div>
          </section>

          <section className={styles.tipCard}>
            <span className={styles.tipIcon}>💡</span>
            <div>
              <h2>{t('home.tips')}</h2>
              <p>{t('home.tipText')}</p>
            </div>
            <Link to="/words/review" className={styles.tipLink}>{t('home.viewReviewPlan')} →</Link>
          </section>
        </div>

        <aside className={styles.sideColumn}>
          <section className={styles.sideCard}>
            <h2>{t('home.reviewQueue.title')}</h2>
            <div className={styles.queueList}>
              {REVIEW_ITEMS.map((item, index) => {
                const s = getModuleStat(stats, item.key)
                return (
                  <Link key={item.key} to={item.to} className={styles.queueItem}>
                    <span className={`${styles.queueIcon} ${styles[`queueTone${index}`]}`}>{item.icon}</span>
                    <span>
                      <strong>{t(item.labelKey)}</strong>
                      <small>{t(item.detailKey, { count: s.due_count || s.today_completed || 0 })}</small>
                    </span>
                    <em>{t('home.start')}</em>
                  </Link>
                )
              })}
            </div>
          </section>

          <section className={styles.sideCard}>
            <div className={styles.sideHeader}>
              <h2>{t('home.jlptProgress')}</h2>
              <Badge level={jlptLevel} size="sm" />
            </div>
            <div className={styles.progressSummary}>
              <div className={styles.ring} style={{ '--progress': `${jlptPercent * 3.6}deg` } as CSSProperties}>
                <strong>{jlptPercent}%</strong>
                <span>{t('home.totalProgress')}</span>
              </div>
              <div className={styles.progressRows}>
                {MODULE_CONFIG.slice(0, 4).map(mod => {
                  const s = getModuleStat(stats, mod.key)
                  const value = s.total_count > 0 ? percent(s.mastered_count, s.total_count) : 0
                  return (
                    <div key={mod.key} className={styles.progressRow}>
                      <span>{t(mod.labelKey)}</span>
                      <ProgressBar value={value} />
                      <strong>{value}%</strong>
                    </div>
                  )
                })}
              </div>
            </div>
          </section>

          <section className={styles.sideCard}>
            <div className={styles.sideHeader}>
              <h2>{t('home.weakness.title')}</h2>
              <span>{t('home.weakness.basedOn')}</span>
            </div>
            <div className={styles.weakList}>
              <Link to={weakModule.to} className={styles.weakItem}>
                <span>{t(weakModule.labelKey)}</span>
                <strong>{t('home.weakness.priority')}</strong>
              </Link>
              <Link to="/grammar" className={styles.weakItem}>
                <span>{t('home.weakness.grammarPoint')}</span>
                <strong>{t('home.weakness.study')}</strong>
              </Link>
            </div>
          </section>

          <section className={styles.sideCard}>
            <div className={styles.sideHeader}>
              <h2>{t('home.calendar.title')}</h2>
              <span>{t('home.streakDays', { count: stats.streak_days })}</span>
            </div>
            <div className={styles.calendarGrid} aria-label={t('home.calendar.title')}>
              {['一', '二', '三', '四', '五', '六', '日'].map(day => <span key={day}>{day}</span>)}
              {Array.from({ length: 7 }).map((_, index) => (
                <strong key={index} className={index < Math.min(7, stats.streak_days) ? styles.checkedDay : ''}>
                  {index < Math.min(7, stats.streak_days) ? '✓' : '·'}
                </strong>
              ))}
            </div>
            <p className={styles.calendarNote}>{t('home.calendar.note', { count: totalCompleted })}</p>
          </section>
        </aside>
      </section>
    </div>
  )
}
