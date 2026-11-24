import { DailyPracticeProgress } from '@/components/ui/DailyPracticeProgress'
import { useDailyPracticeProgress } from '@/hooks/useDailyPracticeProgress'
import { useEffect, useRef, useState, type KeyboardEvent } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Badge } from '@/components/ui/Badge'
import { ProgressBar } from '@/components/ui/ProgressBar'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import { speakExample, speakWord } from '@/util/exampleAudio'
import type { Word, WordCard, JLPTLevel } from '@/types/api'
import { evaluateDictationAnswer, normalizeDictationAnswer, wordRequiresKanji } from './dictationAnswer'
import type { DictationAnswerResult } from './dictationAnswer'
import { ErrataButton } from '@/components/ui/ErrataButton'
import { DictionaryButton } from '@/pages/search/ContextualSearch'
import { createMistake } from '@/api/mistakes'
import { buildWordReadingMistake, buildWordDictationMistake } from '@/pages/mistakes/mistakeInput'
import { wordReviewTargetID } from './wordReviewTarget'
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
type ReviewCard = Omit<WordCard, 'record'> & { record?: WordCard['record'] }

export function WordAnswerReading({ word }: { word: Pick<Word, 'reading' | 'reading_type' | 'verb_type'> }) {
  return (
    <div className={styles.readingRow}>
      {word.reading_type && (
        <span
          className={styles.readingTypeBadge}
          title={READING_TYPE_LABELS[word.reading_type] ?? ''}
        >
          {word.reading_type}
        </span>
      )}
      <div className={styles.reading}>{word.reading}</div>
      {word.verb_type && <span className={styles.verbType}>{word.verb_type}</span>}
    </div>
  )
}

export function WordRatingActions({ onRate, disabled }: {
  onRate: (rating: 'normal' | 'easy') => void
  disabled: boolean
}) {
  const { t } = useTranslation()
  return (
    <div className={styles.ratingRow}>
      <button
        className={`${styles.ratingBtn} ${styles.ratingFamiliar}`}
        onClick={() => onRate('normal')}
        disabled={disabled}
        title={t('word.queue.rating.familiarHint')}
      >
        {t('word.queue.rating.familiar')}
      </button>
      <button
        className={`${styles.ratingBtn} ${styles.ratingMastered}`}
        onClick={() => onRate('easy')}
        disabled={disabled}
        title={t('word.queue.rating.masteredHint')}
      >
        {t('word.queue.rating.mastered')}
      </button>
    </div>
  )
}

