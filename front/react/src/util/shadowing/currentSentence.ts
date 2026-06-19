import type { Sentence } from '@/types/api'

export function getCurrentSentenceIndex(sentences: Sentence[], currentTimeMs: number): number | null {
  if (sentences.length === 0) return null

  let lastValid: number | null = null
  for (const sentence of sentences) {
    if (sentence.end_ms <= sentence.start_ms) continue
    if (currentTimeMs < sentence.start_ms) {
      return lastValid ?? sentence.index
    }
    if (currentTimeMs >= sentence.start_ms && currentTimeMs < sentence.end_ms) {
      return sentence.index
    }
    lastValid = sentence.index
  }

  return lastValid
}
