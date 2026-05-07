// API response wrapper from backend
export interface APIResponse<T> {
  data: T
}

export interface APIError {
  code: string
  message: string
}

// JLPT level
export type JLPTLevel = 'N5' | 'N4' | 'N3' | 'N2' | 'N1'

// User
export interface User {
  id: number
  name: string
  email: string
  jlpt_level: JLPTLevel
  streak_days: number
  created_at: string
}

export interface ModuleStat {
  due_count: number
  mastered_count: number
  total_count: number
}

export interface UserStats {
  streak_days: number
  modules: Record<string, ModuleStat>
}

// Word
export interface WordExample {
  japanese: string
  chinese: string
  furigana_html?: string
}

export interface Word {
  id: number
  kanji_form: string
  reading: string
  part_of_speech: string
  meaning: string
  jlpt_level: JLPTLevel
  examples: WordExample[]
  reading_type: string
}

export interface WordRecord {
  id: number
  user_id: number
  word_id: number
  mastery_level: number
  next_review_at: string
  ease_factor: number
  interval: number
  updated_at: string
}

export interface WordCard {
  word: Word
  record: WordRecord
  is_new: boolean
}

// Grammar
export type QuizType = 'fill_blank' | 'multi_choice'

export interface QuizQuestion {
  id: number
  type: QuizType
  prompt: string
  options?: string[]
  explanation: string
}

export interface GrammarExample {
  japanese: string
  chinese: string
}

export interface GrammarPoint {
  id: number
  name: string
  meaning: string
  conjunction_rule: string
  usage_note: string
  jlpt_level: JLPTLevel
  examples: GrammarExample[]
  quiz_questions: QuizQuestion[]
}

export interface GrammarPointWithStatus extends GrammarPoint {
  user_status: 'unlearned' | 'learning' | 'mastered'
}

export interface QuizSubmission {
  question_id: number
  answer: string
}

export interface QuizItemResult {
  question_id: number
  correct: boolean
  user_answer: string
  expected: string
  explanation: string
}

export interface QuizResult {
  score: number
  results: QuizItemResult[]
}

// Lesson
export interface FuriganaToken {
  surface: string
  reading: string
}

export interface Sentence {
  index: number
  tokens: FuriganaToken[]
  translation: string
  audio_url?: string
}

export interface LessonSummary {
  id: number
  title: string
  jlpt_level: JLPTLevel
  tags: string[]
  sentence_count: number
}

export interface Lesson extends LessonSummary {
  sentences: Sentence[]
}

// Speaking
export interface SentenceAnnotation {
  sentence_index: number
  score: number
  note: string
}

export interface ScoreResult {
  overall_score: number
  annotations: SentenceAnnotation[]
}

export interface SpeakingRecord {
  id: number
  material_id: number
  score: ScoreResult
  practiced_at: string
}

// Writing
export interface AIFeedback {
  score: number
  grammar_correct: boolean
  vocab_correct: boolean
  issue_description: string
  corrected_sentence: string
  alternative_phrases: string[]
  reference_answer: string
}

export interface WritingQuestion {
  id: number
  type: 'input' | 'sentence'
  prompt: string
  hint?: string
}

export interface WritingRecord {
  id: number
  type: 'input' | 'sentence'
  question: string
  user_answer: string
  ai_feedback?: AIFeedback
  score: number
  practiced_at: string
}

// Summary
export interface SessionSummary {
  session_id: string
  module: string
  strengths: string[]
  weaknesses: string[]
  suggestions: string
  score_summary: Record<string, number>
  created_at: string
}
