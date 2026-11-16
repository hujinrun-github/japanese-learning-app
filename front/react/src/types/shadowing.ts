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

// Local shadowing evaluation result (POST /api/v1/lessons/{id}/shadowing/evaluations).
export type EvaluationMode = 'shadow' | 'repeat'
export type EvaluationStatus = 'scoreable' | 'unscorable'

export interface DimensionScore {
  score: number
  confidence: number
  weight: number
  available: boolean
}

export interface EvaluationIssue {
  type: string
  confidence: number
  /** Scoring dimension this suggestion belongs to (sync / prosody / ...). */
  dimension?: string
  /** Measured numbers behind the suggestion, e.g. "起步延迟 1500 ms". */
  evidence_zh?: string
  message_zh: string
  action_zh?: string
}

export interface EvaluationResult {
  status: EvaluationStatus
  unscorable_code?: string
  mode: EvaluationMode
  overall_score?: number
  partial: boolean
  scores: Record<string, DimensionScore>
  recognized_text: string
  lag_ms: number
  issues: EvaluationIssue[]
  score_source: string
  scorer_version: string
  weights_version: string
  model_version: string
  shadowing_version: number
  latency_ms: number
}
