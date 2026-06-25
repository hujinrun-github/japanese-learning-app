import type { Lesson } from './api'

export type PracticeMode = 'normal' | 'slow' | 'loop' | 'record'

export interface ShadowingProgress {
  user_id: number
  lesson_id: number
  shadowing_version: number
  last_sentence_index: number
  last_position_ms: number
  last_practice_mode: PracticeMode
}

export interface SentenceAttemptSummary {
  sentence_index: number
  attempt_count: number
  best_score: number | null
  last_score: number | null
}

export interface ShadowingSession {
  lesson: Lesson
  progress: ShadowingProgress | null
  completed_sentence_indexes: number[]
  completed_sentence_count: number
  attempt_summary: SentenceAttemptSummary[]
}

export interface SaveProgressRequest {
  shadowing_version: number
  last_sentence_index: number
  last_position_ms: number
  last_practice_mode: PracticeMode
}

export interface SaveAttemptRequest {
  shadowing_version: number
  sentence_index: number
  practice_mode: PracticeMode
  playback_rate: number
  loop_count: number
  self_score?: number | null
}
