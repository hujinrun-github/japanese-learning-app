import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Badge } from '@/components/ui/Badge'
import { ProgressBar } from '@/components/ui/ProgressBar'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { speakExample, speakWord } from '@/util/exampleAudio'
import type { WordCard, JLPTLevel } from '@/types/api'
import { evaluateDictationAnswer, normalizeDictationAnswer, wordRequiresKanji } from './dictationAnswer'
import type { DictationAnswerResult } from './dictationAnswer'
import styles from './WordReviewPage.module.css'

const LEVELS: JLPTLevel[] = ['N5', 'N4', 'N3', 'N2', 'N1']

const READING_TYPE_LABELS: Record<string, string> = {
  '1': '音読み',
  '2': '訓読み',
  '3': '熟字訓',
  '4': '重箱読み',
  '5': '湯桶読み',
  '6': 'その他',
}

type ReviewMode = 'recognition' | 'dictation'

export function WordReviewPage() {
  const { t } = useTranslation()
  const [searchParams, setSearchParams] = useSearchParams()
  const mode: ReviewMode = searchParams.get('mode') === 'dictation' ? 'dictation' : 'recognition'
  const [level, setLevel] = useState<JLPTLevel>('N5')
  const [queue, setQueue] = useState<WordCard[]>([])
  const [currentIndex, setCurrentIndex] = useState(0)
  const [flipped, setFlipped] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [readingInput, setReadingInput] = useState('')
  const [inputWrong, setInputWrong] = useState(false)
  const [spellingInput, setSpellingInput] = useState('')
  const [dictationReadingInput, setDictationReadingInput] = useState('')
  const [dictationResult, setDictationResult] = useState<DictationAnswerResult | null>(null)
  const submitLock = useRef(false)

  const done = queue.length === 0 || currentIndex >= queue.length
  const card = done ? null : queue[currentIndex]
  const requiresKanji = card ? wordRequiresKanji(card.word.kanji_form) : false

  // Recognition mode keeps the existing example playback. Dictation stays quiet
  // after revealing the answer so the result feedback is not interrupted.
  useEffect(() => {
    if (mode !== 'recognition' || !flipped || !card) return
    const examples = card.word.examples ?? []
    if (examples.length === 0) return
    const timers: number[] = []
    timers.push(window.setTimeout(() => {
      examples.forEach((example, index) => {
        timers.push(window.setTimeout(() => speakExample(example.japanese), index * 3500))
      })
    }, 600))
    return () => timers.forEach(timer => window.clearTimeout(timer))
  }, [mode, flipped, currentIndex, card])

  useEffect(() => {
    loadQueue(level)
  }, [level])

  useEffect(() => {
    resetCardState()
  }, [mode, currentIndex])

  function resetCardState() {
    setFlipped(false)
    setReadingInput('')
    setInputWrong(false)
    setSpellingInput('')
    setDictationReadingInput('')
    setDictationResult(null)
  }

  function handleModeChange(nextMode: ReviewMode) {
    if (submitting || submitLock.current) return
    const nextParams = new URLSearchParams(searchParams)
    if (nextMode === 'dictation') {
      nextParams.set('mode', 'dictation')
    } else {
      nextParams.delete('mode')
    }
    setSearchParams(nextParams, { replace: true })
  }

  async function loadQueue(lv: JLPTLevel) {
    setLoading(true)
    setError('')
    setCurrentIndex(0)
    resetCardState()
    try {
      const cards = await apiFetch<WordCard[]>('GET', `/api/v1/words/queue?level=${lv}`)
      setQueue(cards ?? [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }

  async function handleCheckReading() {
    if (
      flipped
      || !card
      || inputWrong
      || submitting
      || submitLock.current
      || !normalizeDictationAnswer(readingInput)
    ) return

    submitLock.current = true
    const correct = normalizeDictationAnswer(readingInput) === normalizeDictationAnswer(card.word.reading)
    if (correct) {
      setFlipped(true)
      setInputWrong(false)
      submitLock.current = false
      return
    }

    setSubmitting(true)
    try {
      await apiFetch('POST', `/api/v1/words/${card.word.id}/rate`, { rating: 'hard' })
    } catch {
      // Keep the learning flow available if saving the rating fails.
    } finally {
      setInputWrong(true)
      setSubmitting(false)
      submitLock.current = false
    }
  }

  async function handleCheckDictation() {
    if (!card || dictationResult || submitting || submitLock.current || !dictationReady) return

    submitLock.current = true
    const result = evaluateDictationAnswer(card.word, spellingInput, dictationReadingInput)
    setDictationResult(result)
    setFlipped(true)

    if (result.allCorrect) {
      submitLock.current = false
      return
    }

    setSubmitting(true)
    try {
      await apiFetch('POST', `/api/v1/words/${card.word.id}/rate`, { rating: 'hard' })
    } catch {
      // Keep the answer visible and allow the user to continue.
    } finally {
      setSubmitting(false)
      submitLock.current = false
    }
  }

  function handleNextAfterWrong() {
    if (submitting || submitLock.current) return
    resetCardState()
    setCurrentIndex(index => index + 1)
  }

  async function handleRate(rating: 'easy' | 'normal' | 'hard') {
    if (submitting || submitLock.current || !card) return
    submitLock.current = true
    setSubmitting(true)
    try {
      await apiFetch('POST', `/api/v1/words/${card.word.id}/rate`, { rating })
    } catch {
      // Best effort: a temporary save failure should not trap the user on a card.
    } finally {
      setSubmitting(false)
      submitLock.current = false
      resetCardState()
      setCurrentIndex(index => index + 1)
    }
  }

  function handleAnswerKeyDown(event: KeyboardEvent<HTMLInputElement>, submit: () => void) {
    if (event.key === 'Enter' && !event.nativeEvent.isComposing) submit()
  }

  const dictationReady = Boolean(
    normalizeDictationAnswer(spellingInput)
    && (!requiresKanji || normalizeDictationAnswer(dictationReadingInput)),
  )

  if (loading) {
    return (
      <div className={styles.page} style={{ display: 'flex', justifyContent: 'center', paddingTop: '80px' }}>
        <Spinner size="lg" />
      </div>
    )
  }

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.title}>{t('word.queue.title')}</h1>
      </div>

      <div className={styles.modeSwitch} role="group" aria-label={t('word.queue.mode.label')}>
        <button
          className={`${styles.modeButton} ${mode === 'recognition' ? styles.modeButtonActive : ''}`}
          onClick={() => handleModeChange('recognition')}
          disabled={submitting}
          aria-pressed={mode === 'recognition'}
        >
          📖 {t('word.queue.mode.recognition')}
        </button>
        <button
          className={`${styles.modeButton} ${mode === 'dictation' ? styles.modeButtonActive : ''}`}
          onClick={() => handleModeChange('dictation')}
          disabled={submitting}
          aria-pressed={mode === 'dictation'}
        >
          🎧 {t('word.queue.mode.dictation')}
        </button>
      </div>

      <div className={styles.tabs}>
        {LEVELS.map(lv => (
          <button
            key={lv}
            className={`${styles.tab} ${level === lv ? styles.tabActive : ''}`}
            onClick={() => setLevel(lv)}
            disabled={submitting}
          >
            {lv}
          </button>
        ))}
      </div>

      {error && <p className={styles.error}>{error}</p>}

      {done ? (
        <EmptyState icon="🎉" title={t('word.queue.done')} description={t('word.queue.doneDesc')} />
      ) : (
        <>
          <p className={styles.progress}>
            {t('word.queue.progress', { current: currentIndex + 1, total: queue.length })}
          </p>

          <div className={`${styles.cardScene} ${mode === 'dictation' ? styles.dictationCardScene : ''}`}>
            <div className={`${styles.cardInner} ${flipped ? styles.cardFlipped : ''}`}>
              {!flipped && <div className={styles.cardFace}>
                <div className={styles.badges}>
                  <Badge level={card!.word.jlpt_level} size="sm" />
                  {card!.is_new && <span className={styles.newBadge}>{t('word.queue.isNew')}</span>}
                  {card!.word.part_of_speech && (
                    <span className={styles.partOfSpeech}>{card!.word.part_of_speech}</span>
                  )}
                </div>

                {mode === 'recognition' ? (
                  <>
                    <div className={styles.kanjiRow}>
                      {card!.word.reading_type && (
                        <span
                          className={styles.readingTypeBadge}
                          title={READING_TYPE_LABELS[card!.word.reading_type] ?? ''}
                        >
                          {card!.word.reading_type}
                        </span>
                      )}
                      <div className={styles.kanji}>{card!.word.kanji_form}</div>
                      <button
                        className={styles.speakBtn}
                        aria-label={t('word.queue.speak')}
                        onClick={event => {
                          event.stopPropagation()
                          speakWord(card!.word.audio_url, card!.word.reading)
                        }}
                      >
                        🔊
                      </button>
                    </div>

                    <div className={styles.inputRow}>
                      <input
                        className={`${styles.readingInput} ${inputWrong ? styles.readingInputWrong : ''}`}
                        type="text"
                        value={readingInput}
                        onChange={event => {
                          setReadingInput(event.target.value)
                        }}
                        onKeyDown={event => handleAnswerKeyDown(event, handleCheckReading)}
                        placeholder={t('word.queue.inputPlaceholder')}
                        autoComplete="off"
                        disabled={submitting || inputWrong}
                        autoFocus
                      />
                      <button
                        className={styles.checkBtn}
                        onClick={handleCheckReading}
                        disabled={submitting || !normalizeDictationAnswer(readingInput)}
                      >
                        {t('word.queue.check')}
                      </button>
                    </div>
                    {inputWrong && (
                      <div className={styles.wrongArea}>
                        <p className={styles.wrongHint}>
                          {t('word.queue.wrongHint', { reading: card!.word.reading })}
                        </p>
                        <button className={styles.nextBtn} onClick={handleNextAfterWrong}>
                          {t('word.queue.nextWord')}
                        </button>
                      </div>
                    )}
                  </>
                ) : (
                  <>
                    <p className={styles.dictationPrompt}>{t('word.queue.dictation.prompt')}</p>
                    <button
                      className={styles.dictationPlayButton}
                      onClick={() => speakWord(card!.word.audio_url, card!.word.reading)}
                      aria-label={t('word.queue.dictation.play')}
                    >
                      <span aria-hidden="true">🔊</span>
                      {t('word.queue.dictation.play')}
                    </button>
                    <div className={styles.meaningHint}>
                      <span>{t('word.queue.dictation.meaningHint')}</span>
                      <strong>{card!.word.meaning}</strong>
                    </div>
                    <div className={styles.dictationInputs}>
                      <label className={styles.dictationField}>
                        <span>
                          {requiresKanji
                            ? t('word.queue.dictation.spellingLabel')
                            : t('word.queue.dictation.kanaSpellingLabel')}
                        </span>
                        <input
                          className={styles.readingInput}
                          value={spellingInput}
                          onChange={event => setSpellingInput(event.target.value)}
                          onKeyDown={event => handleAnswerKeyDown(event, handleCheckDictation)}
                          placeholder={t('word.queue.dictation.spellingPlaceholder')}
                          autoComplete="off"
                          disabled={submitting}
                          autoFocus
                        />
                      </label>
                      {requiresKanji && (
                        <label className={styles.dictationField}>
                          <span>{t('word.queue.dictation.readingLabel')}</span>
                          <input
                            className={styles.readingInput}
                            value={dictationReadingInput}
                            onChange={event => setDictationReadingInput(event.target.value)}
                            onKeyDown={event => handleAnswerKeyDown(event, handleCheckDictation)}
                            placeholder={t('word.queue.dictation.readingPlaceholder')}
                            autoComplete="off"
                            disabled={submitting}
                          />
                        </label>
                      )}
                    </div>
                    <button
                      className={`${styles.checkBtn} ${styles.dictationCheckButton}`}
                      onClick={handleCheckDictation}
                      disabled={submitting || !dictationReady}
                    >
                      {t('word.queue.check')}
                    </button>
                  </>
                )}
              </div>}

              {flipped && <div className={`${styles.cardFace} ${styles.cardBack}`}>
                {mode === 'dictation' && dictationResult && (
                  <div className={styles.dictationFeedback} aria-live="polite">
                    <h2>
                      {dictationResult.allCorrect
                        ? t('word.queue.dictation.allCorrect')
                        : t('word.queue.dictation.needsReview')}
                    </h2>
                    <div className={dictationResult.spellingCorrect ? styles.resultCorrect : styles.resultWrong}>
                      <span>{dictationResult.spellingCorrect ? '✓' : '✗'}</span>
                      <div>
                        <strong>
                          {dictationResult.requiresKanji
                            ? t('word.queue.dictation.spellingLabel')
                            : t('word.queue.dictation.kanaSpellingLabel')}
                        </strong>
                        <small>{t('word.queue.dictation.yourAnswer')}: {spellingInput}</small>
                        <small>{t('word.queue.dictation.correctAnswer')}: {card!.word.kanji_form}</small>
                      </div>
                    </div>
                    {dictationResult.requiresKanji && (
                      <div className={dictationResult.readingCorrect ? styles.resultCorrect : styles.resultWrong}>
                        <span>{dictationResult.readingCorrect ? '✓' : '✗'}</span>
                        <div>
                          <strong>{t('word.queue.dictation.readingLabel')}</strong>
                          <small>{t('word.queue.dictation.yourAnswer')}: {dictationReadingInput}</small>
                          <small>{t('word.queue.dictation.correctAnswer')}: {card!.word.reading}</small>
                        </div>
                      </div>
                    )}
                  </div>
                )}

                <div className={styles.answerHeading}>
                  <div className={styles.answerSpelling}>{card!.word.kanji_form}</div>
                  <button
                    className={styles.speakBtn}
                    aria-label={t('word.queue.speak')}
                    onClick={event => {
                      event.stopPropagation()
                      speakWord(card!.word.audio_url, card!.word.reading)
                    }}
                  >
                    🔊
                  </button>
                </div>
                <div className={styles.readingRow}>
                  {card!.word.reading_type && (
                    <span
                      className={styles.readingTypeBadge}
                      title={READING_TYPE_LABELS[card!.word.reading_type] ?? ''}
                    >
                      {card!.word.reading_type}
                    </span>
                  )}
                  <div className={styles.reading}>{card!.word.reading}</div>
                </div>
                <div className={styles.meaning}>{card!.word.meaning}</div>
                {card!.word.examples && card!.word.examples.length > 0 && (
                  <>
                    <div className={styles.examplesTitle}>{t('word.queue.examples')}</div>
                    {card!.word.examples.slice(0, 2).map((example, index) => (
                      <div key={index} className={styles.example}>
                        <div className={styles.exampleJaRow}>
                          {example.furigana_html ? (
                            <span
                              className={styles.exampleJa}
                              dangerouslySetInnerHTML={{ __html: example.furigana_html }}
                            />
                          ) : (
                            <span className={styles.exampleJa}>{example.japanese}</span>
                          )}
                          <button
                            className={`${styles.speakBtn} ${styles.speakBtnSm}`}
                            aria-label={t('word.queue.speak')}
                            onClick={event => {
                              event.stopPropagation()
                              speakExample(example.japanese)
                            }}
                          >
                            🔊
                          </button>
                        </div>
                        <div className={styles.exampleZh}>{example.chinese}</div>
                      </div>
                    ))}
                  </>
                )}
                <div className={styles.masteryBar}>
                  <ProgressBar
                    value={(card!.record?.mastery_level ?? 0) * 20}
                    label={t('word.queue.masteryLevel', { level: card!.record?.mastery_level ?? 0 })}
                  />
                </div>
              </div>}
            </div>
          </div>

          {flipped && (mode === 'recognition' || dictationResult?.allCorrect) && (
            <div className={styles.ratingRow}>
              <button
                className={`${styles.ratingBtn} ${styles.ratingHard}`}
                onClick={() => handleRate('hard')}
                disabled={submitting}
              >
                {t('word.queue.rating.hard')}
              </button>
              <button
                className={`${styles.ratingBtn} ${styles.ratingNormal}`}
                onClick={() => handleRate('normal')}
                disabled={submitting}
              >
                {t('word.queue.rating.normal')}
              </button>
              <button
                className={`${styles.ratingBtn} ${styles.ratingEasy}`}
                onClick={() => handleRate('easy')}
                disabled={submitting}
              >
                {t('word.queue.rating.easy')}
              </button>
            </div>
          )}

          {flipped && mode === 'dictation' && dictationResult && !dictationResult.allCorrect && (
            <div className={styles.nextRow}>
              <button className={styles.nextBtn} onClick={handleNextAfterWrong} disabled={submitting}>
                {submitting ? t('word.queue.dictation.saving') : t('word.queue.nextWord')}
              </button>
            </div>
          )}
        </>
      )}
    </div>
  )
}
