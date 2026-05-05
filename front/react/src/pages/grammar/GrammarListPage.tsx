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

export function GrammarListPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [level, setLevel] = useState<JLPTLevel>('N5')
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

      {error && <p style={{ color: 'var(--color-error)' }}>{error}</p>}

      {loading ? (
        <div style={{ display: 'flex', justifyContent: 'center', paddingTop: '60px' }}>
          <Spinner size="lg" />
        </div>
      ) : points.length === 0 ? (
        <EmptyState icon="📝" title={t('grammar.list.empty')} description="" />
      ) : (
        <div className={styles.list}>
          {points.map((p) => (
            <button key={p.id} className={styles.item} onClick={() => navigate(`/grammar/${p.id}`)}>
              <div className={styles.itemLeft}>
                <div className={styles.itemName}>{p.name}</div>
                <div className={styles.itemMeaning}>{p.meaning}</div>
              </div>
              <div className={styles.itemRight}>
                <StatusBadge status={p.user_status} />
                <Badge level={p.jlpt_level} size="sm" />
                <span className={styles.arrow}>›</span>
              </div>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
