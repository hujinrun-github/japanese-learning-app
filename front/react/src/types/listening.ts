import type { JLPTLevel, Lesson, PracticeKnowledgeLink, Sentence } from './api'

export type ListeningStage = 'blind' | 'intensive' | 'retest' | 'completed'
export type ListeningPhase = 'blind' | 'retest'
export type ListeningStatus = 'all' | 'new' | 'practiced' | 'due'
export type ListeningAudioSource = 'synthetic' | 'human' | 'unknown'
export interface ListeningAnswer { question_id: string; option_index: number }
export interface ListeningDraft { phase: ListeningPhase; answers: ListeningAnswer[]; assisted?: boolean }
export interface ListeningProgress {
  stage: ListeningStage
  sentence_index: number
  position_ms: number
  bookmarks: number[]
  due_at: string | null
  review_step: number
  completed_count: number
  cycle: number
  draft_answers?: ListeningDraft | null
}
export interface ListeningQuestion { id: string; type: string; prompt: string; options: string[] }
export interface ListeningAnswerResult extends ListeningAnswer {
  correct_option_index: number
  correct: boolean
  explanation: string
  evidence_sentence_index: number
}
export interface ListeningAttempt {
  id: number
  phase: ListeningPhase
  correct_count: number
  total: number
  assisted: boolean
  results: ListeningAnswerResult[]
  created_at: string
}
export interface ListeningMaterial {
  id: number
  title: string
  jlpt_level: JLPTLevel
  audio_url: string
  video_url: string
  duration_ms: number
  sentence_count: number
  question_count: number
  status: Exclude<ListeningStatus, 'all'>
  progress: ListeningProgress | null
  training_ready?: boolean
  audio_source?: ListeningAudioSource
}
export interface ListeningCatalog {
  materials: ListeningMaterial[]
  stats: { total: number; new: number; practiced: number; due: number }
}
export interface ListeningSession {
  lesson: Lesson
  questions: ListeningQuestion[]
  progress: ListeningProgress
  attempts: ListeningAttempt[]
  training_ready?: boolean
  audio_source?: ListeningAudioSource
}
export interface ListeningKnowledgeLinks {
  material?: PracticeKnowledgeLink[]
  sentences?: Record<number, PracticeKnowledgeLink[]>
  questions?: Record<string, PracticeKnowledgeLink[]>
}
export interface SaveListeningProgress {
  expected_cycle: number
  restart?: boolean
  stage: ListeningStage
  sentence_index: number
  position_ms: number
  bookmarks?: number[]
  draft_answers?: ListeningDraft | null
}
export interface SubmitListeningAttempt {
  phase: ListeningPhase
  answers: ListeningAnswer[]
  assisted: boolean
  idempotency_key: string
}
export type ListeningSentence = Sentence
