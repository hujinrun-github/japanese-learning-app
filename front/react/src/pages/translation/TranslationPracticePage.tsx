import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { apiFetch } from '@/api/client'
import { Spinner } from '@/components/ui/Spinner'
import type { TranslationSentence, TranslationRecord } from '@/types/api'
import styles from './TranslationPracticePage.module.css'

export function TranslationPracticePage() {
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const mode = searchParams.get('mode') || 'daily'
  const sourceID = searchParams.get('source_id')

  const [sentences, setSentences] = useState<TranslationSentence[]>([])
  const [currentIdx, setCurrentIdx] = useState(0)
  const [userTranslation, setUserTranslation] = useState('')
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [currentResult, setCurrentResult] = useState<TranslationRecord | null>(null)
  const [aiPolling, setAiPolling] = useState(false)

  useEffect(() => { loadSentences() }, [mode, sourceID])

  async function loadSentences() {
    setLoading(true)
    setError('')
    try {
      let data: TranslationSentence[] | null
      if (mode === 'daily') {
        data = await apiFetch<TranslationSentence[]>('GET', '/api/v1/translation/queue?count=5')
      } else if (sourceID) {
        data = await apiFetch<TranslationSentence[]>(
          'GET',
          `/api/v1/translation/sentences?source_id=${sourceID}`,
        )
      } else {
        setError('请选择素材来源')
        return
      }
      setSentences(data ?? [])
      setCurrentIdx(0)
      setCurrentResult(null)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load sentences')
    } finally {
      setLoading(false)
    }
  }

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
          if (rec) setCurrentResult(rec)
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
    return (
      <div className={styles.page}>
        <button className={styles.backBtn} onClick={() => navigate('/translation')}>← 戻る</button>
        <p className={styles.empty}>暂无练习内容。请先导入素材。</p>
      </div>
    )
  }

  const sentence = sentences[currentIdx]
  const directionLabel = sentence.direction === 'jp2cn' ? '日→中' : '中→日'

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <button className={styles.backBtn} onClick={() => navigate('/translation')}>← 戻る</button>
        <span className={styles.directionBadge}>{directionLabel}</span>
        <span className={styles.progress}>#{currentIdx + 1}/{sentences.length}</span>
      </div>

      <div className={styles.sourceText}>
        <p>{sentence.source_text}</p>
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

          {currentResult.ai_feedback?.grammar_explanations && currentResult.ai_feedback.grammar_explanations.length > 0 && (
            <div className={styles.grammarSection}>
              <p className={styles.grammarLabel}>📚 涉及语法</p>
              {currentResult.ai_feedback.grammar_explanations.map((g, i) => (
                <div key={i} className={styles.grammarTag}>
                  <span className={styles.grammarPoint}>{g.grammar_point}</span>
                  <span className={styles.grammarExplanation}>{g.explanation}</span>
                </div>
              ))}
            </div>
          )}

          {currentResult.ai_feedback?.issue_description && (
            <div className={styles.commentary}>
              <p className={styles.commentaryLabel}>💬 AI 点评</p>
              <p className={styles.commentaryText}>{currentResult.ai_feedback.issue_description}</p>
            </div>
          )}

          {currentResult.ai_feedback?.corrected_translation && (
            <div className={styles.correctedSection}>
              <p className={styles.correctedLabel}>修改建议</p>
              <p className={styles.correctedText}>{currentResult.ai_feedback.corrected_translation}</p>
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
