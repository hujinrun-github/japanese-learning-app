import type { FuriganaToken } from '@/types/api'
import styles from './SpeakingPage.module.css'

export function splitSpeakingSentences(text: string): string[] {
  // Honor explicit dialogue turns, then split sentence endings independently
  // of the next character (which may be a number, space, or Latin word).
  return text.split(/[\r\n]+/).flatMap((line) =>
    (line.match(/.+?(?:[。！？!?]+[」』）)\]”’"]*|$)/gu) ?? [])
      .map((sentence) => sentence.trim())
      .filter(Boolean),
  )
}

export function SpeakingText({ sentences, annotations }: {
  sentences: string[]
  annotations: FuriganaToken[][]
}) {
  return (
    <div className={styles.textDisplay}>
      {sentences.map((sentence, i) => (
        <div key={i} className={styles.sentenceLine}>
          {annotations[i]?.length ? (
            annotations[i].map((token, ti) => (
              <ruby key={ti} className={styles.token}>
                {token.surface}
                <rt>{token.reading}</rt>
              </ruby>
            ))
          ) : sentence}
        </div>
      ))}
    </div>
  )
}