export function WordReviewPage() {
  const { t } = useTranslation()
  const { progress, refreshProgress } = useDailyPracticeProgress('word')
  const [searchParams, setSearchParams] = useSearchParams()
  const mode: ReviewMode = searchParams.get('mode') === 'dictation' ? 'dictation' : 'recognition'
  const targetWord = searchParams.get('word_id')
  const queueScope = searchParams.get('queue') === 'new' ? 'new' : 'review'
  const requestedLevel = LEVELS.find(item => item === searchParams.get('level')) ?? 'N5'
  const [queue, setQueue] = useState<ReviewCard[]>([])
  const [round, setRound] = useState(0)
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
  const level = targetWord !== null ? queue[0]?.word.jlpt_level ?? requestedLevel : requestedLevel
  const allLevels = targetWord === null && queueScope === 'review' && searchParams.get('level') === 'all'
  const queueLevel = targetWord === null ? allLevels ? 'all' : level : null

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
    const controller = new AbortController()
    void loadQueue(queueLevel, targetWord, controller.signal)
    return () => controller.abort()
  }, [queueLevel, targetWord, queueScope, round])

  useEffect(() => {
    resetCardState()
  }, [mode, currentIndex])

  // Looking at an answer counts as having studied the word. Persist that fact
  // immediately so a learner who never picks a rating does not meet the same
  // card again on the next page load. A failure here must not block the flow.
  function markSeen(wordID: number) {
    apiFetch('POST', `/api/v1/words/${wordID}/seen`).catch(err => {
      console.warn('[word] failed to mark word as seen', wordID, err)
    })
  }

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

  function returnToQueue(lv: JLPTLevel | 'all' = level) {
    if (submitting || submitLock.current) return
    const nextParams = new URLSearchParams(searchParams)
    nextParams.delete('word_id')
    nextParams.set('level', lv)
    setSearchParams(nextParams)
  }

  function handleQueueChange(nextScope: 'review' | 'new') {
    if (submitting || submitLock.current) return
    const nextParams = new URLSearchParams(searchParams)
    nextParams.delete('word_id')
    nextParams.set('queue', nextScope)
    if (nextScope === 'new' && allLevels) nextParams.set('level', level)
    setSearchParams(nextParams)
  }

  async function loadQueue(lv: JLPTLevel | 'all' | null, target: string | null, signal: AbortSignal) {
    setLoading(true)
    setError('')
    setQueue([])
    setCurrentIndex(0)
    resetCardState()
    try {
      if (target !== null) {
        const id = wordReviewTargetID(target)
        if (id === null) throw new Error(t('word.queue.invalidTarget'))
        const word = await apiFetch<Word>('GET', `/api/v1/words/${id}`, undefined, signal)
        if (signal.aborted) return
        if (!word || word.id !== id) throw new Error(t('word.queue.invalidTarget'))
        setQueue([{ word, is_new: false }])
      } else {
        const cards = await apiFetch<WordCard[]>('GET', `/api/v1/words/queue?level=${lv}&queue=${queueScope}`, undefined, signal)
        if (!signal.aborted) setQueue(cards ?? [])
      }
    } catch (err) {
      if (signal.aborted) return
      setError(err instanceof Error ? err.message : 'Failed to load')
    } finally {
      if (!signal.aborted) setLoading(false)
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
    markSeen(card.word.id)
    if (correct) {
      setFlipped(true)
      setInputWrong(false)
      submitLock.current = false
      return
    }

    createMistake(buildWordReadingMistake(card.word, readingInput)).catch(err => {
      console.error('[word] failed to record reading mistake', card.word.id, err)
    })

    setSubmitting(true)
    try {
      await apiFetch('POST', `/api/v1/words/${card.word.id}/rate`, { rating: 'hard' })
      refreshProgress()
    } catch (err) {
      console.warn('[word] failed to save hard rating', card.word.id, err)
      setError(err instanceof Error ? err.message : 'Failed to save rating')
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
    markSeen(card.word.id)
    setDictationResult(result)
    setFlipped(true)

    if (result.allCorrect) {
      submitLock.current = false
      return
    }

    createMistake(buildWordDictationMistake(card.word, {
      spelling: spellingInput,
      reading: dictationReadingInput,
      requiresKanji: result.requiresKanji,
    })).catch(err => {
      console.error('[word] failed to record dictation mistake', card.word.id, err)
    })

    setSubmitting(true)
    try {
      await apiFetch('POST', `/api/v1/words/${card.word.id}/rate`, { rating: 'hard' })
      refreshProgress()
    } catch (err) {
      console.warn('[word] failed to save hard dictation rating', card.word.id, err)
      setError(err instanceof Error ? err.message : 'Failed to save rating')
    } finally {
      setSubmitting(false)
      submitLock.current = false
    }
  }

  function handleNextAfterWrong() {
    if (submitting || submitLock.current) return
    setError('')
    resetCardState()
    setCurrentIndex(index => index + 1)
  }

  async function handleRate(rating: 'normal' | 'easy') {
    if (submitting || submitLock.current || !card) return
    submitLock.current = true
    setSubmitting(true)
    try {
      await apiFetch('POST', `/api/v1/words/${card.word.id}/rate`, { rating })
      refreshProgress()
      // Only advance after the review is persisted; otherwise the word would
      // look studied while still waiting in the queue.
      setError('')
      resetCardState()
      setCurrentIndex(index => index + 1)
    } catch (err) {
      console.warn('[word] failed to save rating', card.word.id, rating, err)
      setError(err instanceof Error ? err.message : 'Failed to save rating')
    } finally {
      setSubmitting(false)
      submitLock.current = false
    }
  }

  function handleAnswerKeyDown(event: KeyboardEvent<HTMLInputElement>, submit: () => void) {
    if (event.key === 'Enter' && !event.nativeEvent.isComposing) submit()
  }

  const dictationReady = Boolean(
    normalizeDictationAnswer(spellingInput)
    && (!requiresKanji || normalizeDictationAnswer(dictationReadingInput)),
  )

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.title}>{t(queueScope === 'new' && targetWord === null ? 'word.queue.learnTitle' : 'word.queue.title')}</h1>
        {targetWord !== null && (
          <button className={styles.returnToQueue} onClick={() => returnToQueue()} disabled={submitting}>
            {t('word.queue.backToQueue')}
          </button>
        )}
      </div>
      <DailyPracticeProgress progress={progress} />

      <div className={styles.modeSwitch} role="group" aria-label={t('word.queue.scope.label')}>
        {(['review', 'new'] as const).map(scope => (
          <button
            key={scope}
            className={`${styles.modeButton} ${queueScope === scope ? styles.modeButtonActive : ''}`}
            onClick={() => handleQueueChange(scope)}
            disabled={submitting}
            aria-pressed={queueScope === scope}
          >
            {t(`word.queue.scope.${scope}`)}
          </button>
        ))}
      </div>
      <p className={styles.progress}>{t(queueScope === 'new' ? 'word.queue.newHint' : 'word.queue.reviewHint')}</p>

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
        {queueScope === 'review' && (
          <button
            className={`${styles.tab} ${allLevels ? styles.tabActive : ''}`}
            onClick={() => returnToQueue('all')}
            disabled={submitting}
            aria-pressed={allLevels}
          >
            {t('word.queue.allLevels')}
          </button>
        )}
        {LEVELS.map(lv => (
          <button
            key={lv}
            data-level={lv}
            className={`${styles.tab} ${!allLevels && level === lv ? styles.tabActive : ''}`}
            onClick={() => returnToQueue(lv)}
            disabled={submitting}
            aria-pressed={!allLevels && level === lv}
          >
            {lv}
          </button>
        ))}
      </div>

      {error && <p className={styles.error} role="alert">{error}</p>}

      {loading ? (
        <div style={{ display: 'flex', justifyContent: 'center', paddingTop: '40px' }}><Spinner size="lg" /></div>
      ) : done ? (
        !error && <EmptyState
          icon="🎉"
          title={t(targetWord !== null ? 'word.queue.targetDone' : queue.length > 0 ? 'word.queue.roundDone' : queueScope === 'new' ? 'word.queue.noNew' : 'word.queue.noDue')}
          description={t(targetWord !== null ? 'word.queue.targetDoneDesc' : queue.length > 0 ? 'word.queue.roundDoneDesc' : queueScope === 'new' ? 'word.queue.noNewDesc' : 'word.queue.noDueDesc')}
          action={targetWord === null && queue.length > 0 && (
            <button className={styles.returnToQueue} onClick={() => setRound(value => value + 1)}>
              {t(queueScope === 'new' ? 'word.queue.continueNew' : 'word.queue.continueReview')}
            </button>
          )}
        />
      ) : (
        <>
          <p className={styles.progress}>
            {t('word.queue.progress', { current: currentIndex + 1, total: queue.length })}
          </p>
          {card && (
            <ErrataButton
              key={`${card.word.id}:${mode}`}
              module="word"
              itemKey={`${card.word.id}:${mode}`}
              label={mode === 'dictation' && !flipped
                ? t('errata.dictationItem', { id: card.word.id })
                : `${card.word.kanji_form}・${card.word.reading}`}
            />
          )}

          <div className={`${styles.cardScene} ${mode === 'dictation' ? styles.dictationCardScene : ''}`}>
            <div className={`${styles.cardInner} ${flipped ? styles.cardFlipped : ''}`}>
              {!flipped && <div className={styles.cardFace}>
                <div className={styles.badges}>
                  <Badge level={card!.word.jlpt_level} size="sm" />
                  {card!.is_new && <span className={styles.newBadge}>{t('word.queue.isNew')}</span>}
                  {card!.word.part_of_speech && (
                    <span className={styles.partOfSpeech}>{card!.word.part_of_speech}</span>
                  )}
                  {card!.word.verb_type && (
                    <span className={styles.verbType}>{card!.word.verb_type}</span>
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
                <WordAnswerReading word={card!.word} />
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
                              lang="ja"
                              data-dictionary-text
                              dangerouslySetInnerHTML={{ __html: example.furigana_html }}
                            />
                          ) : (
                            <span className={styles.exampleJa} lang="ja" data-dictionary-text>{example.japanese}</span>
                          )}
                          <DictionaryButton text={example.japanese} compact />
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
                {card!.record && <div className={styles.masteryBar}>
                  <ProgressBar
                    value={(card!.record?.mastery_level ?? 0) * 20}
                    label={t('word.queue.masteryLevel', { level: card!.record?.mastery_level ?? 0 })}
                  />
                </div>}
              </div>}
            </div>
          </div>

          {flipped && (mode === 'recognition' || dictationResult?.allCorrect) && (
            <WordRatingActions onRate={handleRate} disabled={submitting} />
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
