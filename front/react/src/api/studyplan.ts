import { apiFetch } from './client'

export interface StudyTask {
  id: string
  title: string
  description: string
  path: string
  minutes: number
  weekdays?: number[]
  practice_target?: number
  review_target?: number
}

export interface StudyTimeSlot {
  id: string
  title: string
  time: string
  duration_minutes: number
  tasks: StudyTask[]
}

export interface StudyPlan {
  title: string
  start_date: string
  end_date: string
  time: string
  timezone: string
  duration_minutes: number
  reminder_minutes: number
  weekdays: number[]
  words: number
  grammar: number
  reading: number
  speaking: number
  writing: number
  translation: number
  time_slots?: StudyTimeSlot[]
}

export interface StudyPlanResult {
  plan: StudyPlan | null
  calendar_path: string
}

export interface TodayStudySlot extends Omit<StudyTimeSlot, 'tasks'> {
  tasks: TodayStudyTask[]
}

export interface TodayStudyTask extends StudyTask {
  completed: boolean
  completion_reason?: string
  progress?: StudyPracticeProgress
  word_review?: Omit<StudyPracticeProgress, 'kind'>
}

export interface StudyPracticeProgress {
  kind: 'word_review' | 'grammar'
  target: number
  completed: number
  level: 'N5' | 'N4' | 'N3' | 'N2' | 'N1' | 'all'
  due_remaining: number
}

export interface TodayStudyPlan {
  has_plan: boolean
  date: string
  timezone: string
  title: string
  rest_day: boolean
  reason: 'no_plan' | 'legacy' | 'before_start' | 'ended' | 'rest_day' | 'study_day'
  total_minutes: number
  slots: TodayStudySlot[]
  legacy_grammar?: StudyPracticeProgress
}

export function getTodayStudyPlan(signal?: AbortSignal) {
  return apiFetch<TodayStudyPlan>('GET', '/api/v1/study-plan/today', undefined, signal)
}

export function setStudyTaskCompleted(date: string, slotID: string, taskID: string, completed: boolean, signal?: AbortSignal) {
  return apiFetch<TodayStudyPlan>('PUT', `/api/v1/study-plan/tasks/${encodeURIComponent(slotID)}/${encodeURIComponent(taskID)}`, { date, completed }, signal)
}
