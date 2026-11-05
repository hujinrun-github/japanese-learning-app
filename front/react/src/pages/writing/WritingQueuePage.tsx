import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import type { WritingMode, WritingRecord, WritingTopic } from '@/types/api'
import { ErrataButton } from '@/components/ui/ErrataButton'
import styles from './WritingQueuePage.module.css'

const modes = ['essay', 'sentence'] as const
const maxAnswerLength = 1000

function scoreColor(score: number) {
  if (score >= 80) return 'var(--color-success)'
  if (score >= 60) return 'var(--color-warning)'
  return 'var(--color-error)'
}

export function WritingQueuePage() {
  const { t } = useTranslation()
  const [topics, setTopics] = useState<WritingTopic[]>([])
  const [currentIndex, setCurrentIndex] = useState(0)
  const [mode, setMode] = useState<WritingMode>('essay')
  const [drafts, setDrafts] = useState<Partial<Record<WritingMode, string>>>({})
  const [loading, setLoading] = useState(true)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const [feedback, setFeedback] = useState<WritingRecord | null>(null)
  const [records, setRecords] = useState<WritingRecord[]>([])
  const [recordsLoading, setRecordsLoading] = useState(false)
  const [historyExpanded, setHistoryExpanded] = useState(false)
  const topic = topics[currentIndex]
  const answer = drafts[mode] ?? ''
  const trimmedAnswer = answer.trim()
  const answerLength = Array.from(trimmedAnswer).length
  const canSubmit = answerLength > 0 && answerLength <= maxAnswerLength

  useEffect(() => {
    void loadTopics()
  }, [])

  async function loadTopics() {
    setLoading(true)
    setError('')
    try {
      const data = await apiFetch<WritingTopic[]>('GET', '/api/v1/writing/topics')
      setTopics(data ?? [])
    } catch (err) {
      setError(err instanceof Error ? err.message : t('writing.loadError'))
    } finally {
      setLoading(false)
    }
  }

  async function fetchRecords() {
    setRecordsLoading(true)
    try {
      const data = await apiFetch<WritingRecord[]>('GET', '/api/v1/writing/records')
      setRecords(data ?? [])
    } catch {
      // silently fail
    } finally {
      setRecordsLoading(false)
    }
  }

  async function handleSubmit() {
    if (!topic || !canSubmit || submitting) return
    setSubmitting(true)
    setError('')
    try {
      const rec = await apiFetch<WritingRecord>('POST', `/api/v1/writing/topics/${topic.id}`, {
        type: mode,
        user_answer: trimmedAnswer,
      })
      setFeedback(rec)
      if (historyExpanded) void fetchRecords()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('writing.submitError'))
    } finally {
      setSubmitting(false)
    }
  }

  function handleNext() {
    setFeedback(null)
    setDrafts({})
    setCurrentIndex((i) => i + 1)
  }

  function formatDate(iso: string) {
    return new Date(iso).toLocaleDateString()
  }

  if (loading) {
    return (
      <div className={styles.page} style={{ display: 'flex', justifyContent: 'center', paddingTop: '80px' }}>
        <Spinner size="lg" />
      </div>
    )
  }

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.title}>{t('writing.title')}</h1>
        <p className={styles.intro}>{t('writing.intro')}</p>
        {topic && (
          <p className={styles.progress}>
            {t('writing.progress', { current: currentIndex + 1, total: topics.length })}
          </p>
        )}
      </div>

      {error && <p className={styles.error} role="alert">{error}</p>}

      {topic ? (
        <>
          <section className={styles.questionCard} aria-labelledby="writing-topic">
            <div className={styles.questionHeader}>
              <span className={styles.topicLabel}>{t('writing.topic')}</span>
              <Badge level={topic.jlpt_level} size="sm" />
            </div>
            <h2 id="writing-topic" className={styles.topicTitle}>{topic.title}</h2>
            {feedback ? (
              <p className={styles.topicLabel}>{t(`writing.modes.${feedback.type}`)}</p>
            ) : (
              <div className={styles.modeSwitch} role="group" aria-label={t('writing.modeLabel')}>
                {modes.map((option) => (
                  <Button
                    key={option}
                    variant={option === mode ? 'primary' : 'secondary'}
                    aria-pressed={option === mode}
                    disabled={submitting}
                    onClick={() => { setMode(option); setError('') }}
                  >
                    <span aria-hidden="true">{option === 'essay' ? '📝' : '✏️'}</span>
                    {t(`writing.modes.${option}`)}
                  </Button>
                ))}
              </div>
            )}
            <p className={styles.prompt} id="writing-instructions">
              {mode === 'essay' ? topic.essay_prompt : topic.sentence_prompt}
            </p>
            <ErrataButton
              key={`${topic.id}:${mode}`}
              module="writing"
              itemKey={`${topic.id}:${mode}`}
              label={`${topic.title} — ${mode === 'essay' ? topic.essay_prompt : topic.sentence_prompt}`}
            />
            {!feedback && (
              <>
                <label className={styles.answerLabel} htmlFor="writing-answer">{t('writing.answerLabel')}</label>
                <textarea
                  id="writing-answer"
                  aria-describedby="writing-instructions writing-length"
                  className={`${styles.textarea} ${mode === 'essay' ? styles.essayTextarea : ''}`}
                  placeholder={t('writing.placeholder')}
                  value={answer}
                  onChange={(e) => {
                    const value = e.target.value
                    setDrafts((previous) => ({ ...previous, [mode]: value }))
                  }}
                  disabled={submitting}
                />
                <div className={styles.submitRow}>
                  <span id="writing-length" className={answerLength > maxAnswerLength ? styles.error : styles.progress}>
                    {t('writing.answerLength', { count: answerLength, max: maxAnswerLength })}
                  </span>
                  <Button onClick={handleSubmit} loading={submitting} disabled={!canSubmit}>
                    {t('writing.submit')}
                  </Button>
                </div>
              </>
            )}
          </section>

          {/* Feedback */}
          {feedback && (
            <div className={styles.feedbackCard}>
              {feedback.ai_feedback ? (
                <div className={styles.scoreRow}>
                  <div className={styles.scoreCircle} style={{ borderColor: scoreColor(feedback.score) }}>
                    <span className={styles.scoreNumber} style={{ color: scoreColor(feedback.score) }}>{feedback.score}</span>
                    <span className={styles.scoreLabel}>{t('writing.feedback.score')}</span>
                  </div>
                  <div className={styles.scoreTagRow}>
                    <span className={`${styles.scoreTag} ${feedback.ai_feedback.grammar_correct ? styles.tagGood : styles.tagBad}`}>
                      文法 {feedback.ai_feedback.grammar_correct ? '✓' : '✗'}
                    </span>
                    <span className={`${styles.scoreTag} ${feedback.ai_feedback.vocab_correct ? styles.tagGood : styles.tagBad}`}>
                      語彙 {feedback.ai_feedback.vocab_correct ? '✓' : '✗'}
                    </span>
                  </div>
                </div>
              ) : (
                <p className={styles.ungraded} role="status">{t('writing.ungraded')}</p>
              )}

              {feedback.user_answer && (
                <div className={styles.feedbackSection}>
                  <div className={styles.feedbackSectionTitle}>{t('writing.feedback.yourAnswer')}</div>
                  <div className={styles.feedbackText} lang="ja">
                    {feedback.user_answer}
                  </div>
                </div>
              )}

              {feedback.ai_feedback?.corrected_sentence && (
                <div className={styles.feedbackSection}>
                  <div className={styles.feedbackSectionTitle}>
                    {t(feedback.type === 'essay' ? 'writing.feedback.correctedEssay' : 'writing.feedback.corrected')}
                  </div>
                  <div className={styles.feedbackText} lang="ja">{feedback.ai_feedback.corrected_sentence}</div>
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
                <Button onClick={handleNext}>{t(currentIndex + 1 === topics.length ? 'writing.finish' : 'writing.next')}</Button>
              </div>
            </div>
          )}
        </>
      ) : error ? (
        <Button onClick={loadTopics} variant="secondary">{t('writing.retry')}</Button>
      ) : (
        <EmptyState icon="🎉" title={t('writing.done')} description={t('writing.doneDesc')} />
      )}

      {/* history */}
      <div className={styles.historySection}>
        <button
          className={styles.historyToggle}
          aria-expanded={historyExpanded}
          disabled={submitting}
          onClick={() => { setHistoryExpanded(!historyExpanded); if (!historyExpanded) fetchRecords() }}
        >
          {t('writing.records.title')} {historyExpanded ? '▲' : '▼'}
        </button>

        {historyExpanded && (
          recordsLoading ? (
            <div style={{ display: 'flex', justifyContent: 'center', paddingTop: '20px' }}>
              <Spinner size="sm" />
            </div>
          ) : records.length === 0 ? (
            <EmptyState icon="✍️" title={t('writing.records.empty')} description="" />
          ) : (
            <div className={styles.recordList}>
              {records.map((rec) => (
                <div key={rec.id} className={styles.recordItem}>
                  <div className={styles.recordTop}>
                    <span className={styles.recordType}>{t(`writing.modes.${rec.type}`)}</span>
                    <span className={styles.recordDate}>{formatDate(rec.practiced_at)}</span>
                  </div>
                  <div className={styles.recordPrompt}>{rec.question}</div>
                  <div className={styles.recordAnswer} lang="ja">{rec.user_answer}</div>
                  <div className={styles.recordBottom}>
                    {rec.type === 'input' || rec.ai_feedback ? (
                      <span className={styles.scoreBadge} style={{ background: scoreColor(rec.score) }}>
                        {t('writing.score')} {rec.score}
                      </span>
                    ) : (
                      <span className={styles.recordDate}>{t('writing.ungradedShort')}</span>
                    )}
                    {rec.ai_feedback?.reference_answer && (
                      <span className={styles.recordRef}>{rec.ai_feedback.reference_answer}</span>
                    )}
                  </div>
                </div>
              ))}
            </div>
          )
        )}
      </div>
    </div>
  )
}
