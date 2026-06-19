import { useEffect, useState } from 'react'
import { useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Badge } from '@/components/ui/Badge'
import { EmptyState } from '@/components/ui/EmptyState'
import { Spinner } from '@/components/ui/Spinner'
import type { LessonSummary } from '@/types/api'
import styles from './LessonPage.module.css'

export function LessonListPage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [summaries, setSummaries] = useState<LessonSummary[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [readSet, setReadSet] = useState<Set<number>>(new Set())

  useEffect(() => {
    const userRaw = localStorage.getItem('user')
    if (!userRaw) return
    try {
      const user = JSON.parse(userRaw) as { id?: number }
      if (!user.id) return
      const ids = new Set<number>()
      for (let i = 0; i < localStorage.length; i++) {
        const key = localStorage.key(i)
        if (key?.startsWith(`lesson_read_${user.id}_`)) {
          const lessonId = Number(key.replace(`lesson_read_${user.id}_`, ''))
          if (!isNaN(lessonId)) ids.add(lessonId)
        }
      }
      setReadSet(ids)
    } catch {
      // Ignore malformed legacy localStorage.
    }
  }, [])

  useEffect(() => {
    async function loadLessons() {
      setLoading(true)
      setError('')
      try {
        const data = await apiFetch<LessonSummary[]>('GET', '/api/v1/lessons')
        setSummaries(data ?? [])
      } catch (err) {
        setError(err instanceof Error ? err.message : 'Failed to load')
      } finally {
        setLoading(false)
      }
    }

    void loadLessons()
  }, [])

  if (loading) {
    return (
      <div className={styles.page} style={{ display: 'flex', justifyContent: 'center', paddingTop: '80px' }}>
        <Spinner size="lg" />
      </div>
    )
  }

  return (
    <div className={styles.page}>
      <h1 className={styles.title}>{t('lesson.title')}</h1>

      {error && <p style={{ color: 'var(--color-error)' }}>{error}</p>}

      {summaries.length === 0 ? (
        <EmptyState icon="📖" title="No lessons available" description="" />
      ) : (
        <div className={styles.list}>
          {summaries.map((lesson) => (
            <button
              key={lesson.id}
              className={styles.lessonItem}
              onClick={() => navigate(`/lesson/${lesson.id}`)}
            >
              <div className={styles.lessonLeft}>
                <div className={styles.lessonTitle}>
                  {readSet.has(lesson.id) && (
                    <span className={styles.readMark} title="read">✓</span>
                  )}
                  {lesson.title}
                </div>
                <div className={styles.lessonMeta}>
                  <span className={styles.charCount}>
                    {t('lesson.chars', { count: lesson.char_count })}
                  </span>
                  {lesson.tags?.length > 0 && (
                    <div className={styles.tags}>
                      {lesson.tags.map((tag) => (
                        <span key={tag} className={styles.tag}>{tag}</span>
                      ))}
                    </div>
                  )}
                </div>
              </div>
              <div className={styles.lessonRight}>
                <Badge level={lesson.jlpt_level} size="sm" />
                <span className={styles.arrow}>›</span>
              </div>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}
