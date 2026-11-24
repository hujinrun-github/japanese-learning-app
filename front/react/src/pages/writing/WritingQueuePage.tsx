import { DailyPracticeProgress } from '@/components/ui/DailyPracticeProgress'
import { useDailyPracticeProgress } from '@/hooks/useDailyPracticeProgress'
import { useState, useEffect, useRef } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import type { WritingDraft, WritingMode, WritingRecord, WritingTopic } from '@/types/api'
import { ErrataButton } from '@/components/ui/ErrataButton'
import { DictionaryButton } from '@/pages/search/ContextualSearch'
import {
  filterWritingTopics,
  findWritingTopicIndex,
  type PracticeStatusFilter,
  type TopicLevelFilter,
} from './writingTopicList'
import styles from './WritingQueuePage.module.css'

const modes = ['essay', 'sentence'] as const
const levels = ['N5', 'N4', 'N3', 'N2', 'N1'] as const
const maxAnswerLength = 1000
type TopicDrafts = Record<string, Partial<Record<WritingMode, string>>>

function scoreColor(score: number) {
  if (score >= 80) return 'var(--color-success)'
  if (score >= 60) return 'var(--color-warning)'
  return 'var(--color-error)'
}

export function WritingQueuePage() {
  const { t } = useTranslation()
  const { progress, refreshProgress } = useDailyPracticeProgress('writing')
  const [searchParams] = useSearchParams()
  const requestedTopicID = searchParams.get('topic_id')
  const [topics, setTopics] = useState<WritingTopic[]>([])
  const [level, setLevel] = useState<TopicLevelFilter>('all')
  const [practiceStatus, setPracticeStatus] = useState<PracticeStatusFilter>(requestedTopicID ? 'all' : 'unpracticed')
  const [currentIndex, setCurrentIndex] = useState(0)
  const [mode, setMode] = useState<WritingMode>('essay')
  const [drafts, setDrafts] = useState<TopicDrafts>({})
  const [savedDrafts, setSavedDrafts] = useState<TopicDrafts>({})
  const [savingDraft, setSavingDraft] = useState(false)
  const [draftMessage, setDraftMessage] = useState('')
  const [draftError, setDraftError] = useState('')
  const requests = useRef<AbortController | null>(null)
  const [loading, setLoading] = useState(true)
  const [hasLoaded, setHasLoaded] = useState(false)
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState('')
  const [feedback, setFeedback] = useState<WritingRecord | null>(null)
  const [records, setRecords] = useState<WritingRecord[]>([])
  const [recordsLoading, setRecordsLoading] = useState(false)
  const [historyExpanded, setHistoryExpanded] = useState(false)
  const filteredTopics = filterWritingTopics(topics, level, practiceStatus)
  const topic = filteredTopics[currentIndex]
  const answer = topic ? drafts[topic.id]?.[mode] ?? '' : ''
  const trimmedAnswer = answer.trim()
  const answerLength = Array.from(trimmedAnswer).length
  const canSubmit = answerLength > 0 && answerLength <= maxAnswerLength
  const busy = submitting || savingDraft
  const hasDraftChanges = topic && modes.some(option => (drafts[topic.id]?.[option] ?? '') !== (savedDrafts[topic.id]?.[option] ?? ''))

  useEffect(() => {
    const controller = new AbortController()
    requests.current = controller
    void loadTopics(controller.signal)
    return () => controller.abort()
  }, [])

  async function loadTopics(signal = requests.current?.signal) {
    setLoading(true)
    setError('')
    try {
      const [data, saved] = await Promise.all([
        apiFetch<WritingTopic[]>('GET', '/api/v1/writing/topics', undefined, signal),
        apiFetch<WritingDraft[]>('GET', '/api/v1/writing/drafts', undefined, signal),
      ])
      if (signal?.aborted) return
      setTopics(data ?? [])
      setHasLoaded(true)
      const available = filterWritingTopics(data ?? [], level, practiceStatus)
      const restored: TopicDrafts = {}
      const validDrafts = (saved ?? []).filter(draft => data?.some(item => item.id === draft.topic_id))
      for (const draft of validDrafts) {
        restored[draft.topic_id] = { ...restored[draft.topic_id], [draft.type]: draft.user_answer }
      }
      setDrafts(restored)
      setSavedDrafts(restored)
      const resume = validDrafts.find(draft => available.some(item => item.id === draft.topic_id))
      const resumeIndex = resume ? available.findIndex(item => item.id === resume.topic_id) : -1
      const requestedIndex = findWritingTopicIndex(available, requestedTopicID)
      setCurrentIndex(requestedIndex >= 0 ? requestedIndex : (resumeIndex >= 0 ? resumeIndex : 0))
      setMode(resume?.type ?? 'essay')
      setDraftMessage(resume ? t('writing.draftRestored') : '')
    } catch (err) {
      if (signal?.aborted) return
      setError(err instanceof Error ? err.message : t('writing.loadError'))
    } finally {
      if (!signal?.aborted) setLoading(false)
    }
  }

  async function saveCurrentDrafts(): Promise<boolean> {
    if (!topic) return true
    const changed = [...modes.filter(option => option !== mode), mode].filter(option =>
      (drafts[topic.id]?.[option] ?? '') !== (savedDrafts[topic.id]?.[option] ?? ''),
    )
    if (changed.length === 0) return true
    const signal = requests.current?.signal
    setSavingDraft(true)
    setDraftMessage('')
    setDraftError('')
    try {
      for (const option of changed) {
        const value = drafts[topic.id]?.[option] ?? ''
        await apiFetch<WritingDraft>('PUT', `/api/v1/writing/topics/${topic.id}/draft`, {
          type: option, user_answer: value,
        }, signal)
        setSavedDrafts(previous => ({ ...previous, [topic.id]: { ...previous[topic.id], [option]: value } }))
      }
      setDraftMessage(t('writing.draftSaved'))
      return true
    } catch {
      if (!signal?.aborted) setDraftError(t('writing.draftSaveError'))
      return false
    } finally {
      setSavingDraft(false)
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
    if (!topic || !canSubmit || busy) return
    setSubmitting(true)
    setError('')
    setDraftError('')
    try {
      if (!(await saveCurrentDrafts())) return
      const rec = await apiFetch<WritingRecord>('POST', `/api/v1/writing/topics/${topic.id}`, {
        type: mode,
        user_answer: trimmedAnswer,
      }, requests.current?.signal)
      setFeedback(rec)
      setDrafts(previous => ({ ...previous, [topic.id]: { ...previous[topic.id], [mode]: '' } }))
      setSavedDrafts(previous => ({ ...previous, [topic.id]: { ...previous[topic.id], [mode]: '' } }))
      setDraftMessage('')
      refreshProgress()
      if (historyExpanded) void fetchRecords()
    } catch (err) {
      setError(err instanceof Error ? err.message : t('writing.submitError'))
    } finally {
      setSubmitting(false)
    }
  }

  async function handleNext() {
    if (!topic || busy || (currentIndex + 1 === filteredTopics.length && !feedback)) return
    if (!(await saveCurrentDrafts())) return
    const nextID = filteredTopics[currentIndex + 1]?.id
    // Keep the submitted topic visible until its feedback has been read.
    const updated = feedback ? topics.map(item => item.id === topic.id ? { ...item, practiced: true } : item) : topics
    const nextQueue = filterWritingTopics(updated, level, practiceStatus)
    const nextIndex = nextQueue.findIndex(item => item.id === nextID)
    setTopics(updated)
    setFeedback(null)
    setError('')
    setDraftError('')
    setDraftMessage('')
    setCurrentIndex(nextIndex >= 0 ? nextIndex : nextQueue.length)
  }

  async function handleFilterChange(nextLevel: TopicLevelFilter, nextStatus: PracticeStatusFilter) {
    if (busy || !(await saveCurrentDrafts())) return
    const updated = feedback && topic ? topics.map(item => item.id === topic.id ? { ...item, practiced: true } : item) : topics
    const available = filterWritingTopics(updated, nextLevel, nextStatus)
    const selected = available.find(item => modes.some(option => drafts[item.id]?.[option]?.trim()))
      ?? available[0]
    const resumeMode = selected && modes.find(option => drafts[selected.id]?.[option]?.trim())
    setTopics(updated)
    setLevel(nextLevel)
    setPracticeStatus(nextStatus)
    setCurrentIndex(selected ? available.findIndex(item => item.id === selected.id) : 0)
    setMode(selected?.id === topic?.id && !feedback ? mode : resumeMode || 'essay')
    setFeedback(null)
    setError('')
    setDraftError('')
    setDraftMessage(resumeMode ? t('writing.draftRestored') : '')
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
        <Link className={styles.backLink} to="/writing">{t('writing.topicList.backToList')}</Link>
        <div className={styles.headerRow}>
          <h1 className={styles.title}>{t('writing.title')}</h1>
          {topic && (
            <Button variant="secondary" onClick={() => void handleNext()} disabled={busy || (currentIndex + 1 === filteredTopics.length && !feedback)}>
              {t(currentIndex + 1 === filteredTopics.length ? (feedback ? 'writing.finish' : 'writing.last') : 'writing.next')}
            </Button>
          )}
        </div>
        <p className={styles.intro}>{t('writing.intro')}</p>
        <div className={styles.filters}>
          <label className={styles.filter} htmlFor="writing-level">
            {t('writing.levelFilter')}
            <select id="writing-level" value={level} disabled={busy || !hasLoaded} onChange={event => void handleFilterChange(event.target.value as TopicLevelFilter, practiceStatus)}>
              <option value="all">{t('writing.allLevels')}</option>
              {levels.map(option => <option key={option} value={option}>{option}</option>)}
            </select>
          </label>
          <label className={styles.filter} htmlFor="writing-practice-status">
            {t('writing.statusFilter')}
            <select id="writing-practice-status" value={practiceStatus} disabled={busy || !hasLoaded} onChange={event => void handleFilterChange(level, event.target.value as PracticeStatusFilter)}>
              <option value="unpracticed">{t('writing.unpracticed')}</option>
              <option value="practiced">{t('writing.practiced')}</option>
              <option value="all">{t('writing.allTopics')}</option>
            </select>
          </label>
        </div>
        <p className={styles.filterHint}>{t('writing.filterHint')}</p>
        {topic && (
          <p className={styles.progress}>
            {t('writing.progress', { current: currentIndex + 1, total: filteredTopics.length })}
          </p>
        )}
      </div>

      <DailyPracticeProgress progress={progress} />
      {error && <p className={styles.error} role="alert">{error}</p>}
      {draftError && <p className={styles.error} role="alert">{draftError}</p>}

      {topic ? (
        <>
          <section className={styles.questionCard} aria-labelledby="writing-topic">
            <div className={styles.questionHeader}>
              <span className={styles.topicLabel}>{t('writing.topic')}</span>
              <Badge level={topic.jlpt_level} size="sm" />
              {(topic.practiced || feedback) && <span className={styles.practicedBadge}>{t('writing.practiced')}</span>}
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
                    disabled={busy}
                    onClick={() => { setMode(option); setError(''); setDraftMessage('') }}
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
                    setDrafts((previous) => ({ ...previous, [topic.id]: { ...previous[topic.id], [mode]: value } }))
                    setDraftMessage('')
                  }}
                  disabled={busy}
                />
                <div className={styles.submitRow}>
                  <span id="writing-length" className={answerLength > maxAnswerLength ? styles.error : styles.progress}>
                    {t('writing.answerLength', { count: answerLength, max: maxAnswerLength })}
                  </span>
                  <div className={styles.answerActions}>
                    <Button variant="secondary" onClick={() => void saveCurrentDrafts()} loading={savingDraft && !submitting} disabled={busy || !hasDraftChanges}>
                      {t('writing.saveDraft')}
                    </Button>
                    <Button onClick={handleSubmit} loading={submitting} disabled={!canSubmit || savingDraft}>
                      {t('writing.submit')}
                    </Button>
                  </div>
                </div>
                <p className={styles.draftHint}>{t('writing.draftHint')}</p>
                {draftMessage && <p className={styles.draftStatus} role="status">{draftMessage}</p>}
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
                  <div className={styles.feedbackText} lang="ja" data-dictionary-text>{feedback.ai_feedback.corrected_sentence}</div>
                  <DictionaryButton text={feedback.ai_feedback.corrected_sentence} />
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
                <Button onClick={() => void handleNext()} disabled={busy}>{t(currentIndex + 1 === filteredTopics.length ? 'writing.finish' : 'writing.next')}</Button>
              </div>
            </div>
          )}
        </>
      ) : error ? (
        <Button onClick={() => void loadTopics()} variant="secondary">{t('writing.retry')}</Button>
      ) : filteredTopics.length === 0 ? (
        <EmptyState icon="📝" title={t('writing.noMatchingTopics')} description={t('writing.noMatchingTopicsDesc')} />
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
