import { useState, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Badge } from '@/components/ui/Badge'
import { ProgressBar } from '@/components/ui/ProgressBar'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import type { WordCard, JLPTLevel } from '@/types/api'
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

export function WordReviewPage() {
  const { t } = useTranslation()
  const [level, setLevel] = useState<JLPTLevel>('N5')
  const [queue, setQueue] = useState<WordCard[]>([])
  const [currentIndex, setCurrentIndex] = useState(0)
  const [flipped, setFlipped] = useState(false)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const voiceRef = useRef<SpeechSynthesisVoice | null>(null)

  // Cache preferred Japanese voice once on mount
  useEffect(() => {
    const loadVoice = () => {
      const voices = speechSynthesis.getVoices()
      const jaVoices = voices.filter((v) => v.lang.startsWith('ja'))
      if (jaVoices.length > 0) {
        voiceRef.current = jaVoices.find((v) => v.name.includes('Google'))
          ?? jaVoices.find((v) => v.name.includes('Kyoko'))
          ?? jaVoices[0]
      }
    }
    loadVoice()
    speechSynthesis.onvoiceschanged = loadVoice
  }, [])

  // Pre-warm Google voice: speak a phrase at volume 0 to trigger voice download
  useEffect(() => {
    if (queue.length === 0) return
    const timer = setTimeout(() => {
      const u = new SpeechSynthesisUtterance('こんにちは')
      u.lang = 'ja-JP'
      u.volume = 0
      if (voiceRef.current) u.voice = voiceRef.current
      speechSynthesis.speak(u)
    }, 500)
    return () => clearTimeout(timer)
  }, [queue])

  useEffect(() => {
    loadQueue(level)
  }, [level])

  async function loadQueue(lv: JLPTLevel) {
    setLoading(true)
    setError('')
    setCurrentIndex(0)
    setFlipped(false)
    try {
      const cards = await apiFetch<WordCard[]>('GET', `/api/v1/words/queue?level=${lv}`)
      setQueue(cards ?? [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Failed to load')
    } finally {
      setLoading(false)
    }
  }

  function handleSpeak(text: string) {
    if (speechSynthesis.speaking) {
      speechSynthesis.cancel()
    }
    const u = new SpeechSynthesisUtterance(text)
    u.lang = 'ja-JP'
    u.rate = 0.95
    u.pitch = 1.1
    if (voiceRef.current) {
      u.voice = voiceRef.current
    }
    speechSynthesis.speak(u)
  }

  async function handleRate(r: 'easy' | 'normal' | 'hard') {
    if (submitting) return
    const card = queue[currentIndex]
    setSubmitting(true)
    try {
      await apiFetch('POST', `/api/v1/words/${card.word.id}/rate`, { rating: r })
    } catch {
      // best-effort — don't block user flow on rating failure
    } finally {
      setSubmitting(false)
      setFlipped(false)
      setCurrentIndex((i) => i + 1)
    }
  }

  if (loading) {
    return (
      <div className={styles.page} style={{ display: 'flex', justifyContent: 'center', paddingTop: '80px' }}>
        <Spinner size="lg" />
      </div>
    )
  }

  const done = queue.length === 0 || currentIndex >= queue.length
  const card = done ? null : queue[currentIndex]

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.title}>{t('word.queue.title')}</h1>
      </div>

      <div className={styles.tabs}>
        {LEVELS.map((lv) => (
          <button
            key={lv}
            className={`${styles.tab} ${level === lv ? styles.tabActive : ''}`}
            onClick={() => setLevel(lv)}
          >
            {lv}
          </button>
        ))}
      </div>

      {error && <p style={{ color: 'var(--color-error)', marginBottom: 'var(--space-4)' }}>{error}</p>}

      {done ? (
        <EmptyState icon="🎉" title={t('word.queue.done')} description={t('word.queue.doneDesc')} />
      ) : (
        <>
          <p className={styles.progress}>
            {t('word.queue.progress', { current: currentIndex + 1, total: queue.length })}
          </p>

          {/* Flashcard */}
          <div className={styles.cardScene} onClick={() => !flipped && setFlipped(true)}>
            <div className={`${styles.cardInner} ${flipped ? styles.cardFlipped : ''}`}>
              {/* Front face */}
              <div className={styles.cardFace}>
                <div className={styles.badges}>
                  <Badge level={card!.word.jlpt_level} size="sm" />
                  {card!.is_new && (
                    <span style={{
                      fontSize: 'var(--font-size-xs)',
                      background: 'var(--color-primary)',
                      color: '#fff',
                      padding: '2px 8px',
                      borderRadius: 'var(--radius-full)',
                      fontWeight: 600,
                    }}>
                      {t('word.queue.isNew')}
                    </span>
                  )}
                  {card!.word.part_of_speech && (
                    <span style={{ fontSize: 'var(--font-size-xs)', color: 'var(--color-text-secondary)' }}>
                      {card!.word.part_of_speech}
                    </span>
                  )}
                </div>
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
                    onClick={(e) => { e.stopPropagation(); handleSpeak(card!.word.reading) }}
                  >
                    🔊
                  </button>
                </div>
                <p className={styles.flipHint}>{t('word.queue.flip')}</p>
              </div>

              {/* Back face */}
              <div className={`${styles.cardFace} ${styles.cardBack}`}>
                <div className={styles.readingRow}>
                  <div className={styles.reading}>{card!.word.reading}</div>
                  <button
                    className={styles.speakBtn}
                    aria-label={t('word.queue.speak')}
                    onClick={(e) => { e.stopPropagation(); handleSpeak(card!.word.reading) }}
                  >
                    🔊
                  </button>
                </div>
                <div className={styles.meaning}>{card!.word.meaning}</div>
                {card!.word.examples && card!.word.examples.length > 0 && (
                  <>
                    <div className={styles.examplesTitle}>例文</div>
                    {card!.word.examples.slice(0, 2).map((ex, i) => (
                      <div key={i} className={styles.example}>
                        <div className={styles.exampleJaRow}>
                          {ex.furigana_html ? (
                            <span
                              className={styles.exampleJa}
                              dangerouslySetInnerHTML={{ __html: ex.furigana_html }}
                            />
                          ) : (
                            <span className={styles.exampleJa}>{ex.japanese}</span>
                          )}
                          <button
                            className={`${styles.speakBtn} ${styles.speakBtnSm}`}
                            aria-label={t('word.queue.speak')}
                            onClick={(e) => { e.stopPropagation(); handleSpeak(ex.japanese) }}
                          >
                            🔊
                          </button>
                        </div>
                        <div className={styles.exampleZh}>{ex.chinese}</div>
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
              </div>
            </div>
          </div>

          {/* Rating buttons — only shown after flip */}
          {flipped && (
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
        </>
      )}
    </div>
  )
}
