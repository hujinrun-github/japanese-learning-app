import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import type { WritingQuestion, WritingRecord } from '@/types/api'
import styles from './WritingQueuePage.module.css'

export function WritingQueuePage() {
  const { t } = useTranslation()
  const [queue, setQueue] = useState<WritingQuestion[]>([])
  const [currentIndex, setCurrentIndex] = useState(0)
  const [answer, setAnswer] = useState('')
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const [feedback, setFeedback] = useState<WritingRecord | null>(null)

  useEffect(() => {
    loadQueue()
  }, [])

  async function loadQueue() {
    setLoading(true)
    setError('')
    try {
      const data = await apiFetch<WritingQuestion[]>('GET', '/api/v1/writing/queue')
      setQueue(data ?? [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }

  async function handleSubmit() {
    if (!answer.trim() || submitting) return
    const q = queue[currentIndex]
    setSubmitting(true)
    setError('')
    try {
      let rec: WritingRecord
      if (q.type === 'sentence') {
        rec = await apiFetch<WritingRecord>('POST', '/api/v1/writing/sentence', {
          question: q.prompt,
          user_answer: answer,
        })
      } else {
        rec = await apiFetch<WritingRecord>('POST', '/api/v1/writing/input', {
          question: q.prompt,
          user_answer: answer,
          expected: '',
        })
      }
      setFeedback(rec)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Submit failed')
    } finally {
      setSubmitting(false)
    }
  }

  function handleNext() {
    setFeedback(null)
    setAnswer('')
    setCurrentIndex((i) => i + 1)
  }

  if (loading) {
    return (
      <div className={styles.page} style={{ display: 'flex', justifyContent: 'center', paddingTop: '80px' }}>
        <Spinner size="lg" />
      </div>
    )
  }

  const done = queue.length === 0 || currentIndex >= queue.length
  const q = done ? null : queue[currentIndex]

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.title}>{t('writing.title')}</h1>
        {!done && (
          <p className={styles.progress}>
            {currentIndex + 1} / {queue.length}
          </p>
        )}
      </div>

      {error && <p style={{ color: 'var(--color-error)', marginBottom: 'var(--space-3)' }}>{error}</p>}

      {done ? (
        <EmptyState icon="🎉" title={t('writing.done')} description={t('writing.doneDesc')} />
      ) : (
        <>
          {/* Question */}
          <div className={styles.questionCard}>
            <span className={styles.typeBadge}>{q!.type}</span>
            <p className={styles.prompt}>{q!.prompt}</p>

            {!feedback && (
              <>
                <textarea
                  className={styles.textarea}
                  placeholder={t('writing.placeholder')}
                  value={answer}
                  onChange={(e) => setAnswer(e.target.value)}
                  disabled={submitting}
                />
                <div className={styles.submitRow}>
                  <Button onClick={handleSubmit} loading={submitting} disabled={!answer.trim()}>
                    {t('writing.submit')}
                  </Button>
                </div>
              </>
            )}
          </div>

          {/* Feedback */}
          {feedback && (
            <div className={styles.feedbackCard}>
              <div className={styles.scoreRow}>
                <div className={styles.scoreCircle} style={{
                  borderColor: feedback.score >= 80
                    ? 'var(--color-success)'
                    : feedback.score >= 60
                    ? 'var(--color-warning)'
                    : 'var(--color-error)',
                }}>
                  <span className={styles.scoreNumber} style={{
                    color: feedback.score >= 80
                      ? 'var(--color-success)'
                      : feedback.score >= 60
                      ? 'var(--color-warning)'
                      : 'var(--color-error)',
                  }}>{feedback.score}</span>
                  <span className={styles.scoreLabel}>{t('writing.feedback.score')}</span>
                </div>
                {feedback.ai_feedback && (
                  <div className={styles.scoreTagRow}>
                    <span className={`${styles.scoreTag} ${feedback.ai_feedback.grammar_correct ? styles.tagGood : styles.tagBad}`}>
                      文法 {feedback.ai_feedback.grammar_correct ? '✓' : '✗'}
                    </span>
                    <span className={`${styles.scoreTag} ${feedback.ai_feedback.vocab_correct ? styles.tagGood : styles.tagBad}`}>
                      語彙 {feedback.ai_feedback.vocab_correct ? '✓' : '✗'}
                    </span>
                  </div>
                )}
              </div>

              {feedback.ai_feedback?.corrected_sentence && (
                <div className={styles.feedbackSection}>
                  <div className={styles.feedbackSectionTitle}>{t('writing.feedback.corrected')}</div>
                  <div className={styles.feedbackText}>{feedback.ai_feedback.corrected_sentence}</div>
                </div>
              )}

              {feedback.ai_feedback?.issue_description && (
                <div className={styles.feedbackSection}>
                  <div className={styles.feedbackSectionTitle}>{t('writing.feedback.issues')}</div>
                  <div className={styles.feedbackText}>{feedback.ai_feedback.issue_description}</div>
                </div>
              )}

              {feedback.ai_feedback?.alternative_phrases && feedback.ai_feedback.alternative_phrases.length > 0 && (
                <div className={styles.feedbackSection}>
                  <div className={styles.feedbackSectionTitle}>{t('writing.feedback.alternatives')}</div>
                  <div className={styles.alternativeList}>
                    {feedback.ai_feedback.alternative_phrases.map((phrase, i) => (
                      <div key={i} className={styles.alternativeItem}>{phrase}</div>
                    ))}
                  </div>
                </div>
              )}

              {feedback.ai_feedback?.reference_answer && (
                <div className={styles.feedbackSection}>
                  <div className={styles.feedbackSectionTitle}>{t('writing.feedback.reference')}</div>
                  <div className={styles.feedbackText}>{feedback.ai_feedback.reference_answer}</div>
                </div>
              )}

              <div className={styles.nextRow}>
                <Button onClick={handleNext}>{t('writing.next')}</Button>
              </div>
            </div>
          )}
        </>
      )}
    </div>
  )
}
