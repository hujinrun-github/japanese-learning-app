import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import type { LessonSummary, Lesson } from '@/types/api'
import styles from './LessonPage.module.css'

export function LessonPage() {
  const { t } = useTranslation()
  const [summaries, setSummaries] = useState<LessonSummary[]>([])
  const [selected, setSelected] = useState<Lesson | null>(null)
  const [loading, setLoading] = useState(true)
  const [detailLoading, setDetailLoading] = useState(false)
  const [error, setError] = useState('')
  const [showTranslation, setShowTranslation] = useState(false)
  const [readSet, setReadSet] = useState<Set<number>>(new Set())

  // Load read-status from localStorage once on mount
  useEffect(() => {
    const userRaw = localStorage.getItem('user')
    if (!userRaw) return
    try {
      const user = JSON.parse(userRaw) as { id?: number }
      if (!user.id) return
      const uid = user.id
      const ids = new Set<number>()
      for (let i = 0; i < localStorage.length; i++) {
        const key = localStorage.key(i)
        if (key?.startsWith(`lesson_read_${uid}_`)) {
          const lessonId = Number(key.replace(`lesson_read_${uid}_`, ''))
          if (!isNaN(lessonId)) ids.add(lessonId)
        }
      }
      setReadSet(ids)
    } catch {
      // ignore parse errors
    }
  }, [])

  useEffect(() => {
    loadLessons()
  }, [])

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

  async function loadLesson(id: number) {
    setDetailLoading(true)
    setError('')
    setShowTranslation(false)
    try {
      const data = await apiFetch<Lesson>('GET', `/api/v1/lessons/${id}`)
      setSelected(data)
      // Mark as read in localStorage
      const userRaw = localStorage.getItem('user')
      if (userRaw) {
        try {
          const user = JSON.parse(userRaw) as { id?: number }
          if (user.id) {
            localStorage.setItem(`lesson_read_${user.id}_${id}`, '1')
            setReadSet((prev) => new Set(prev).add(id))
          }
        } catch {
          // ignore
        }
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    } finally {
      setDetailLoading(false)
    }
  }

  if (loading) {
    return (
      <div className={styles.page} style={{ display: 'flex', justifyContent: 'center', paddingTop: '80px' }}>
        <Spinner size="lg" />
      </div>
    )
  }

  // Detail view
  if (selected) {
    return (
      <div className={styles.page}>
        <button className={styles.backBtn} onClick={() => setSelected(null)}>
          {t('lesson.back')}
        </button>

        {detailLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', paddingTop: '60px' }}>
            <Spinner size="lg" />
          </div>
        ) : (
          <>
            <h1 className={styles.detailTitle}>{selected.title}</h1>
            <div className={styles.detailMeta}>
              <Badge level={selected.jlpt_level} size="sm" />
              <span className={styles.charCount}>
                {t('lesson.chars', { count: selected.sentence_count })}
              </span>
              {selected.tags?.map((tag) => (
                <span key={tag} className={styles.tag}>{tag}</span>
              ))}
            </div>

            <button
              className={styles.translateToggle}
              onClick={() => setShowTranslation((v) => !v)}
            >
              {showTranslation ? t('lesson.translation.hide') : t('lesson.translation.show')}
            </button>

            <div className={styles.sentences}>
              {selected.sentences?.map((sentence) => (
                <div key={sentence.index} className={styles.sentenceBlock}>
                  <div className={styles.sentenceJa}>
                    {sentence.tokens.map((token, i) =>
                      token.reading ? (
                        <ruby key={i}>
                          {token.surface}
                          <rt>{token.reading}</rt>
                        </ruby>
                      ) : (
                        <span key={i}>{token.surface}</span>
                      )
                    )}
                  </div>
                  {showTranslation && sentence.translation && (
                    <div className={styles.sentenceZh}>{sentence.translation}</div>
                  )}
                </div>
              ))}
            </div>
          </>
        )}
      </div>
    )
  }

  // List view
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
              onClick={() => loadLesson(lesson.id)}
            >
              <div className={styles.lessonLeft}>
                <div className={styles.lessonTitle}>
                  {readSet.has(lesson.id) && (
                    <span className={styles.readMark} title="已读">✓</span>
                  )}
                  {lesson.title}
                </div>
                <div className={styles.lessonMeta}>
                  <span className={styles.charCount}>
                    {t('lesson.chars', { count: lesson.sentence_count })}
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
