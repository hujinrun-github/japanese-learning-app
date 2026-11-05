import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { apiFetch } from '@/api/client'
import { Spinner } from '@/components/ui/Spinner'
import type { TranslationSentence, TranslationRecord } from '@/types/api'
import { ErrataButton } from '@/components/ui/ErrataButton'
import styles from './TranslationPracticePage.module.css'

const grammarSourceLabels = { bank: '题库标注', ai: 'AI 分析', local: '本地规则' }

export function TranslationPracticePage() {
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const mode = searchParams.get('mode') || 'daily'
  const sourceID = searchParams.get('source_id')
  const review = searchParams.get('review') === 'true'

  const [sentences, setSentences] = useState<TranslationSentence[]>([])
  const [currentIdx, setCurrentIdx] = useState(0)
  const [userTranslation, setUserTranslation] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [currentResult, setCurrentResult] = useState<TranslationRecord | null>(null)
  const [aiPolling, setAiPolling] = useState(false)

  useEffect(() => {
    const controller = new AbortController()

    async function loadSentences() {
      setLoading(true)
      setError('')
      setSentences([])
      setCurrentIdx(0)
      setCurrentResult(null)
      setUserTranslation('')
      try {
        if (mode !== 'daily' && !sourceID) return
        const path = mode === 'daily'
          ? '/api/v1/translation/queue?count=5'
          : `/api/v1/translation/sentences?${new URLSearchParams({
              source_id: sourceID ?? '',
              unpracticed: String(!review),
            })}`
        const data = await apiFetch<TranslationSentence[]>('GET', path, undefined, controller.signal)
        if (!controller.signal.aborted) setSentences(data ?? [])
      } catch (err) {
        if (!controller.signal.aborted) setError(err instanceof Error ? err.message : '加载翻译题失败')
      } finally {
        if (!controller.signal.aborted) setLoading(false)
      }
    }

    void loadSentences()
    return () => controller.abort()
  }, [mode, sourceID, review])

  async function handleSubmit() {
    if (!userTranslation.trim()) return
    const sentence = sentences[currentIdx]
    setSubmitting(true)
    setCurrentResult(null)
    try {
      const rec = await apiFetch<TranslationRecord>('POST', '/api/v1/translation/submit', {
        sentence_id: sentence.id,
        user_translation: userTranslation.trim(),
      })
      setCurrentResult(rec)

      if (rec && !rec.ai_feedback && rec.score === rec.rule_score) {
        setAiPolling(true)
        pollForAIFeedback(rec.id)
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Submit failed')
    } finally {
      setSubmitting(false)
    }
  }

  async function pollForAIFeedback(recordID: number) {
    let attempts = 0
    const maxAttempts = 15
    const interval = setInterval(async () => {
      attempts++
      try {
        const rec = await apiFetch<TranslationRecord>('GET', `/api/v1/translation/records/${recordID}`)
        if (rec?.ai_feedback || attempts >= maxAttempts) {
          clearInterval(interval)
          setAiPolling(false)
          if (rec) setCurrentResult(current => current?.id === recordID ? rec : current)
        }
      } catch {
        clearInterval(interval)
        setAiPolling(false)
      }
    }, 2000)
  }

  function handleNext() {
    if (currentIdx < sentences.length - 1) {
      setCurrentIdx(currentIdx + 1)
      setUserTranslation('')
      setCurrentResult(null)
    }
  }

  if (loading) {
    return <div className={styles.page}><div className={styles.center}><Spinner size="lg" /></div></div>
  }

  if (sentences.length === 0) {
    const canReview = mode !== 'daily' && sourceID && !review && !error
    return (
      <div className={styles.page}>
        <button className={styles.backBtn} onClick={() => navigate('/translation')}>← 戻る</button>
        {error ? (
          <p className={styles.error} role="alert">{error}</p>
        ) : (
          <p className={styles.empty}>
            {mode === 'daily' ? '当前没有未练习的翻译题。'
              : !sourceID ? '请返回翻译页选择素材。'
              : review ? '这份素材暂无翻译题。'
              : '这份素材没有未练习的题。'}
          </p>
        )}
        {canReview && (
          <button
            className={styles.nextBtn}
            onClick={() => setSearchParams({ mode: 'free', source_id: sourceID, review: 'true' })}
          >
            重练本素材
          </button>
        )}
      </div>
    )
  }

  const sentence = sentences[currentIdx]
  const directionLabel = sentence.direction === 'jp2cn' ? '日→中' : '中→日'
  const reference = currentResult?.reference_translation?.trim() || currentResult?.ai_feedback?.reference_translation?.trim()
  const correction = currentResult?.ai_feedback?.corrected_translation?.trim()
  const grammarSource = currentResult?.ai_feedback?.grammar_source
  const grammar = grammarSource ? currentResult?.ai_feedback?.grammar_explanations : undefined

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <button className={styles.backBtn} onClick={() => navigate('/translation')}>← 戻る</button>
        <span className={styles.directionBadge}>{directionLabel}</span>
        <span className={styles.progress}>#{currentIdx + 1}/{sentences.length}</span>
      </div>

      <div className={styles.sourceText}>
        <p>{sentence.source_text}</p>
        <ErrataButton key={sentence.id} module="translation" itemKey={String(sentence.id)} label={sentence.source_text} />
      </div>

      <textarea
        className={styles.translateInput}
        placeholder="在此输入你的翻译…"
        value={userTranslation}
        onChange={e => setUserTranslation(e.target.value)}
        rows={4}
        disabled={!!currentResult}
      />

      {!currentResult && (
        <button
          className={styles.submitBtn}
          onClick={handleSubmit}
          disabled={submitting || !userTranslation.trim()}
        >
          {submitting ? <Spinner size="sm" /> : '提出する'}
        </button>
      )}

      {currentResult && (
        <div className={styles.result}>
          <div className={styles.scores}>
            <div className={styles.scoreItem}>
              <span className={styles.scoreLabel}>即时评分</span>
              <span className={styles.scoreValue}>{currentResult.rule_score}</span>
            </div>
            {currentResult.ai_feedback ? (
              <div className={styles.scoreItem}>
                <span className={styles.scoreLabel}>AI 评分</span>
                <span className={styles.scoreValue}>{currentResult.ai_feedback.ai_score}</span>
              </div>
            ) : aiPolling ? (
              <div className={styles.scoreItem}>
                <span className={styles.scoreLabel}>AI 评分中…</span>
                <Spinner size="sm" />
              </div>
            ) : null}
          </div>

          <section className={styles.referenceSection} aria-labelledby="translation-reference">
            <h2 className={styles.correctedLabel} id="translation-reference">参考译文</h2>
            {reference ? (
              <p className={styles.correctedText} lang={sentence.direction === 'jp2cn' ? 'zh' : 'ja'}>{reference}</p>
            ) : (
              <p className={styles.grammarEmpty}>{aiPolling ? '正在获取参考译文…' : '这道题暂无参考译文。'}</p>
            )}
          </section>

          {currentResult.ai_feedback && (
            <div className={styles.grammarSection}>
              <p className={styles.grammarLabel}>📚 涉及语法</p>
              {grammarSource && grammar?.length ? (
                <>
                  <p className={styles.grammarSource}>
                    {sentence.direction === 'jp2cn' ? '日语原文' : '日语参考译文'} · {grammarSourceLabels[grammarSource]}
                  </p>
                  {grammar.map(point => (
                    <div key={point.grammar_point} className={styles.grammarTag}>
                      <span className={styles.grammarPoint}>{point.grammar_point}</span>
                      {point.explanation && <span className={styles.grammarExplanation}>{point.explanation}</span>}
                      {point.evidence && <q className={styles.grammarEvidence} lang="ja">{point.evidence}</q>}
                    </div>
                  ))}
                </>
              ) : (
                <p className={styles.grammarEmpty}>本句暂无可靠的语法标注。</p>
              )}
            </div>
          )}

          {currentResult.ai_feedback?.issue_description && (
            <div className={styles.commentary}>
              <p className={styles.commentaryLabel}>💬 AI 点评</p>
              <p className={styles.commentaryText}>{currentResult.ai_feedback.issue_description}</p>
            </div>
          )}

          {correction && correction !== reference && (
            <div className={styles.correctedSection}>
              <p className={styles.correctedLabel}>修改建议</p>
              <p className={styles.correctedText}>{correction}</p>
            </div>
          )}
        </div>
      )}

      {currentResult && currentIdx < sentences.length - 1 && (
        <button className={styles.nextBtn} onClick={handleNext}>
          下一题 →
        </button>
      )}

      {error && <p className={styles.error}>{error}</p>}
    </div>
  )
}
