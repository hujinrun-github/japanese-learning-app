import { useEffect, useRef, useState } from 'react'
import { Link, useParams } from 'react-router-dom'
import { APIError } from '@/api/client'
import { getShadowingSession, saveShadowingAttempt, saveShadowingProgress } from '@/api/shadowing'
import { useAudioRecorder } from '@/hooks/useAudioRecorder'
import type { PracticeMode, ShadowingSession as ShadowingSessionDTO } from '@/types/shadowing'
import { getCurrentSentenceIndex } from '@/util/shadowing/currentSentence'
import { selectShadowingMedia } from '@/util/shadowing/media'
import styles from './ShadowingPage.module.css'

export function ShadowingPage() {
  const { id } = useParams()
  const mediaRef = useRef<HTMLMediaElement | null>(null)
  const recordingAudioRef = useRef<HTMLAudioElement | null>(null)
  const progressTimeoutRef = useRef<number | null>(null)
  const loopCountRef = useRef(0)
  const loopSavingRef = useRef(false)
  const recorder = useAudioRecorder()

  const [session, setSession] = useState<ShadowingSessionDTO | null>(null)
  const [currentSentenceIndex, setCurrentSentenceIndex] = useState<number | null>(null)
  const [playbackRate, setPlaybackRate] = useState(1)
  const [looping, setLooping] = useState(false)
  const [loopCount, setLoopCount] = useState(0)
  const [selfScore, setSelfScore] = useState(80)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [saving, setSaving] = useState(false)

  async function loadSession(showSpinner = true, signal?: AbortSignal) {
    const lessonID = Number(id)
    if (!Number.isInteger(lessonID) || lessonID <= 0) {
      setError('Invalid lesson id')
      setLoading(false)
      return
    }

    if (showSpinner) setLoading(true)
    setError('')
    try {
      const data = await getShadowingSession(lessonID, signal)
      if (signal?.aborted) return
      setSession(data)
      setLooping(false)
      setLoopCountValue(0)
      setPlaybackRate(data.progress?.last_practice_mode === 'slow' ? 0.75 : 1)
      setCurrentSentenceIndex(data.progress?.last_sentence_index ?? data.lesson.sentences[0]?.index ?? null)
    } catch (err) {
      if (signal?.aborted) return
      setError(err instanceof Error ? err.message : 'Failed to load shadowing session')
    } finally {
      if (!signal?.aborted && showSpinner) setLoading(false)
    }
  }

  useEffect(() => {
    const controller = new AbortController()
    void loadSession(true, controller.signal)
    return () => controller.abort()
  }, [id])

  useEffect(() => {
    return () => {
      if (progressTimeoutRef.current !== null) {
        window.clearTimeout(progressTimeoutRef.current)
      }
    }
  }, [])

  useEffect(() => {
    function handleBeforeUnload() {
      void saveProgressNow()
    }

    window.addEventListener('beforeunload', handleBeforeUnload)
    return () => window.removeEventListener('beforeunload', handleBeforeUnload)
  }, [session, currentSentenceIndex, looping, playbackRate])

  function currentSentence() {
    if (!session || currentSentenceIndex === null) return null
    return session.lesson.sentences.find((sentence) => sentence.index === currentSentenceIndex) ?? null
  }

  function setLoopCountValue(value: number) {
    loopCountRef.current = value
    setLoopCount(value)
  }

  function handleLoadedMetadata() {
    const media = mediaRef.current
    if (!media || !session?.progress) return
    media.currentTime = session.progress.last_position_ms / 1000
    media.playbackRate = session.progress.last_practice_mode === 'slow' ? 0.75 : 1
    setCurrentSentenceIndex(session.progress.last_sentence_index)
  }

  function handleRateChange() {
    const media = mediaRef.current
    if (media) setPlaybackRate(media.playbackRate)
  }

  function handleTimeUpdate() {
    const media = mediaRef.current
    if (!media || !session) return

    const currentTimeMs = Math.floor(media.currentTime * 1000)
    const next = getCurrentSentenceIndex(session.lesson.sentences, currentTimeMs)
    if (next !== currentSentenceIndex) {
      setCurrentSentenceIndex(next)
      if (next !== null) scheduleProgressSave(next)
    }

    if (!looping || currentSentenceIndex === null) return
    const sentence = session.lesson.sentences.find((item) => item.index === currentSentenceIndex)
    if (!sentence || currentTimeMs < sentence.end_ms) return

    media.currentTime = sentence.start_ms / 1000
    const nextLoopCount = loopCountRef.current + 1
    const requiredLoopCount = getConfiguredLoopCount(session)
    if (nextLoopCount < requiredLoopCount) {
      setLoopCountValue(nextLoopCount)
      return
    }

    setLoopCountValue(0)
    if (!loopSavingRef.current) {
      loopSavingRef.current = true
      void saveLoopAttempt(sentence.index, requiredLoopCount).finally(() => {
        loopSavingRef.current = false
      })
    }
  }

  function seekToSentence(index: number) {
    const media = mediaRef.current
    const sentence = session?.lesson.sentences.find((item) => item.index === index)
    if (!media || !sentence) return
    media.currentTime = sentence.start_ms / 1000
    setCurrentSentenceIndex(sentence.index)
    scheduleProgressSave(sentence.index)
  }

  function seekPrevious() {
    if (!session || currentSentenceIndex === null) return
    const currentPosition = session.lesson.sentences.findIndex((sentence) => sentence.index === currentSentenceIndex)
    if (currentPosition <= 0) return
    seekToSentence(session.lesson.sentences[currentPosition - 1].index)
  }

  function replayCurrent() {
    const media = mediaRef.current
    if (!media || currentSentenceIndex === null) return
    seekToSentence(currentSentenceIndex)
    media.playbackRate = playbackRate
    void media.play()
  }

  function playSlow() {
    const media = mediaRef.current
    if (!media || currentSentenceIndex === null) return
    media.playbackRate = 0.75
    setPlaybackRate(0.75)
    seekToSentence(currentSentenceIndex)
    void media.play()
  }

  function toggleLoop() {
    const media = mediaRef.current
    if (!media || currentSentenceIndex === null) return
    const nextLooping = !looping
    setLooping(nextLooping)
    setLoopCountValue(0)
    if (nextLooping) {
      seekToSentence(currentSentenceIndex)
      void media.play()
    }
  }

  function playRecording() {
    void recordingAudioRef.current?.play()
  }

  async function toggleRecording() {
    if (currentSentenceIndex === null || !session) return
    setError('')
    try {
      if (!recorder.isRecording) {
        await recorder.start()
        return
      }

      await recorder.stop()
      await saveRecordAttempt(currentSentenceIndex)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to record audio')
    }
  }

  function scheduleProgressSave(sentenceIndex: number) {
    if (progressTimeoutRef.current !== null) {
      window.clearTimeout(progressTimeoutRef.current)
    }
    progressTimeoutRef.current = window.setTimeout(() => {
      void saveProgressNow(sentenceIndex)
    }, 800)
  }

  async function saveProgressNow(sentenceIndex = currentSentenceIndex) {
    if (!session || sentenceIndex === null) return
    const media = mediaRef.current
    try {
      await saveShadowingProgress(session.lesson.id, {
        shadowing_version: session.lesson.shadowing_version,
        last_sentence_index: sentenceIndex,
        last_position_ms: Math.floor((media?.currentTime ?? 0) * 1000),
        last_practice_mode: progressMode(),
      })
    } catch (err) {
      await handleWriteError(err)
    }
  }

  async function saveLoopAttempt(sentenceIndex: number, completedLoopCount: number) {
    if (!session) return
    setSaving(true)
    try {
      await saveShadowingAttempt(session.lesson.id, {
        shadowing_version: session.lesson.shadowing_version,
        sentence_index: sentenceIndex,
        practice_mode: 'loop',
        playback_rate: playbackRate,
        loop_count: completedLoopCount,
      })
      setNotice(`Loop attempt saved (${completedLoopCount}x).`)
      await loadSession(false)
    } catch (err) {
      await handleWriteError(err)
    } finally {
      setSaving(false)
    }
  }

  async function saveRecordAttempt(sentenceIndex: number) {
    if (!session) return
    setSaving(true)
    try {
      await saveShadowingAttempt(session.lesson.id, {
        shadowing_version: session.lesson.shadowing_version,
        sentence_index: sentenceIndex,
        practice_mode: 'record',
        playback_rate: playbackRate,
        loop_count: 0,
        self_score: selfScore,
      })
      setNotice('Recording attempt saved.')
      await loadSession(false)
    } catch (err) {
      await handleWriteError(err)
    } finally {
      setSaving(false)
    }
  }

  async function handleWriteError(err: unknown) {
    if (err instanceof APIError && err.code === 'ERR_SHADOWING_VERSION_STALE') {
      setNotice('Content changed. Session refreshed.')
      await loadSession(false)
      return
    }
    setError(err instanceof Error ? err.message : 'Failed to save shadowing progress')
  }

  function progressMode(): PracticeMode {
    if (looping) return 'loop'
    if (playbackRate === 0.75) return 'slow'
    return 'normal'
  }

  if (loading) {
    return (
      <div className={styles.page}>
        <p className={styles.intro}>Loading shadowing session...</p>
      </div>
    )
  }

  if (error && !session) {
    return (
      <div className={styles.page}>
        <Link className={styles.backLink} to={id ? `/lesson/${id}` : '/lesson'}>
          返回课文
        </Link>
        <p className={styles.error}>{error}</p>
      </div>
    )
  }

  if (!session) return null

  const sentence = currentSentence()
  const completed = new Set(session.completed_sentence_indexes)
  const media = selectShadowingMedia(session.lesson)

  return (
    <div className={styles.page}>
      <Link className={styles.backLink} to={`/lesson/${session.lesson.id}`}>
        返回课文
      </Link>

      <header className={styles.header}>
        <div>
          <p className={styles.eyebrow}>{media.kind === 'video' ? 'Video Shadowing' : 'Audio Shadowing'}</p>
          <h1 className={styles.title}>{session.lesson.title}</h1>
        </div>
        <div className={styles.progressPill}>
          {session.completed_sentence_count}/{session.lesson.sentences.length} completed
        </div>
      </header>

      {error && <p className={styles.error}>{error}</p>}
      {notice && <p className={styles.notice}>{notice}</p>}

      <section className={styles.playerCard}>
        {media.kind === 'video' ? (
          <video
            ref={(node) => { mediaRef.current = node }}
            controls
            src={media.url}
            onLoadedMetadata={handleLoadedMetadata}
            onRateChange={handleRateChange}
            onTimeUpdate={handleTimeUpdate}
          />
        ) : (
          <audio
            ref={(node) => { mediaRef.current = node }}
            controls
            src={media.url}
            onLoadedMetadata={handleLoadedMetadata}
            onRateChange={handleRateChange}
            onTimeUpdate={handleTimeUpdate}
          />
        )}
      </section>

      <section className={styles.currentCard}>
        <div className={styles.currentLabel}>Current sentence</div>
        {sentence ? (
          <>
            <div className={styles.sentenceJa}>
              {sentence.tokens.map((token, index) =>
                token.reading ? (
                  <ruby key={index}>
                    {token.surface}
                    <rt>{token.reading}</rt>
                  </ruby>
                ) : (
                  <span key={index}>{token.surface}</span>
                )
              )}
            </div>
            <p className={styles.sentenceZh}>{sentence.chinese}</p>
          </>
        ) : (
          <p className={styles.sentenceZh}>Select a sentence to start.</p>
        )}
      </section>

      <section className={styles.controls} aria-label="Shadowing controls">
        <button onClick={seekPrevious} disabled={currentSentenceIndex === null}>Previous</button>
        <button onClick={replayCurrent} disabled={currentSentenceIndex === null}>Replay</button>
        <button onClick={playSlow} disabled={currentSentenceIndex === null}>Slow 0.75x</button>
        <button className={looping ? styles.activeButton : ''} onClick={toggleLoop} disabled={currentSentenceIndex === null}>
          {looping ? `Looping ${loopCount}/${getConfiguredLoopCount(session)}` : 'Loop'}
        </button>
        <button onClick={toggleRecording} disabled={currentSentenceIndex === null || saving}>
          {recorder.isRecording ? 'Stop recording' : 'Record'}
        </button>
      </section>

      <label className={styles.scoreControl}>
        Self score
        <input
          type="number"
          min={0}
          max={100}
          value={selfScore}
          onChange={(event) => setSelfScore(clampScore(Number(event.target.value)))}
        />
      </label>

      {recorder.error && <p className={styles.error}>{recorder.error}</p>}
      {recorder.audioURL && (
        <div className={styles.recordingCard}>
          <span>Local recording playback</span>
          <button type="button" onClick={playRecording}>Play recording</button>
          <audio ref={recordingAudioRef} controls src={recorder.audioURL} />
        </div>
      )}

      <section className={styles.subtitleList} aria-label="Sentence subtitles">
        {session.lesson.sentences.map((item) => {
          const isCurrent = item.index === currentSentenceIndex
          const isCompleted = completed.has(item.index)
          return (
            <button
              key={item.index}
              className={`${styles.subtitleItem} ${isCurrent ? styles.currentSubtitle : ''} ${isCompleted ? styles.completedSubtitle : ''}`}
              onClick={() => seekToSentence(item.index)}
            >
              <span className={styles.subtitleIndex}>{item.index + 1}</span>
              <span className={styles.subtitleText}>
                {item.tokens.map((token) => token.surface).join('')}
              </span>
              {isCompleted && <span className={styles.completedMark}>Done</span>}
            </button>
          )
        })}
      </section>
    </div>
  )
}

function getConfiguredLoopCount(session: ShadowingSessionDTO): number {
  const raw = session.lesson.shadowing_config?.loop_count ?? session.lesson.shadowing_config?.loopCount
  return typeof raw === 'number' && raw > 0 ? Math.floor(raw) : 3
}

function clampScore(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.min(100, Math.max(0, Math.round(value)))
}
