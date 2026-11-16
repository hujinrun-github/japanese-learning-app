import { apiFetch } from './client'
import type {
  EvaluationMode,
  EvaluationResult,
  SaveAttemptRequest,
  SaveProgressRequest,
  ShadowingProgress,
  ShadowingSession,
} from '@/types/shadowing'

export function getShadowingSession(lessonId: number, signal?: AbortSignal) {
  return apiFetch<ShadowingSession>('GET', `/api/v1/lessons/${lessonId}/shadowing`, undefined, signal)
}

export function saveShadowingProgress(lessonId: number, body: SaveProgressRequest) {
  return apiFetch<ShadowingProgress>('POST', `/api/v1/lessons/${lessonId}/shadowing/progress`, body)
}

export function saveShadowingAttempt(lessonId: number, body: SaveAttemptRequest) {
  return apiFetch<void>('POST', `/api/v1/lessons/${lessonId}/shadowing/attempts`, body)
}

export interface EvaluateShadowingRequest {
  audio: Blob
  mode: EvaluationMode
  sentence_index: number
  shadowing_version: number
}

// Field names must match the backend multipart handler exactly. apiFetch lets
// the browser set the multipart boundary, so no Content-Type is added here.
export function buildEvaluationFormData(req: EvaluateShadowingRequest): FormData {
  const form = new FormData()
  form.append('audio', req.audio, 'attempt.wav')
  form.append('mode', req.mode)
  form.append('sentence_index', String(req.sentence_index))
  form.append('shadowing_version', String(req.shadowing_version))
  return form
}

export function evaluateShadowing(lessonId: number, req: EvaluateShadowingRequest) {
  return apiFetch<EvaluationResult>(
    'POST',
    `/api/v1/lessons/${lessonId}/shadowing/evaluations`,
    buildEvaluationFormData(req),
  )
}
