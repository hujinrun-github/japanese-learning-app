import { useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Button } from '@/components/ui/Button'
import { DailyPracticeProgress } from '@/components/ui/DailyPracticeProgress'
import { useDailyPracticeProgress } from '@/hooks/useDailyPracticeProgress'
import { PracticeKnowledgeLinks, RemotePracticeKnowledgeLinks } from '@/pages/search/PracticeKnowledgeLinks'
import { useLookup } from '@/pages/search/useLookup'
import type { WritingQuestion, WritingRecord } from '@/types/api'
import styles from './WritingQueuePage.module.css'

export function WritingQuestionsPage() {
  const { t } = useTranslation()
  const { result: queue, retry } = useLookup<WritingQuestion[]>('/api/v1/writing/queue')
  const { progress, refreshProgress } = useDailyPracticeProgress('writing')
  const [index, setIndex] = useState(0)
  const [answer, setAnswer] = useState('')
  const [feedback, setFeedback] = useState<WritingRecord | null>(null)
  const [pending, setPending] = useState(false)
  const [error, setError] = useState('')
  const question = queue.status === 'ready' ? queue.data[index] : undefined

  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!question || !answer.trim() || pending || feedback) return
    setPending(true)
    setError('')
    try {
      const record = await apiFetch<WritingRecord>('POST', `/api/v1/writing/${question.type}`, {
        question_id: question.id,
        question: question.prompt,
        user_answer: answer.trim(),
      })
      setFeedback(record)
      refreshProgress()
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : t('writing.submitError'))
    } finally { setPending(false) }
  }

  function next() {
    setFeedback(null)
    setAnswer('')
    setError('')
    setIndex((current) => current + 1)
  }

  return (
    <div className={styles.page}>
      <header className={styles.header}>
        <Link className={styles.backLink} to="/writing">{t('writing.topicList.backToList')}</Link>
        <h1 className={styles.title}>{t('writing.questionPractice')}</h1>
        <DailyPracticeProgress progress={progress} />
      </header>
      {queue.status === 'loading' && <p role="status">{t('common.loading')}</p>}
      {queue.status === 'error' && <div role="alert">
        <p>{t('writing.questionLoadError')}</p><Button variant="secondary" onClick={retry}>{t('writing.retry')}</Button>
      </div>}
      {queue.status === 'ready' && !queue.data.length && <p role="status">{t('writing.noQuestions')}</p>}
      {queue.status === 'ready' && queue.data.length > 0 && !question && (
        <section className={styles.questionCard}>
          <h2>{t('writing.questionRoundComplete')}</h2>
          <Button variant="secondary" onClick={() => { setIndex(0); retry() }}>{t('writing.newQuestionRound')}</Button>
        </section>
      )}
      {question && queue.status === 'ready' && (
        <section className={styles.questionCard} aria-labelledby="writing-single-question">
          <div className={styles.questionHeader}>
            <span className={styles.topicLabel}>{t(`writing.modes.${question.type}`)}</span>
            <span className={styles.progress}>{index + 1}/{queue.data.length}</span>
          </div>
          <h2 className={styles.topicTitle} id="writing-single-question">{question.prompt}</h2>
          {!feedback && question.type === 'sentence' && (
            <RemotePracticeKnowledgeLinks path={`/api/v1/writing/questions/${question.id}/knowledge-links`} hints />
          )}
          <form onSubmit={(event) => void submit(event)}>
            <label className={styles.answerLabel} htmlFor="writing-single-answer">{t('writing.answerLabel')}</label>
            <textarea id="writing-single-answer" className={styles.textarea} value={answer} maxLength={1000}
              readOnly={pending || !!feedback} onChange={(event) => setAnswer(event.target.value)} />
            {!feedback && <Button type="submit" loading={pending} disabled={!answer.trim()}>{t('writing.submit')}</Button>}
          </form>
          {error && <p className={styles.error} role="alert">{error}</p>}
          {feedback && (
            <div>
              <p role="status">{t('writing.feedback.score')}：{feedback.score}</p>
              {feedback.ai_feedback?.reference_answer && <div className={styles.feedbackSection}>
                <strong>{t('writing.feedback.reference')}</strong>
                <p lang="ja">{feedback.ai_feedback.reference_answer}</p>
              </div>}
              {feedback.ai_feedback?.corrected_sentence && <p lang="ja">{feedback.ai_feedback.corrected_sentence}</p>}
              {feedback.ai_feedback?.issue_description && <p>{feedback.ai_feedback.issue_description}</p>}
              <PracticeKnowledgeLinks links={feedback.knowledge_links} />
              <Button onClick={next}>{t(index + 1 < queue.data.length ? 'writing.nextQuestion' : 'writing.finish')}</Button>
            </div>
          )}
        </section>
      )}
    </div>
  )
}
