import { useState, useEffect } from 'react'
import { useParams, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { StatusBadge } from '@/components/ui/StatusBadge'
import type { GrammarPointWithStatus, QuizSubmission, QuizResult } from '@/types/api'
import styles from './GrammarDetailPage.module.css'

export function GrammarDetailPage() {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()

  const [point, setPoint] = useState<GrammarPointWithStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  // Quiz state
  const [quizOpen, setQuizOpen] = useState(false)
  const [answers, setAnswers] = useState<Record<number, string>>({})
  const [result, setResult] = useState<QuizResult | null>(null)
  const [submitting, setSubmitting] = useState(false)

  useEffect(() => {
    if (!id) return
    loadPoint(Number(id))
  }, [id])

  async function loadPoint(pointId: number) {
    setLoading(true)
    setError('')
    try {
      const data = await apiFetch<GrammarPointWithStatus>('GET', `/api/v1/grammar/${pointId}`)
      setPoint(data)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }

  async function handleSubmitQuiz() {
    if (!point) return
    setSubmitting(true)
    try {
      const submissions: QuizSubmission[] = point.quiz_questions.map((q) => ({
        question_id: q.id,
        answer: answers[q.id] ?? '',
      }))
      const res = await apiFetch<QuizResult>('POST', `/api/v1/grammar/${point.id}/quiz`, submissions)
      setResult(res)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Submit failed')
    } finally {
      setSubmitting(false)
    }
  }

  if (loading) {
    return (
      <div className={styles.page} style={{ display: 'flex', justifyContent: 'center', paddingTop: '80px' }}>
        <Spinner size="lg" />
      </div>
    )
  }

  if (error || !point) {
    return (
      <div className={styles.page}>
        <p style={{ color: 'var(--color-error)' }}>{error || 'Not found'}</p>
        <Button variant="secondary" onClick={() => navigate('/grammar')}>← Back</Button>
      </div>
    )
  }

  return (
    <div className={styles.page}>
      <button className={styles.back} onClick={() => navigate('/grammar')}>
        ← {t('lesson.back')}
      </button>

      <div className={styles.nameRow}>
        <h1 className={styles.name}>{point.name}</h1>
        <Badge level={point.jlpt_level} size="sm" />
        {point.user_status && <StatusBadge status={point.user_status} />}
      </div>
      <p className={styles.meaning}>{point.meaning}</p>

      {/* Conjunction rule */}
      {point.conjunction_rule && (
        <div className={styles.section}>
          <div className={styles.sectionTitle}>{t('grammar.detail.conjunction')}</div>
          <div className={styles.codeBlock}>{point.conjunction_rule}</div>
        </div>
      )}

      {/* Usage note */}
      {point.usage_note && (
        <div className={styles.section}>
          <div className={styles.sectionTitle}>{t('grammar.detail.usage')}</div>
          <p className={styles.usageNote}>{point.usage_note}</p>
        </div>
      )}

      {/* Examples */}
      {point.examples && point.examples.length > 0 && (
        <div className={styles.section}>
          <div className={styles.sectionTitle}>{t('grammar.detail.examples')}</div>
          <div className={styles.exampleList}>
            {point.examples.map((ex, i) => (
              <div key={i} className={styles.exampleItem}>
                <div className={styles.exampleJa}>{ex.japanese}</div>
                <div className={styles.exampleZh}>{ex.chinese}</div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Quiz */}
      {point.quiz_questions && point.quiz_questions.length > 0 && (
        <div className={styles.section}>
          <button className={styles.quizToggle} onClick={() => setQuizOpen((o) => !o)}>
            <span>{t('grammar.detail.quiz.title')}</span>
            <span className={`${styles.chevron} ${quizOpen ? styles.chevronOpen : ''}`}>▼</span>
          </button>

          {quizOpen && (
            <div className={styles.quizBody}>
              {result && (
                <div className={styles.scoreCard}>
                  <div className={styles.scoreBig}>{result.score}</div>
                  <div className={styles.scoreLabel}>{t('grammar.detail.quiz.score', { score: result.score })}</div>
                </div>
              )}

              {point.quiz_questions.map((q) => {
                const itemResult = result?.results.find((r) => r.question_id === q.id)
                return (
                  <div key={q.id} className={styles.question}>
                    <div className={styles.questionPrompt}>{q.prompt}</div>

                    {q.type === 'fill_blank' ? (
                      <input
                        className={styles.fillInput}
                        type="text"
                        placeholder={t('grammar.detail.quiz.placeholder')}
                        value={answers[q.id] ?? ''}
                        onChange={(e) => setAnswers((a) => ({ ...a, [q.id]: e.target.value }))}
                        disabled={!!result}
                      />
                    ) : (
                      <div className={styles.options}>
                        {q.options?.map((opt) => {
                          const selected = answers[q.id] === opt
                          let cls = styles.option
                          if (itemResult) {
                            if (opt === itemResult.expected) cls += ` ${styles.resultCorrect}`
                            else if (selected && !itemResult.correct) cls += ` ${styles.resultWrong}`
                          } else if (selected) {
                            cls += ` ${styles.optionSelected}`
                          }
                          return (
                            <label key={opt} className={cls}>
                              <input
                                type="radio"
                                name={`q-${q.id}`}
                                value={opt}
                                checked={selected}
                                onChange={() => !result && setAnswers((a) => ({ ...a, [q.id]: opt }))}
                                disabled={!!result}
                                style={{ accentColor: 'var(--color-primary)' }}
                              />
                              {opt}
                            </label>
                          )
                        })}
                      </div>
                    )}

                    {itemResult && (
                      <>
                        <div className={`${styles.resultLabel} ${itemResult.correct ? styles.labelCorrect : styles.labelWrong}`}>
                          {itemResult.correct ? t('grammar.detail.quiz.correct') : t('grammar.detail.quiz.wrong')}
                        </div>
                        {itemResult.explanation && (
                          <div className={styles.explanation}>
                            {t('grammar.detail.quiz.explanation')}：{itemResult.explanation}
                          </div>
                        )}
                      </>
                    )}
                  </div>
                )
              })}

              {!result && (
                <div className={styles.submitRow}>
                  <Button onClick={handleSubmitQuiz} loading={submitting}>
                    {t('grammar.detail.quiz.submit')}
                  </Button>
                </div>
              )}
            </div>
          )}
        </div>
      )}
    </div>
  )
}
