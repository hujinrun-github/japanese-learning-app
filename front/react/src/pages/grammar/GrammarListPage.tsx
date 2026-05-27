import { useState, useEffect } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Badge } from '@/components/ui/Badge'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import type { GrammarPointWithStatus, JLPTLevel } from '@/types/api'
import styles from './GrammarListPage.module.css'

const LEVELS: JLPTLevel[] = ['N5', 'N4', 'N3', 'N2', 'N1']

type FilterKey = '' | 'learning' | 'mastered' | 'unlearned'

const FILTERS: { key: FilterKey; labelKey: string }[] = [
  { key: '', labelKey: 'grammar.list.filter.all' },
  { key: 'learning', labelKey: 'grammar.list.filter.learning' },
  { key: 'mastered', labelKey: 'grammar.list.filter.mastered' },
  { key: 'unlearned', labelKey: 'grammar.list.filter.unlearned' },
]

function filterPoints(items: GrammarPointWithStatus[], f: FilterKey): GrammarPointWithStatus[] {
  if (f === 'learning') return items.filter((p) => p.user_status === 'learning')
  if (f === 'mastered') return items.filter((p) => p.user_status === 'mastered')
  if (f === 'unlearned') return items.filter((p) => p.user_status === 'unlearned')
  return items
}

export function GrammarListPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [level, setLevel] = useState<JLPTLevel>('N5')
  const [filter, setFilter] = useState<FilterKey>('')
  const [points, setPoints] = useState<GrammarPointWithStatus[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    loadPoints(level)
  }, [level])

  async function loadPoints(lv: JLPTLevel) {
    setLoading(true)
    setError('')
    try {
      const data = await apiFetch<GrammarPointWithStatus[]>('GET', `/api/v1/grammar?level=${lv}`)
      setPoints(data ?? [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.title}>{t('grammar.list.title')}</h1>
      </div>

      <div className={styles.tabs}>
        {LEVELS.map((lv) => (
          <button
            key={lv}
            className={`${styles.tab} ${level === lv ? styles.tabActive : ''}`}
            onClick={() => setLevel(lv)}
          >
            {lv}
          </button>
        ))}
      </div>

      {/* filter bar */}
      <div className={styles.filterBar}>
        {FILTERS.map((f) => (
          <button
            key={f.key}
            className={`${styles.filterBtn} ${filter === f.key ? styles.filterBtnActive : ''}`}
            onClick={() => setFilter(f.key)}
          >
            {t(f.labelKey)}
          </button>
        ))}
      </div>

      {error && <p style={{ color: 'var(--color-error)' }}>{error}</p>}

      {loading ? (
        <div style={{ display: 'flex', justifyContent: 'center', paddingTop: '60px' }}>
          <Spinner size="lg" />
        </div>
      ) : (() => {
        const filtered = filterPoints(points, filter)
        return filtered.length === 0 ? (
          <EmptyState icon="📝" title={t('grammar.list.empty')} description="" />
        ) : (
          <div className={styles.list}>
            {filtered.map((p) => (
            <button key={p.id} className={styles.item} onClick={() => navigate(`/grammar/${p.id}`)}>
              <div className={styles.itemLeft}>
                <div className={styles.itemName}>{p.name}</div>
                <div className={styles.itemMeaning}>{p.meaning}</div>
              </div>
              <div className={styles.itemRight}>
                <StatusBadge status={p.user_status} />
                {p.last_quiz_score >= 0 && p.last_quiz_score < 100 && (
                  <span className={styles.quizFailBadge} title={`Last quiz: ${p.last_quiz_score}/100`}>
                    {p.last_quiz_score}
                  </span>
                )}
                <Badge level={p.jlpt_level} size="sm" />
                <span className={styles.arrow}>›</span>
              </div>
            </button>
          ))}
        </div>
      )
      })()}
    </div>
  )
}
