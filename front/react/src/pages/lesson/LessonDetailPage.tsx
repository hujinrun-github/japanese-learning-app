import { FavoriteButton } from '@/pages/favorites/FavoriteButton'
import { useEffect, useState } from 'react'
import { Link, useLocation, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Badge } from '@/components/ui/Badge'
import { ErrataButton } from '@/components/ui/ErrataButton'
import { Spinner } from '@/components/ui/Spinner'
import type { Lesson } from '@/types/api'
import { LessonReading } from './LessonReading'
import styles from './LessonPage.module.css'

export function LessonDetailPage() {
  const { t } = useTranslation()
  const { id } = useParams()
  const { state, hash } = useLocation()
  const sentenceAnchor = /^#sentence-(?:0|[1-9]\d*)$/.test(hash) ? hash.slice(1) : null
  const listSearch = typeof state?.lessonListSearch === 'string' ? state.lessonListSearch : ''
  const listLocation = { pathname: '/lesson', search: listSearch }
  const [lesson, setLesson] = useState<Lesson | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  useEffect(() => {
    let ignore = false

    async function loadLesson() {
      if (!id) return
      setLoading(true)
      setError('')
      try {
        const data = await apiFetch<Lesson>('GET', `/api/v1/lessons/${id}`)
        if (ignore) return
        setLesson(data)
        markLessonRead(data.id)
      } catch (err) {
        if (!ignore) setError(err instanceof Error ? err.message : 'Failed to load')
      } finally {
        if (!ignore) setLoading(false)
      }
    }

    void loadLesson()
    return () => {
      ignore = true
    }
  }, [id])

  if (loading) {
    return (
      <div className={styles.page} style={{ display: 'flex', justifyContent: 'center', paddingTop: '80px' }}>
        <Spinner size="lg" />
      </div>
    )
  }

  if (error) {
    return (
      <div className={styles.page}>
        <Link className={styles.backBtn} to={listLocation}>
          {t('lesson.back')}
        </Link>
        <p style={{ color: 'var(--color-error)' }}>{error}</p>
      </div>
    )
  }

  if (!lesson) return null

  return (
    <div className={styles.detailPage}>
      <Link className={styles.backBtn} to={listLocation}>
        {t('lesson.back')}
      </Link>

      <header className={styles.detailHeader}>
        <div>
          <h1 className={styles.detailTitle} lang="ja">{lesson.title}</h1>
          <div className={styles.detailMeta}>
            <Badge level={lesson.jlpt_level} size="sm" />
            <span className={styles.charCount}>{t('lesson.chars', { count: lesson.char_count })}</span>
            {lesson.tags?.map((tag) => (
              <span key={tag} className={styles.tag}>{tag}</span>
            ))}
          </div>
        </div>
        <div className={styles.detailActions}>
          <FavoriteButton target={{ kind: 'lesson', key: String(lesson.id) }} />
          <ErrataButton key={lesson.id} module="lesson" itemKey={String(lesson.id)} label={lesson.title} />
          {lesson.shadowing_enabled && (
            <Link className={styles.shadowingCta} to={`/lesson/${lesson.id}/shadowing`}>
              {t('lesson.reading.shadowing')}
            </Link>
          )}
        </div>
      </header>

      <LessonReading key={`${lesson.id}:${sentenceAnchor ?? ''}`} lessonId={lesson.id} sentences={lesson.sentences} sentenceAnchor={sentenceAnchor} />
    </div>
  )
}

function markLessonRead(lessonID: number) {
  const userRaw = localStorage.getItem('user')
  if (!userRaw) return
  try {
    const user = JSON.parse(userRaw) as { id?: number }
    if (user.id) {
      localStorage.setItem(`lesson_read_${user.id}_${lessonID}`, '1')
    }
  } catch {
    // Ignore malformed legacy localStorage.
  }
}
