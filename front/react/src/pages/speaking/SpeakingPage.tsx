import { useState, useEffect, useReducer, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { DailyPracticeProgress } from '@/components/ui/DailyPracticeProgress'
import { useDailyPracticeProgress } from '@/hooks/useDailyPracticeProgress'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { useWavRecorder } from '@/hooks/useWavRecorder'
import { APIError } from '@/api/client'
import { evaluateSpeaking } from '@/api/speaking'
import { ScoreCard } from '@/pages/shadowing/ScoreCard'
import { initialEvaluationState, speakingEvaluationReducer } from './speakingEvaluation'
import { SpeakingText, splitSpeakingTurns } from './SpeakingText'
import { toFuriganaTokens } from '@/util/furigana'
import { speakExample, playReferenceAudio, stopCurrent } from '@/util/exampleAudio'
import type { SpeakingRecord, SpeakingMaterial, FuriganaToken, JLPTLevel } from '@/types/api'
import { ErrataButton } from '@/components/ui/ErrataButton'
import styles from './SpeakingPage.module.css'

type PracticeType = 'shadow' | 'free'
type PracticeStatus = 'all' | 'unpracticed' | 'practiced'

const LEVELS: JLPTLevel[] = ['N5', 'N4', 'N3', 'N2', 'N1']
const FILTER_OPTIONS: { key: PracticeType | ''; labelKey: string }[] = [
  { key: '', labelKey: 'speaking.filter.all' },
  { key: 'shadow', labelKey: 'speaking.filter.shadow' },
  { key: 'free', labelKey: 'speaking.filter.free' },
]
const STATUS_FILTER_OPTIONS: { key: PracticeStatus; labelKey: string }[] = [
  { key: 'all', labelKey: 'speaking.filter.all' },
  { key: 'unpracticed', labelKey: 'speaking.filter.unpracticed' },
  { key: 'practiced', labelKey: 'speaking.practiced' },
]

export function SpeakingPage() {
  const { t } = useTranslation()
  const { progress, refreshProgress } = useDailyPracticeProgress('speaking')

  // material list state
  const [activeType, setActiveType] = useState<PracticeType | ''>('')
  const [activeLevel, setActiveLevel] = useState<JLPTLevel>('N5')
  const [activeStatus, setActiveStatus] = useState<PracticeStatus>('all')
  const [materials, setMaterials] = useState<SpeakingMaterial[]>([])
  const [materialsLoading, setMaterialsLoading] = useState(false)
  const [materialsError, setMaterialsError] = useState('')
  const [selectedMaterial, setSelectedMaterial] = useState<SpeakingMaterial | null>(null)
  const [practiceMaterials, setPracticeMaterials] = useState<SpeakingMaterial[]>([])

  // practice state
  const [furiganaLines, setFuriganaLines] = useState<FuriganaToken[][]>([])
  const [practiceLines, setPracticeLines] = useState<string[]>([])
  const [selfRating, setSelfRating] = useState(0)
  const [submitting, setSubmitting] = useState(false)
  const [submitted, setSubmitted] = useState(false)
  const [records, setRecords] = useState<SpeakingRecord[]>([])
  const [recordsLoading, setRecordsLoading] = useState(true)
  const [recordsError, setRecordsError] = useState(false)
  const [submissionError, setSubmissionError] = useState(false)
  const [historyExpanded, setHistoryExpanded] = useState(false)
  const [evaluationState, dispatchEvaluation] = useReducer(speakingEvaluationReducer, initialEvaluationState)
  const { result: evaluation, evaluating, error: evaluationError } = evaluationState
  const evaluationRequest = useRef<AbortController | null>(null)
  const requestID = useRef(0)
  const activeMaterialID = useRef<number | null>(null)
  const recorder = useWavRecorder()

  function resetEvaluation(materialID: number | null) {
    evaluationRequest.current?.abort()
    evaluationRequest.current = null
    activeMaterialID.current = materialID
    dispatchEvaluation({ type: 'reset', materialID, requestID: ++requestID.current })
  }

  useEffect(() => () => {
    ++requestID.current
    evaluationRequest.current?.abort()
    stopCurrent()
  }, [])

  // fetch materials on filter change
  useEffect(() => {
    fetchMaterials()
  }, [activeType, activeLevel])

  async function fetchMaterials() {
    setMaterialsLoading(true)
    setMaterialsError('')
    try {
      const params = new URLSearchParams()
      if (activeType) params.set('type', activeType)
      if (activeLevel) params.set('level', activeLevel)
      const data = await apiFetch<SpeakingMaterial[]>(
        'GET',
        `/api/v1/speaking/materials?${params.toString()}`,
      )
      setMaterials(data ?? [])
    } catch (err) {
      setMaterialsError(err instanceof Error ? err.message : 'Failed to load materials')
    } finally {
      setMaterialsLoading(false)
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    apiFetch<SpeakingRecord[]>('GET', '/api/v1/speaking/records', undefined, controller.signal)
      .then((data) => {
        if (controller.signal.aborted) return
        setRecords((current) => {
          const savedIds = new Set(current.map((record) => record.id))
          return [...current, ...(data ?? []).filter((record) => !savedIds.has(record.id))]
        })
        setRecordsError(false)
      })
      .catch(() => {
        if (!controller.signal.aborted) setRecordsError(true)
      })
      .finally(() => {
        if (!controller.signal.aborted) setRecordsLoading(false)
      })
    return () => controller.abort()
  }, [])

  const practicedMaterials = new Set(records.map((record) => record.material_id))
  const filteredMaterials = materials.filter((material) =>
    activeStatus === 'all' || practicedMaterials.has(material.id) === (activeStatus === 'practiced'),
  )
  // Keep this session's order when saving removes the current item from "unpracticed".
  const selectedMaterialIndex = practiceMaterials.findIndex((material) => material.id === selectedMaterial?.id)
  const nextMaterial = selectedMaterialIndex >= 0 ? practiceMaterials[selectedMaterialIndex + 1] : undefined

  function handleNextMaterial() {
    if (!nextMaterial || submitting) return
    void handleSelectMaterial(nextMaterial)
  }

  // select material and enter practice mode
  async function handleSelectMaterial(m: SpeakingMaterial) {
    resetEvaluation(m.id)
    recorder.reset()
    stopCurrent()
    setSelectedMaterial(m)
    setSubmitted(false)
    setSelfRating(0)
    setSubmissionError(false)
    setHistoryExpanded(false)
    const selectionID = requestID.current
    setFuriganaLines([])
    const lines = splitSpeakingTurns(m.text)
    setPracticeLines(lines)
    try {
      const tokens = await Promise.all(lines.map(toFuriganaTokens))
      if (requestID.current === selectionID) setFuriganaLines(tokens)
    } catch {
      if (requestID.current !== selectionID) return
      setFuriganaLines([])
    }
  }

  function handleBackToList() {
    resetEvaluation(null)
    recorder.reset()
    stopCurrent()
    setSelectedMaterial(null)
    setPracticeMaterials([])
    setFuriganaLines([])
    setPracticeLines([])
    setSubmitted(false)
  }

  async function handlePlayTTS() {
    if (!selectedMaterial) return
    const materialID = selectedMaterial.id
    const id = requestID.current
    try {
      const url = selectedMaterial.audio_url?.trim()
      if (url) await playReferenceAudio(url)
      else await speakExample(selectedMaterial.text)
    } catch {
      dispatchEvaluation({ type: 'failure', materialID, requestID: id, error: t('speaking.practice.noReference') })
    }
  }

  async function handleStartRecording() {
    if (!selectedMaterial || submitting) return
    resetEvaluation(selectedMaterial.id)
    stopCurrent()
    setSubmitted(false)
    setSubmissionError(false)
    setSelfRating(0)
    try { await recorder.start() } catch { /* Recorder displays capture errors. */ }
  }

  async function handleStopRecording() {
    const materialID = activeMaterialID.current
    const id = requestID.current
    if (materialID === null) return
    // Includes the asynchronous decoding phase, during which navigation can occur.
    dispatchEvaluation({ type: 'start', materialID, requestID: id })
    const controller = new AbortController()
    evaluationRequest.current = controller
    try {
      const recording = await recorder.stop()
      if (controller.signal.aborted || id !== requestID.current) return
      const result = await evaluateSpeaking({ audio: recording.blob, material_id: materialID }, controller.signal)
      dispatchEvaluation({ type: 'success', materialID, requestID: id, result })
    } catch (err) {
      if (controller.signal.aborted || id !== requestID.current) return
      dispatchEvaluation({
        type: 'failure', materialID, requestID: id,
        error: err instanceof APIError && err.code === 'ERR_SPEAKING_REFERENCE_MISSING'
          ? t('speaking.practice.noReference') : t('speaking.practice.evalError'),
      })
    } finally {
      dispatchEvaluation({ type: 'finish', materialID, requestID: id })
      if (evaluationRequest.current === controller) evaluationRequest.current = null
    }
  }

  const autoScore = evaluation?.status === 'scoreable' && typeof evaluation.overall_score === 'number'
    ? evaluation.overall_score
    : null
  const effectiveScore = autoScore ?? selfRating * 20

  // submit practice with the automatic score, falling back to self rating
  async function handleSubmit() {
    if (!selectedMaterial || (autoScore === null && selfRating === 0) || submitting || evaluating || recorder.isRecording) return
    setSubmitting(true)
    setSubmissionError(false)
    try {
      const record = await apiFetch<SpeakingRecord>('POST', '/api/v1/speaking/practice', {
        type: selectedMaterial.type,
        material_id: selectedMaterial.id,
        score: effectiveScore,
      })
      setRecords((current) => [record, ...current.filter((item) => item.id !== record.id)])
      setSubmitted(true)
      refreshProgress()
    } catch {
      setSubmissionError(true)
    } finally {
      setSubmitting(false)
    }
  }

  function formatDate(iso: string) {
    return new Date(iso).toLocaleDateString()
  }

  // ==== view: material list ====

  if (!selectedMaterial) {
    return (
      <div className={styles.page}>
        <h1 className={styles.title}>{t('speaking.title')}</h1>
        <DailyPracticeProgress progress={progress} />

        {/* filter bar: type toggles */}
        <div className={styles.filterBar}>
          {FILTER_OPTIONS.map((opt) => (
            <button
              key={opt.key}
              className={`${styles.filterBtn} ${activeType === opt.key ? styles.filterBtnActive : ''}`}
              onClick={() => setActiveType(opt.key)}
            >
              {t(opt.labelKey)}
            </button>
          ))}
        </div>

        <div className={styles.filterBar} role="group" aria-label={t('speaking.filter.status')}>
          {STATUS_FILTER_OPTIONS.map((opt) => (
            <button
              key={opt.key}
              type="button"
              className={`${styles.filterBtn} ${activeStatus === opt.key ? styles.filterBtnActive : ''}`}
              aria-pressed={activeStatus === opt.key}
              disabled={opt.key !== 'all' && (recordsLoading || recordsError)}
              onClick={() => setActiveStatus(opt.key)}
            >
              {t(opt.labelKey)}
            </button>
          ))}
        </div>

        {/* level tabs */}
        <div className={styles.levelTabs}>
          {LEVELS.map((lvl) => (
            <button
              key={lvl}
              className={`${styles.levelTab} ${activeLevel === lvl ? styles.levelTabActive : ''}`}
              onClick={() => setActiveLevel(lvl)}
            >
              {lvl}
            </button>
          ))}
        </div>

        {/* material list */}
        {recordsError && <p className={styles.errorMsg} role="alert">{t('speaking.historyError')}</p>}
        {materialsError && <p className={styles.errorMsg}>{materialsError}</p>}

        {materialsLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', paddingTop: '40px' }}>
            <Spinner size="lg" />
          </div>
        ) : filteredMaterials.length === 0 ? (
          <EmptyState icon="📚" title={t('speaking.materials.empty')} description="" />
        ) : (
          <div className={styles.materialList}>
            {filteredMaterials.map((m) => (
              <button
                key={m.id}
                type="button"
                className={styles.materialCard}
                onClick={() => {
                  setPracticeMaterials(filteredMaterials)
                  void handleSelectMaterial(m)
                }}
              >
                <span className={styles.cardHeader}>
                  <span className={styles.cardTitle}>{m.title}</span>
                  <span className={`${styles.typeTag} ${m.type === 'shadow' ? styles.typeShadow : styles.typeFree}`}>
                    {m.type === 'shadow' ? t('speaking.materials.shadowLabel') : t('speaking.materials.freeLabel')}
                  </span>
                </span>
                <span className={styles.cardPreview}>{m.text.slice(0, 80)}{m.text.length > 80 ? '…' : ''}</span>
                <span className={styles.cardFooter}>
                  <span className={styles.levelBadge}>{m.jlpt_level}</span>
                  {practicedMaterials.has(m.id) && <span className={styles.practiced}>{t('speaking.practiced')}</span>}
                  <span className={styles.cardArrow} aria-hidden="true">→</span>
                </span>
              </button>
            ))}
          </div>
        )}
      </div>
    )
  }

  // ==== view: practice mode ====

  return (
    <div className={styles.page}>
      <div className={styles.practiceNav}>
        <button type="button" className={styles.backBtn} onClick={handleBackToList} disabled={submitting}>
          {t('speaking.practice.back')}
        </button>
        {selectedMaterialIndex >= 0 && (
          <span className={styles.materialPosition} aria-live="polite">
            {t('speaking.practice.position', { current: selectedMaterialIndex + 1, total: practiceMaterials.length })}
          </span>
        )}
        <button type="button" className={styles.nextBtn} onClick={handleNextMaterial} disabled={!nextMaterial || submitting}>
          {t(nextMaterial ? 'speaking.practice.next' : 'speaking.practice.lastMaterial')}
        </button>
      </div>

      {/* practice header */}
      <div className={styles.practiceHeader}>
        <h1 className={styles.practiceTitle}>{selectedMaterial.title}</h1>
        <ErrataButton module="speaking" itemKey={String(selectedMaterial.id)} label={selectedMaterial.title} />
        <span className={`${styles.typeTag} ${selectedMaterial.type === 'shadow' ? styles.typeShadow : styles.typeFree}`}>
          {selectedMaterial.type === 'shadow' ? t('speaking.materials.shadowLabel') : t('speaking.materials.freeLabel')}
        </span>
      </div>

      <DailyPracticeProgress progress={progress} />

      {/* Keep annotations aligned with each complete speaker turn. */}
      <SpeakingText lines={practiceLines} annotations={furiganaLines} />

      {/* toolbar */}
      <div className={styles.toolbar}>
        {/* TTS (shadowing mode) */}
        {selectedMaterial.type === 'shadow' && (
          <button className={styles.toolBtn} onClick={handlePlayTTS}>
            {t('speaking.practice.playRef')}
          </button>
        )}

        {/* recording */}
        {recorder.isRecording ? (
          <button className={`${styles.toolBtn} ${styles.toolBtnDanger}`} onClick={handleStopRecording} disabled={evaluating}>
            <span className={styles.recordingIndicator} />{t('speaking.practice.stopRecord')}
          </button>
        ) : (
          <button
            className={styles.toolBtn}
            onClick={handleStartRecording}
            disabled={submitting || (evaluating && !recorder.recording)}
          >
            {recorder.recording ? t('speaking.practice.reRecord') : t('speaking.practice.startRecord')}
          </button>
        )}
      </div>

      {/* playback */}
      {recorder.recording && (
        <div style={{ marginTop: 'var(--space-4)' }}>
          <p style={{ fontSize: 'var(--font-size-sm)', fontWeight: 600, marginBottom: 'var(--space-2)' }}>
            {t('speaking.practice.playback')}
          </p>
          <audio controls src={recorder.recording.url} style={{ width: '100%' }} />
        </div>
      )}

      {recorder.error && (
        <p style={{ color: 'var(--color-error)', fontSize: 'var(--font-size-sm)', marginTop: 'var(--space-2)' }}>
          {recorder.error}
        </p>
      )}

      <p className={styles.scoringHint}>{t('speaking.practice.scoringHint')}</p>
      {evaluating && <p className={styles.errorMsg} role="status">{t('speaking.practice.scoring')}</p>}
      {evaluationError && <p className={styles.errorMsg} role="alert">{evaluationError}</p>}
      <ScoreCard result={evaluation} />

      {/* self rating */}
      <div className={styles.ratingSection}>
        <p className={styles.ratingLabel}>{t('speaking.practice.selfRate')}</p>
        {autoScore !== null && <p className={styles.autoScoreNote}>{t('speaking.practice.autoScoreUsed')}</p>}
        <div className={styles.stars}>
          {[1, 2, 3, 4, 5].map((i) => (
            <button
              key={i}
              className={`${styles.star} ${i <= selfRating ? styles.starActive : ''}`}
              onClick={() => setSelfRating(i)}
            >
              {i <= selfRating ? '★' : '☆'}
            </button>
          ))}
        </div>
      </div>

      {/* submit */}
      <button
        className={styles.submitBtn}
        onClick={handleSubmit}
        disabled={(autoScore === null && selfRating === 0) || submitting || submitted || evaluating || recorder.isRecording}
      >
        {submitting ? <Spinner size="sm" /> : t('speaking.practice.submit')}
      </button>

      {submissionError && <p className={styles.errorMsg} role="alert">{t('speaking.saveError')}</p>}
      {submitted && (
        <p className={styles.successMsg}>{t('speaking.practice.submitSuccess')}</p>
      )}

      {/* history */}
      <div className={styles.historySection}>
        <button
          className={styles.historyToggle}
          aria-expanded={historyExpanded}
          onClick={() => setHistoryExpanded((expanded) => !expanded)}
        >
          {t('speaking.records.title')} {historyExpanded ? '▲' : '▼'}
        </button>

        {historyExpanded && (
          recordsLoading ? (
            <div style={{ display: 'flex', justifyContent: 'center', paddingTop: '20px' }}>
              <Spinner size="sm" />
            </div>
          ) : recordsError && records.length === 0 ? (
            <p className={styles.errorMsg} role="alert">{t('speaking.historyError')}</p>
          ) : records.length === 0 ? (
            <EmptyState icon="🎙️" title={t('speaking.records.empty')} description="" />
          ) : (
            <div className={styles.recordList}>
              {records.map((rec) => (
                <div key={rec.id} className={styles.recordItem}>
                  <span className={styles.recordDate}>{formatDate(rec.practiced_at)}</span>
                  <span className={styles.scoreBadge}>
                    {t('speaking.score')} {rec.score}
                  </span>
                  <StatusBadge status={rec.score >= 80 ? 'pass' : 'needs_work'} />
                </div>
              ))}
            </div>
          )
        )}
      </div>
    </div>
  )
}
