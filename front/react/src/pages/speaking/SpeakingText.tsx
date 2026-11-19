import type { FuriganaToken } from '@/types/api'
import styles from './SpeakingPage.module.css'

export function splitSpeakingTurns(text: string): string[] {
  // Imported materials already separate numbered items and speaker turns.
  // Keep all sentences in a turn together, including short replies.
  return text.split(/[\r\n]+/).map((line) => line.trim()).filter(Boolean)
}

export function SpeakingText({ lines, annotations }: {
  lines: string[]
  annotations: FuriganaToken[][]
}) {
  return (
    <div className={styles.textDisplay}>
      {lines.map((line, i) => (
        <div key={i} className={styles.sentenceLine}>
          {annotations[i]?.length ? (
            annotations[i].map((token, ti) => (
              <ruby key={ti} className={styles.token}>
                {token.surface}
                <rt>{token.reading}</rt>
              </ruby>
            ))
          ) : line}
        </div>
      ))}
    </div>
  )
}
