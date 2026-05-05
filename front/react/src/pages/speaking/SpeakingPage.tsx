import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { StatusBadge } from '@/components/ui/StatusBadge'
import type { SpeakingRecord } from '@/types/api'
import styles from './SpeakingPage.module.css'

export function SpeakingPage() {
  const { t } = useTranslation()
  const [records, setRecords] = useState<SpeakingRecord[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  useEffect(() => {
    loadRecords()
  }, [])

  async function loadRecords() {
    setLoading(true)
    setError('')
    try {
      const data = await apiFetch<SpeakingRecord[]>('GET', '/api/v1/speaking/records')
      setRecords(data ?? [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }

  function formatDate(iso: string) {
    return new Date(iso).toLocaleDateString()
  }

  return (
    <div className={styles.page}>
      <h1 className={styles.title}>{t('speaking.title')}</h1>

      {/* Intro card */}
      <div className={styles.introCard}>
        <p className={styles.introDesc}>{t('speaking.desc')}</p>
        <div className={styles.howtoTitle}>{t('speaking.howto')}</div>
        <div className={styles.methodList}>
          <div className={styles.method}>
            <span className={styles.methodName}>{t('speaking.shadow')}</span>
            <span className={styles.methodDesc}>{t('speaking.shadowDesc')}</span>
          </div>
          <div className={styles.method}>
            <span className={styles.methodName}>{t('speaking.free')}</span>
            <span className={styles.methodDesc}>{t('speaking.freeDesc')}</span>
          </div>
        </div>
        <p className={styles.micNote}>{t('speaking.micNote')}</p>
      </div>

      {/* History */}
      <div className={styles.sectionTitle}>{t('speaking.records.title')}</div>

      {error && <p style={{ color: 'var(--color-error)' }}>{error}</p>}

      {loading ? (
        <div style={{ display: 'flex', justifyContent: 'center', paddingTop: '40px' }}>
          <Spinner size="lg" />
        </div>
      ) : records.length === 0 ? (
        <EmptyState icon="🎙️" title={t('speaking.records.empty')} description="" />
      ) : (
        <div className={styles.recordList}>
          {records.map((rec) => (
            <div key={rec.id} className={styles.recordItem}>
              <span className={styles.recordDate}>{formatDate(rec.practiced_at)}</span>
              <span className={styles.scoreBadge}>
                {t('speaking.score')} {rec.score.overall_score}
              </span>
              <StatusBadge status={rec.score.overall_score >= 80 ? 'pass' : 'needs_work'} />
            </div>
          ))}
        </div>
      )}
    </div>
  )
}
