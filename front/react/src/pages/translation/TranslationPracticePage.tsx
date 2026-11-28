import { FavoriteButton } from '@/pages/favorites/FavoriteButton'
import { useTranslation } from 'react-i18next'
import { DailyPracticeProgress } from '@/components/ui/DailyPracticeProgress'
import { useDailyPracticeProgress } from '@/hooks/useDailyPracticeProgress'
import { Fragment, useState, useEffect, useLayoutEffect, useRef } from 'react'
import { Link, useNavigate, useOutlet, useSearchParams } from 'react-router-dom'
import { apiFetch } from '@/api/client'
import { Spinner } from '@/components/ui/Spinner'
import { Badge } from '@/components/ui/Badge'
import type { TranslationSentence, TranslationRecord } from '@/types/api'
import { ErrataButton } from '@/components/ui/ErrataButton'
import { DictionaryButton } from '@/pages/search/ContextualSearch'
import { grammarSearchPath, translationRoundPath } from './grammarLink'
import { PracticeKnowledgeLinks } from '@/pages/search/PracticeKnowledgeLinks'
import styles from './TranslationPracticePage.module.css'

const grammarSourceLabels = { bank: '题库标注', local: '本地规则' }

export function TranslationPracticePage() {
  const { t } = useTranslation()
  const navigate = useNavigate()
  const [searchParams] = useSearchParams()
  const grammar = useOutlet()
  const showingGrammar = grammar !== null
  const origin = useRef<{ scrollY: number; trigger: HTMLButtonElement } | null>(null)
  const returnButton = useRef<HTMLButtonElement>(null)

  useLayoutEffect(() => {
    if (showingGrammar) {
      window.scrollTo({ top: 0, behavior: 'instant' })
      returnButton.current?.focus({ preventScroll: true })
    } else if (origin.current) {
      window.scrollTo({ top: origin.current.scrollY, behavior: 'instant' })
      origin.current.trigger.focus({ preventScroll: true })
    }
  }, [showingGrammar])

  function openGrammar(point: string, trigger: HTMLButtonElement) {
    origin.current = { scrollY: window.scrollY, trigger }
    navigate(grammarSearchPath(point, searchParams))
  }

  function returnToPractice() {
    if (origin.current?.trigger.isConnected) navigate(-1)
    else navigate(translationRoundPath(searchParams), { replace: true })
  }

  return <>
    <div hidden={showingGrammar}><TranslationExercise onOpenGrammar={openGrammar} /></div>
    {showingGrammar && <>
      <div className={styles.grammarReturn}>
        <button className={styles.backBtn} type="button" ref={returnButton} onClick={returnToPractice}>
          ← {t(origin.current?.trigger.isConnected ? 'translation.backToQuestion' : 'translation.backToPractice')}
        </button>
      </div>
      {grammar}
    </>}
  </>
}

