import { apiFetch } from './client'
import type {
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
