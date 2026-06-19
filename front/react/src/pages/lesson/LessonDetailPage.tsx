import { useEffect, useState } from 'react'
import { Link, useNavigate, useParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import type { Lesson } from '@/types/api'
import styles from './LessonPage.module.css'

export function LessonDetailPage() {
  const { t } = useTranslation()
  const { id } = useParams()
  const navigate = useNavigate()
  const [lesson, setLesson] = useState<Lesson | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [showTranslation, setShowTranslation] = useState(false)

  useEffect(() => {
    let ignore = false

    async function loadLesson() {
      if (!id) return
      setLoading(true)
      setError('')
      setShowTranslation(false)
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
        <button className={styles.backBtn} onClick={() => navigate('/lesson')}>
          {t('lesson.back')}
        </button>
        <p style={{ color: 'var(--color-error)' }}>{error}</p>
      </div>
    )
  }

  if (!lesson) return null

  return (
    <div className={styles.page}>
      <button className={styles.backBtn} onClick={() => navigate('/lesson')}>
        {t('lesson.back')}
      </button>

      <h1 className={styles.detailTitle}>{lesson.title}</h1>
      <div className={styles.detailMeta}>
        <Badge level={lesson.jlpt_level} size="sm" />
        <span className={styles.charCount}>{t('lesson.chars', { count: lesson.char_count })}</span>
        {lesson.tags?.map((tag) => (
          <span key={tag} className={styles.tag}>{tag}</span>
        ))}
      </div>

      {lesson.shadowing_enabled && (
        <Link className={styles.shadowingCta} to={`/lesson/${lesson.id}/shadowing`}>
          开始影子跟读
        </Link>
      )}

      <button className={styles.translateToggle} onClick={() => setShowTranslation((value) => !value)}>
        {showTranslation ? t('lesson.translation.hide') : t('lesson.translation.show')}
      </button>

      <div className={styles.sentences}>
        {lesson.sentences?.map((sentence) => (
          <div key={sentence.index} className={styles.sentenceBlock}>
            <div className={styles.sentenceJa}>
              {sentence.tokens.map((token, index) =>
                token.reading ? (
                  <ruby key={index}>
                    {token.surface}
                    <rt>{token.reading}</rt>
                  </ruby>
                ) : (
                  <span key={index}>{token.surface}</span>
                )
              )}
            </div>
            {showTranslation && sentence.chinese && (
              <div className={styles.sentenceZh}>{sentence.chinese}</div>
            )}
          </div>
        ))}
      </div>
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
