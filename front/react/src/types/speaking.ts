import type { EvaluationResult } from './shadowing'

export interface PronunciationWord {
  /** Unicode codepoint offsets in the original material text; end is exclusive. */
  start: number
  end: number
  surface: string
  reading: string
  recognized?: string
  status: 'correct' | 'mismatch' | 'missing'
}

export interface PronunciationResult {
  status: 'available' | 'unavailable' | 'error'
  reason?: 'insufficient_audio'
  message?: string
  words: PronunciationWord[]
}

export interface SpeakingEvaluationResult extends EvaluationResult {
  pronunciation?: PronunciationResult
}
