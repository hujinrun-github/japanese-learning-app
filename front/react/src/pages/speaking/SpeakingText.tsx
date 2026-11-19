import { Fragment, useEffect, useRef } from 'react'
import { useTranslation } from 'react-i18next'
import type { FuriganaToken } from '@/types/api'
import type { PronunciationResult, PronunciationWord } from '@/types/speaking'
import styles from './SpeakingPage.module.css'

export function splitSpeakingTurns(text: string): string[] {
  // Imported materials already separate numbered items and speaker turns.
  // Keep all sentences in a turn together, including short replies.
  return text.split(/[\r\n]+/).map((line) => line.trim()).filter(Boolean)
}

function WordText({ text, reading }: { text: string; reading?: string }) {
  return reading ? <ruby className={styles.token}>{text}<rt>{reading}</rt></ruby> : <>{text}</>
}

export function SpeakingText({ lines, annotations, offsets, pronunciation, activeSentenceIndex, onPlaySentence, playDisabled }: {
  lines: string[]
  annotations: FuriganaToken[][]
  offsets?: number[]
  pronunciation?: PronunciationResult
  activeSentenceIndex?: number | null
  onPlaySentence?: (index: number) => void
  playDisabled?: boolean
}) {
  const { t } = useTranslation()
  const textRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (activeSentenceIndex === null || activeSentenceIndex === undefined) return
    textRef.current?.querySelector('[aria-current="true"]')?.scrollIntoView({ block: 'nearest', behavior: 'smooth' })
  }, [activeSentenceIndex])
  const words = pronunciation?.status === 'available' ? pronunciation.words : []

  function renderLine(line: string, index: number) {
    const start = offsets?.[index]
    const chars = Array.from(line)
    const lineWords = start === undefined ? [] : words.filter((word) =>
      word.start >= start && word.end <= start + chars.length && word.end > word.start &&
      chars.slice(word.start - start, word.end - start).join('') === word.surface,
    ).sort((a, b) => a.start - b.start)
    if (!lineWords.length) return annotations[index]?.length
      ? annotations[index].map((token, ti) => <WordText key={ti} text={token.surface} reading={token.reading} />)
      : line

    const parts = []
    let cursor = 0
    for (const word of lineWords) {
      const localStart = word.start - start!
      if (localStart < cursor) continue
      parts.push(<Fragment key={`gap-${cursor}`}>{chars.slice(cursor, localStart).join('')}</Fragment>)
      const content = <WordText text={word.surface} reading={word.reading} />
      parts.push(word.status === 'correct' ? <Fragment key={word.start}>{content}</Fragment> : (
        <mark key={word.start} data-pronunciation={word.status}
          className={word.status === 'missing' ? styles.wordMissing : styles.wordMismatch}
          title={wordTitle(word)} tabIndex={0}>
          {content}
          <span className={styles.wordTooltip}>{wordTitle(word)}</span>
        </mark>
      ))
      cursor = word.end - start!
    }
    parts.push(<Fragment key="tail">{chars.slice(cursor).join('')}</Fragment>)
    return parts
  }

  function wordTitle(word: PronunciationWord) {
    const status = t(`speaking.pronunciation.${word.status}`, { defaultValue: word.status === 'missing' ? '疑似漏读' : '疑似读错' })
    const expected = t('speaking.pronunciation.expected', { reading: word.reading, defaultValue: '应读：{{reading}}' })
    const recognized = word.recognized ? t('speaking.pronunciation.recognized', { reading: word.recognized, defaultValue: '识别为：{{reading}}' }) : ''
    return `${status} · ${word.surface} · ${expected}${recognized ? ` · ${recognized}` : ''}`
  }

  return (
    <div ref={textRef} className={styles.textDisplay}>
      {lines.map((line, i) => (
        <div key={i} className={`${styles.sentenceLine}${activeSentenceIndex === i ? ` ${styles.sentenceActive}` : ''}`}
          data-sentence-index={i} aria-current={activeSentenceIndex === i ? 'true' : undefined}>
          <span lang="ja">{renderLine(line, i)}</span>
          {onPlaySentence && <button type="button" className={styles.sentencePlay} disabled={playDisabled}
            aria-label={t('speaking.practice.playSentence', { index: i + 1, defaultValue: '播放第 {{index}} 句' })}
            onClick={() => onPlaySentence(i)}>
            🔊 {t('speaking.practice.sentence', { defaultValue: '本句' })}
          </button>}
        </div>
      ))}
    </div>
  )
}
