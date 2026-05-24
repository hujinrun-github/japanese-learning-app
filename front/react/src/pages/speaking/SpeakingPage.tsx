import { useState, useEffect } from 'react'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { useAudioRecorder } from '@/hooks/useAudioRecorder'
import { toFuriganaTokens } from '@/util/furigana'
import { speakExample } from '@/util/exampleAudio'
import type { SpeakingRecord, SpeakingMaterial, FuriganaToken, JLPTLevel } from '@/types/api'
import styles from './SpeakingPage.module.css'

type PracticeType = 'shadow' | 'free'

const LEVELS: JLPTLevel[] = ['N5', 'N4', 'N3', 'N2', 'N1']
const FILTER_OPTIONS: { key: PracticeType | ''; labelKey: string }[] = [
  { key: '', labelKey: 'speaking.filter.all' },
  { key: 'shadow', labelKey: 'speaking.filter.shadow' },
  { key: 'free', labelKey: 'speaking.filter.free' },
]

const SPEAKER_COLORS = ['#228be6', '#2f9e44', '#e8590c', '#7950f2']

/**
 * Split Japanese text into sentences for per-line display.
 * Splits on 。！？ followed by optional 」or ).
 */
function splitSentences(text: string): string[] {
  const parts = text.split(/(?<=[。！？」])(?=[　-〿一-鿿぀-ゟ゠-ヿ])/)
  if (parts.length === 0) return [text]
  // Merge short fragments into previous sentence
  const result: string[] = []
  for (const p of parts) {
    if (result.length > 0 && p.length < 4 && result[result.length - 1].length + p.length < 40) {
      result[result.length - 1] += p
    } else {
      result.push(p)
    }
  }
  return result
}

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
  const [furiganaError, setFuriganaError] = useState(false)
  const [rawSentences, setRawSentences] = useState<string[]>([])
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
    setFuriganaError(false)
    if (recorder.audioURL) {
      URL.revokeObjectURL(recorder.audioURL)
    }
    const sentences = splitSentences(m.text)
    setRawSentences(sentences)
    try {
      const tokens = await toFuriganaTokens(m.text)
      setFuriganaTokens(tokens)
    } catch {
      setFuriganaError(true)
      setFuriganaTokens([])
    }
    fetchRecords()
  }

  function handleBackToList() {
    setSelectedMaterial(null)
    setFuriganaTokens([])
    setFuriganaError(false)
    setRawSentences([])
    setSubmitted(false)
  }

  function handlePlayTTS() {
    if (!selectedMaterial) return
    speakExample(selectedMaterial.text)
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

  // Group furigana tokens into sentence groups matching the raw sentence splits.
  function groupTokensBySentence(
    tokens: FuriganaToken[],
    sentences: string[],
  ): { tokens: FuriganaToken[]; raw: string }[] {
    if (tokens.length === 0) {
      return sentences.map((s) => ({ tokens: [], raw: s }))
    }
    const groups: { tokens: FuriganaToken[]; raw: string }[] = []
    let ti = 0
    let accum = ''
    let currentTokens: FuriganaToken[] = []
    for (let si = 0; si < sentences.length; si++) {
      currentTokens = []
      while (accum.length < sentences[si].length && ti < tokens.length) {
        currentTokens.push(tokens[ti])
        accum += tokens[ti].surface
        ti++
      }
      groups.push({ tokens: currentTokens, raw: sentences[si] })
    }
    // Remaining tokens (punctuation etc.) go into last group
    if (ti < tokens.length && groups.length > 0) {
      groups[groups.length - 1].tokens.push(...tokens.slice(ti))
    }
    return groups
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

      {/* furigana text display — grouped by sentence */}
      <div className={styles.textDisplay}>
        {furiganaError || furiganaTokens.length === 0 ? (
          rawSentences.length > 0 ? (
            rawSentences.map((s, i) => {
              const isDialog = selectedMaterial.type === 'shadow'
              const speakerIdx = isDialog ? i % 2 : -1
              return (
                <div key={i} className={styles.sentenceLine}>
                  {speakerIdx >= 0 && rawSentences.length > 1 && (
                    <span
                      className={styles.speakerLabel}
                      style={{ background: SPEAKER_COLORS[speakerIdx % SPEAKER_COLORS.length] }}
                    >
                      {String.fromCharCode(65 + speakerIdx)}
                    </span>
                  )}
                  <span>{s}</span>
                </div>
              )
            })
          ) : (
            <span>{selectedMaterial.text}</span>
          )
        ) : (
          groupTokensBySentence(furiganaTokens, rawSentences).map((group, gi) => {
            const isDialog = selectedMaterial.type === 'shadow'
            const speakerIdx = isDialog ? gi % 2 : -1
            return (
              <div key={gi} className={styles.sentenceLine}>
                {speakerIdx >= 0 && rawSentences.length > 1 && (
                  <span
                    className={styles.speakerLabel}
                    style={{ background: SPEAKER_COLORS[speakerIdx % SPEAKER_COLORS.length] }}
                  >
                    {String.fromCharCode(65 + speakerIdx)}
                  </span>
                )}
                {group.tokens.map((tok, ti) => (
                  <ruby key={ti} className={styles.token}>
                    {tok.surface}
                    <rt>{tok.reading}</rt>
                  </ruby>
                ))}
              </div>
            )
          })
        )}
      </div>

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
