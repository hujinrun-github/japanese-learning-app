import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { useAudioRecorder } from '@/hooks/useAudioRecorder'
import { toFuriganaTokens, charIndexToTokenIndex } from '@/util/furigana'
import type { SpeakingRecord, SpeakingMaterial, FuriganaToken, JLPTLevel } from '@/types/api'
import styles from './SpeakingPage.module.css'

type PracticeType = 'shadow' | 'free'

const LEVELS: JLPTLevel[] = ['N5', 'N4', 'N3']
const FILTER_OPTIONS: { key: PracticeType | ''; labelKey: string }[] = [
  { key: '', labelKey: 'speaking.filter.all' },
  { key: 'shadow', labelKey: 'speaking.filter.shadow' },
  { key: 'free', labelKey: 'speaking.filter.free' },
]

export function SpeakingPage() {
  const { t } = useTranslation()

  // material list state
  const [activeType, setActiveType] = useState<PracticeType | ''>('')
  const [activeLevel, setActiveLevel] = useState<JLPTLevel>('N5')
  const [materials, setMaterials] = useState<SpeakingMaterial[]>([])
  const [materialsLoading, setMaterialsLoading] = useState(false)
  const [materialsError, setMaterialsError] = useState('')
  const [selectedMaterial, setSelectedMaterial] = useState<SpeakingMaterial | null>(null)

  // practice state
  const [furiganaTokens, setFuriganaTokens] = useState<FuriganaToken[]>([])
  const [highlightIdx, setHighlightIdx] = useState(-1)
  const [isPlaying, setIsPlaying] = useState(false)
  const [selfRating, setSelfRating] = useState(0)
  const [submitting, setSubmitting] = useState(false)
  const [submitted, setSubmitted] = useState(false)
  const [records, setRecords] = useState<SpeakingRecord[]>([])
  const [recordsLoading, setRecordsLoading] = useState(false)
  const [historyExpanded, setHistoryExpanded] = useState(false)

  const recorder = useAudioRecorder()

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

  async function fetchRecords() {
    setRecordsLoading(true)
    try {
      const data = await apiFetch<SpeakingRecord[]>('GET', '/api/v1/speaking/records')
      setRecords(data ?? [])
    } catch {
      // silently fail for history
    } finally {
      setRecordsLoading(false)
    }
  }

  // select material and enter practice mode
  async function handleSelectMaterial(m: SpeakingMaterial) {
    setSelectedMaterial(m)
    setSubmitted(false)
    setSelfRating(0)
    setHighlightIdx(-1)
    setIsPlaying(false)
    speechSynthesis.cancel()
    if (recorder.audioURL) {
      URL.revokeObjectURL(recorder.audioURL)
    }
    const tokens = await toFuriganaTokens(m.text)
    setFuriganaTokens(tokens)
    fetchRecords()
  }

  function handleBackToList() {
    setSelectedMaterial(null)
    setFuriganaTokens([])
    setHighlightIdx(-1)
    setIsPlaying(false)
    speechSynthesis.cancel()
    setSubmitted(false)
  }

  // TTS playback with token highlighting
  function handlePlayTTS() {
    if (!selectedMaterial) return
    speechSynthesis.cancel()
    const utterance = new SpeechSynthesisUtterance(selectedMaterial.text)
    utterance.lang = 'ja-JP'
    utterance.rate = 0.9

    utterance.onboundary = (e) => {
      const idx = charIndexToTokenIndex(e.charIndex, furiganaTokens)
      setHighlightIdx(idx)
    }
    utterance.onend = () => {
      setIsPlaying(false)
      setHighlightIdx(-1)
    }
    utterance.onerror = () => {
      setIsPlaying(false)
      setHighlightIdx(-1)
    }

    setIsPlaying(true)
    speechSynthesis.speak(utterance)
  }

  function handlePauseTTS() {
    speechSynthesis.cancel()
    setIsPlaying(false)
    setHighlightIdx(-1)
  }

  // recording
  async function handleStartRecording() {
    try {
      await recorder.start()
    } catch {
      // permission denied handled by hook
    }
  }

  async function handleStopRecording() {
    try {
      await recorder.stop()
    } catch {
      // error handled by hook
    }
  }

  // submit self-rated practice
  async function handleSubmit() {
    if (!selectedMaterial || selfRating === 0) return
    setSubmitting(true)
    try {
      await apiFetch('POST', '/api/v1/speaking/practice', {
        type: selectedMaterial.type,
        material_id: selectedMaterial.id,
        score: selfRating * 20,
      })
      setSubmitted(true)
      fetchRecords()
    } catch (err) {
      // error swallowed per plan — user can retry
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
        {materialsError && <p className={styles.errorMsg}>{materialsError}</p>}

        {materialsLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', paddingTop: '40px' }}>
            <Spinner size="lg" />
          </div>
        ) : materials.length === 0 ? (
          <EmptyState icon="📚" title="No materials found" description="" />
        ) : (
          <div className={styles.materialList}>
            {materials.map((m) => (
              <div
                key={m.id}
                className={styles.materialCard}
                onClick={() => handleSelectMaterial(m)}
              >
                <div className={styles.cardHeader}>
                  <span className={styles.cardTitle}>{m.title}</span>
                  <span className={`${styles.typeTag} ${m.type === 'shadow' ? styles.typeShadow : styles.typeFree}`}>
                    {m.type === 'shadow' ? t('speaking.materials.shadowLabel') : t('speaking.materials.freeLabel')}
                  </span>
                </div>
                <p className={styles.cardPreview}>{m.text.slice(0, 80)}{m.text.length > 80 ? '…' : ''}</p>
                <div className={styles.cardFooter}>
                  <span className={styles.levelBadge}>{m.jlpt_level}</span>
                  <span className={styles.cardArrow}>→</span>
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    )
  }

  // ==== view: practice mode ====

  return (
    <div className={styles.page}>
      {/* back button */}
      <button className={styles.backBtn} onClick={handleBackToList}>
        {t('speaking.practice.back')}
      </button>

      {/* practice header */}
      <div className={styles.practiceHeader}>
        <h1 className={styles.practiceTitle}>{selectedMaterial.title}</h1>
        <span className={`${styles.typeTag} ${selectedMaterial.type === 'shadow' ? styles.typeShadow : styles.typeFree}`}>
          {selectedMaterial.type === 'shadow' ? t('speaking.materials.shadowLabel') : t('speaking.materials.freeLabel')}
        </span>
      </div>

      {/* furigana text display */}
      <div className={styles.textDisplay}>
        {furiganaTokens.map((tok, i) => (
          <ruby
            key={i}
            className={`${styles.token} ${i === highlightIdx ? styles.tokenHighlight : ''}`}
          >
            {tok.surface}
            <rt>{tok.reading}</rt>
          </ruby>
        ))}
      </div>

      {/* toolbar */}
      <div className={styles.toolbar}>
        {/* TTS (shadowing mode) */}
        {selectedMaterial.type === 'shadow' && (
          isPlaying ? (
            <button className={styles.toolBtn} onClick={handlePauseTTS}>
              {t('speaking.practice.pauseRef')}
            </button>
          ) : (
            <button className={styles.toolBtn} onClick={handlePlayTTS}>
              {t('speaking.practice.playRef')}
            </button>
          )
        )}

        {/* recording */}
        {recorder.isRecording ? (
          <button className={`${styles.toolBtn} ${styles.toolBtnDanger}`} onClick={handleStopRecording}>
            <span className={styles.recordingIndicator} />{t('speaking.practice.stopRecord')}
          </button>
        ) : (
          <button
            className={styles.toolBtn}
            onClick={recorder.audioURL ? undefined : handleStartRecording}
            disabled={!!recorder.error}
          >
            {recorder.audioURL ? t('speaking.practice.reRecord') : t('speaking.practice.startRecord')}
          </button>
        )}
      </div>

      {/* playback */}
      {recorder.audioURL && (
        <div style={{ marginTop: 'var(--space-4)' }}>
          <p style={{ fontSize: 'var(--font-size-sm)', fontWeight: 600, marginBottom: 'var(--space-2)' }}>
            {t('speaking.practice.playback')}
          </p>
          <audio controls src={recorder.audioURL} style={{ width: '100%' }} />
        </div>
      )}

      {recorder.error && (
        <p style={{ color: 'var(--color-error)', fontSize: 'var(--font-size-sm)', marginTop: 'var(--space-2)' }}>
          {recorder.error}
        </p>
      )}

      {/* self rating */}
      <div className={styles.ratingSection}>
        <p className={styles.ratingLabel}>{t('speaking.practice.selfRate')}</p>
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
        disabled={selfRating === 0 || submitting || submitted}
      >
        {submitting ? <Spinner size="sm" /> : t('speaking.practice.submit')}
      </button>

      {submitted && (
        <p className={styles.successMsg}>{t('speaking.practice.submitSuccess')}</p>
      )}

      {/* history */}
      <div className={styles.historySection}>
        <button
          className={styles.historyToggle}
          onClick={() => { setHistoryExpanded(!historyExpanded); if (!historyExpanded) fetchRecords() }}
        >
          {t('speaking.records.title')} {historyExpanded ? '▲' : '▼'}
        </button>

        {historyExpanded && (
          recordsLoading ? (
            <div style={{ display: 'flex', justifyContent: 'center', paddingTop: '20px' }}>
              <Spinner size="sm" />
            </div>
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