function TranslationExercise({ onOpenGrammar }: { onOpenGrammar: (point: string, trigger: HTMLButtonElement) => void }) {
  const { t } = useTranslation()
  const { progress, refreshProgress, jlptLevels } = useDailyPracticeProgress('translation')
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const retrySentenceID = searchParams.get('sentence_id')
  const mode = retrySentenceID ? 'mistake' : searchParams.get('mode') || 'daily'
  const fromFavorites = searchParams.get('from') === 'favorites'
  const backPath = fromFavorites ? '/favorites' : retrySentenceID ? '/mistakes' : '/translation'
  const backLabel = fromFavorites ? t('nav.favorites') : retrySentenceID ? t('translation.backToMistakes') : '戻る'
  const sourceID = searchParams.get('source_id')
  const review = searchParams.get('review') === 'true'

  const [sentences, setSentences] = useState<TranslationSentence[]>([])
  const [currentIdx, setCurrentIdx] = useState(0)
  const [userTranslation, setUserTranslation] = useState('')
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [currentResult, setCurrentResult] = useState<TranslationRecord | null>(null)

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
        if (!retrySentenceID && mode !== 'daily' && !sourceID) return
        const filters: Record<string, string> = retrySentenceID
          ? { sentence_id: retrySentenceID }
          : { source_id: sourceID ?? '', unpracticed: String(!review) }
        const path = mode === 'daily'
          ? '/api/v1/translation/queue'
          : `/api/v1/translation/sentences?${new URLSearchParams(filters)}`
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
  }, [mode, sourceID, review, retrySentenceID])

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
      refreshProgress()
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Submit failed')
    } finally {
      setSubmitting(false)
    }
  }

  function handleNext() {
    if (currentIdx < sentences.length - 1) {
      setCurrentIdx(currentIdx + 1)
      setUserTranslation('')
      setCurrentResult(null)
    }
  }

  if (loading || (mode === 'daily' && !error && progress.status === 'loading')) {
    return <div className={styles.page}><div className={styles.center}><Spinner size="lg" /></div></div>
  }

  if (sentences.length === 0) {
    const canReview = mode === 'free' && sourceID && !review && !error
    let dailyMessage = '当前没有未练习的翻译题。'
    let showLevelSettings = false
    if (mode === 'daily' && progress.status === 'ready') {
      if (progress.goal === 0) dailyMessage = '今天没有设置翻译练习目标。'
      else if (progress.completed >= progress.goal) dailyMessage = '今日翻译目标已完成。'
      else if (jlptLevels?.length) {
        dailyMessage = t('translation.emptyForLevels', { levels: jlptLevels.join(' / ') })
        showLevelSettings = !error
      }
    }
    return (
      <div className={styles.page}>
        <button className={styles.backBtn} onClick={() => navigate(backPath)}>← {backLabel}</button>
        <DailyPracticeProgress progress={progress} />
        {error ? (
          <p className={styles.error} role="alert">{error}</p>
        ) : (
          <p className={styles.empty}>
            {mode === 'daily' ? dailyMessage
              : !sourceID ? '请返回翻译页选择素材。'
              : review ? '这份素材暂无翻译题。'
              : '这份素材没有未练习的题。'}
          </p>
        )}
        {showLevelSettings && <Link className={styles.backBtn} to="/settings">{t('translation.levelSettings')}</Link>}
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
  const reference = currentResult?.reference_translation?.trim()
  const referenceTokens = currentResult?.reference_tokens
  const assessment = currentResult?.assessment
  const assessmentResult = assessment?.result ?? 'unrecognized'
  const grammarSource = assessment?.grammar_source
  const grammar = assessment?.grammar_explanations

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <button className={styles.backBtn} onClick={() => navigate(backPath)}>← {backLabel}</button>
        <span className={styles.directionBadge}>{directionLabel}</span>
        {sentence.jlpt_level && <Badge level={sentence.jlpt_level} size="sm" />}
        <span className={styles.progress}>{t('common.translationRound', { current: currentIdx + 1, total: sentences.length })}</span>
      </div>

      <DailyPracticeProgress progress={progress} />

      <div className={styles.sourceText}>
        <p lang={sentence.direction === 'jp2cn' ? 'ja' : 'zh'}
          data-dictionary-text={sentence.direction === 'jp2cn' ? '' : undefined}>{sentence.source_text}</p>
        {sentence.direction === 'jp2cn' && <DictionaryButton text={sentence.source_text} />}
        <FavoriteButton target={{ kind: 'translation', key: String(sentence.id) }} />
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
          <div className={styles.assessment} data-result={assessmentResult} role="status">
            <p className={styles.assessmentTitle}>{t(`translation.assessment.${assessmentResult}.title`)}</p>
            <p className={styles.assessmentDetail}>{t(`translation.assessment.${assessmentResult}.detail`)}</p>
            {assessmentResult === 'meaning_changed' && (
              <Link className={styles.mistakeLink} to="/mistakes">{t('translation.savedToMistakes')}</Link>
            )}
          </div>

          <section className={styles.referenceSection} aria-labelledby="translation-reference">
            <h2 className={styles.correctedLabel} id="translation-reference">参考译文</h2>
            {reference ? (
              <>
                <p className={styles.correctedText} lang={sentence.direction === 'jp2cn' ? 'zh' : 'ja'}
                  data-dictionary-text={sentence.direction === 'cn2jp' ? '' : undefined}>
                  {referenceTokens?.length ? referenceTokens.map(({ surface, reading }, index) => (
                    reading ? <ruby key={index}>{surface}<rt>{reading}</rt></ruby> : <Fragment key={index}>{surface}</Fragment>
                  )) : reference}
                </p>
                {sentence.direction === 'cn2jp' && <DictionaryButton text={reference} />}
              </>
            ) : (
              <p className={styles.grammarEmpty}>{t('translation.noReference')}</p>
            )}
          </section>

          <PracticeKnowledgeLinks links={currentResult.knowledge_links} />

          {assessment && (
            <div className={styles.grammarSection}>
              <p className={styles.grammarLabel}>📚 涉及语法</p>
              {grammarSource && grammar?.length ? (
                <>
                  <p className={styles.grammarSource}>
                    {sentence.direction === 'jp2cn' ? '日语原文' : '日语参考译文'} · {grammarSourceLabels[grammarSource]}
                  </p>
                  {grammar.map(point => (
                    <button
                      key={point.grammar_point}
                      type="button"
                      className={styles.grammarTag}
                      onClick={event => onOpenGrammar(point.grammar_point, event.currentTarget)}
                      title="查看语法详情"
                    >
                      <span className={styles.grammarPoint}>{point.grammar_point}</span>
                      {point.explanation && <span className={styles.grammarExplanation}>{point.explanation}</span>}
                      {point.evidence && <q className={styles.grammarEvidence} lang="ja">{point.evidence}</q>}
                      <span className={styles.grammarLinkArrow} aria-hidden="true">›</span>
                    </button>
                  ))}
                </>
              ) : (
                <p className={styles.grammarEmpty}>本句暂无可靠的语法标注。</p>
              )}
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
