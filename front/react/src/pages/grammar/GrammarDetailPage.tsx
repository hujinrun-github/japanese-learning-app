import { DailyPracticeProgress } from '@/components/ui/DailyPracticeProgress'
import { useSourceAnchor } from '@/hooks/useSourceAnchor'
import { useState, useEffect } from 'react'
import { useLocation, useParams, useNavigate } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { speakExample } from '@/util/exampleAudio'
import type { GrammarPointWithStatus, QuizSubmission, QuizResult } from '@/types/api'
import { ErrataButton } from '@/components/ui/ErrataButton'
import { DictionaryButton } from '@/pages/search/ContextualSearch'
import { createMistake } from '@/api/mistakes'
import { buildGrammarMistakes } from '@/pages/mistakes/mistakeInput'
import { answerPartsFor, hasFullAnswer } from './quizAnswer'
import { grammarDeepLink } from './grammarDeepLink'
import { withPracticeTask } from '@/pages/studyplan/practiceTask'
import { translationRoundPath } from '@/pages/translation/grammarLink'
import { useGrammarPracticeProgress } from './useGrammarPracticeProgress'
import styles from './GrammarDetailPage.module.css'

export function GrammarDetailPage({ fromTranslation = false }: { fromTranslation?: boolean }) {
  const { t } = useTranslation()
  const { id } = useParams<{ id: string }>()
  const navigate = useNavigate()
  const { state, search, hash } = useLocation()
  const { quizOpen: initialQuizOpen, anchor } = grammarDeepLink(search, hash)
  const backToSearch = typeof state?.fromSearch === 'string' && state.fromSearch.startsWith('/search?')
  const params = new URLSearchParams(search)

  const [point, setPoint] = useState<GrammarPointWithStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const { progress, refreshProgress } = useGrammarPracticeProgress(point?.jlpt_level ?? null, params)
  const listSearch = typeof state?.grammarListSearch === 'string'
    ? state.grammarListSearch
    : point ? new URLSearchParams({ level: point.jlpt_level }).toString() : ''
  const backPath = backToSearch ? state.fromSearch : withPracticeTask(listSearch ? `/grammar?${listSearch}` : '/grammar', params)
  const backLabel = t(backToSearch ? 'search.back' : 'grammar.detail.back')

  // Quiz state
  const [quizOpen, setQuizOpen] = useState(initialQuizOpen)
  const [answers, setAnswers] = useState<Record<number, string>>({})
  const [result, setResult] = useState<QuizResult | null>(null)
  const [submitting, setSubmitting] = useState(false)
  const questions = point?.quiz_questions ?? []
  const canSubmitQuiz = questions.length > 0 && questions.every(question => (answers[question.id] ?? '').trim() !== '')

  // Neighbor navigation
  const [prevId, setPrevId] = useState<number | null>(null)
  const [nextId, setNextId] = useState<number | null>(null)

  useEffect(() => {
    if (!id) return
    loadPoint(Number(id))
  }, [id])

  // 勘误「返回原题」带来的锚点：等内容渲染完成后再滚动到例句区或指定测验题。
  useEffect(() => {
    setQuizOpen(initialQuizOpen)
  }, [id, initialQuizOpen])

  const isQuizTarget = anchor?.startsWith('quiz-') ?? false
  useSourceAnchor(anchor, !loading && point?.id === Number(id) && (!isQuizTarget || quizOpen), isQuizTarget ? 'quiz' : 'examples')

  async function loadPoint(pointId: number) {
    setLoading(true)
    setError('')
    setAnswers({})
    setResult(null)
    setPrevId(null)
    setNextId(null)
    try {
      const data = await apiFetch<GrammarPointWithStatus>('GET', `/api/v1/grammar/${pointId}`)
      setPoint(data)
      // Fetch neighbors after point is loaded so we know the level
      loadNeighbors(pointId, data.jlpt_level)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }

  async function loadNeighbors(pointId: number, level: string) {
    try {
      const all = await apiFetch<GrammarPointWithStatus[]>('GET', `/api/v1/grammar?level=${level}`)
      if (!all || all.length === 0) return
      const ids = all.map((p) => p.id)
      const idx = ids.indexOf(pointId)
      setPrevId(idx > 0 ? ids[idx - 1] : null)
      setNextId(idx < ids.length - 1 ? ids[idx + 1] : null)
    } catch {
      // non-critical, ignore
    }
  }

  async function handleSubmitQuiz() {
    if (!point || !canSubmitQuiz || submitting) return
    setSubmitting(true)
    try {
      const submissions: QuizSubmission[] = point.quiz_questions.map((q) => ({
        question_id: q.id,
        answer: answers[q.id] ?? '',
      }))
      const res = await apiFetch<QuizResult>('POST', `/api/v1/grammar/${point.id}/quiz`, submissions)
      setResult(res)
      refreshProgress()
      const mistakes = buildGrammarMistakes(point.id, point.quiz_questions, res.results ?? [])
      for (const input of mistakes) {
        createMistake(input).catch(err => {
          console.error('[grammar] failed to record quiz mistake', input.item_key, err)
        })
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Submit failed')
    } finally {
      setSubmitting(false)
    }
  }

  function openNeighbor(pointId: number) {
    const path = `/grammar/${pointId}`
    navigate(fromTranslation ? translationRoundPath(params, path) : withPracticeTask(path, params), { state, replace: fromTranslation })
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
        {!fromTranslation && <Button variant="secondary" onClick={() => navigate(backPath)}>← {backLabel}</Button>}
      </div>
    )
  }

  return (
    <div className={styles.page}>
      {!fromTranslation && <button className={styles.back} onClick={() => navigate(backPath)}>
        ← {backLabel}
      </button>}

      <div className={styles.nameRow}>
        <h1 className={styles.name}>{point.name}</h1>
        <ErrataButton key={point.id} module="grammar" itemKey={String(point.id)} label={point.name} />
        <Badge level={point.jlpt_level} size="sm" />
        {point.user_status && <StatusBadge status={point.user_status} />}
      </div>
      <DailyPracticeProgress progress={progress} />
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
        <div className={styles.section} id="examples" tabIndex={-1}>
          <div className={styles.sectionTitle}>{t('grammar.detail.examples')}</div>
          <div className={styles.exampleList}>
            {point.examples.map((ex, i) => (
              <div key={ex.id ?? i} id={ex.id ? `example-${ex.id}` : undefined} tabIndex={ex.id ? -1 : undefined} className={styles.exampleItem}>
                <div className={styles.exampleJaRow}>
                  {ex.furigana_html ? (
                    <div className={styles.exampleJa} lang="ja" data-dictionary-text dangerouslySetInnerHTML={{ __html: ex.furigana_html }} />
                  ) : (
                    <div className={styles.exampleJa} lang="ja" data-dictionary-text>{ex.japanese}</div>
                  )}
                  <DictionaryButton text={ex.japanese} compact />
                  <button
                    className={styles.speakBtn}
                    onClick={() => speakExample(ex.japanese)}
                    title="🔊"
                    aria-label="Read aloud"
                  >
                    🔊
                  </button>
                  {ex.id && <ErrataButton module="grammar" itemKey={`${point.id}:example:${ex.id}`} label={ex.japanese} />}
                </div>
                <div className={styles.exampleZh}>{ex.chinese}</div>
              </div>
            ))}
          </div>
        </div>
      )}

      {/* Quiz */}
      {point.quiz_questions && point.quiz_questions.length > 0 && (
        <div className={styles.section} id="quiz" tabIndex={-1}>
          <button className={styles.quizToggle} aria-expanded={quizOpen} onClick={() => setQuizOpen((o) => !o)}>
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
                const answerParts = itemResult ? answerPartsFor(q.type, q.prompt) : null
                return (
                  <div key={q.id} id={`quiz-${q.id}`} tabIndex={-1} className={styles.question}>
                    <div className={styles.questionPrompt}>{q.prompt}</div>
                    <ErrataButton module="grammar" itemKey={`${point.id}:quiz:${q.id}`} label={q.prompt} />

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
                        {!itemResult.correct && itemResult.expected && !hasFullAnswer(itemResult.explanation ?? '') && (
                          <div className={styles.correctAnswer}>
                            {t('grammar.detail.quiz.correctAnswer')}：
                            {answerParts?.before}
                            <strong>{itemResult.expected}</strong>
                            {answerParts?.after}
                          </div>
                        )}
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
                  {!canSubmitQuiz && <span>{t('grammar.detail.quiz.incomplete')}</span>}
                  <Button onClick={handleSubmitQuiz} loading={submitting} disabled={!canSubmitQuiz}>
                    {t('grammar.detail.quiz.submit')}
                  </Button>
                </div>
              )}
            </div>
          )}
        </div>
      )}

      {/* Prev / Next navigation */}
      <div className={styles.navRow}>
        <Button
          variant="secondary"
          disabled={!prevId}
          onClick={() => prevId && openNeighbor(prevId)}
        >
          ← {t('grammar.detail.prev')}
        </Button>
        <Button
          variant="secondary"
          disabled={!nextId}
          onClick={() => nextId && openNeighbor(nextId)}
        >
          {t('grammar.detail.next')} →
        </Button>
      </div>
    </div>
  )
}
