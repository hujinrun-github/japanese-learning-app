import { apiFetch } from './client'
import type {
  ListeningAttempt, ListeningCatalog, ListeningProgress, ListeningSession,
  ListeningStatus, SaveListeningProgress, SubmitListeningAttempt,
} from '@/types/listening'

const base = '/api/v1/listening/materials'
export function getListeningMaterials(level = '', status: ListeningStatus = 'all', signal?: AbortSignal) {
  const query = new URLSearchParams({ status })
  if (level) query.set('level', level)
  return apiFetch<ListeningCatalog>('GET', `${base}?${query}`, undefined, signal)
}
export function getListeningSession(id: number, signal?: AbortSignal) {
  return apiFetch<ListeningSession>('GET', `${base}/${id}`, undefined, signal)
}
export function saveListeningProgress(id: number, progress: SaveListeningProgress) {
  return apiFetch<ListeningProgress>('PUT', `${base}/${id}/progress`, progress)
}
export function submitListeningAttempt(id: number, attempt: SubmitListeningAttempt) {
  return apiFetch<ListeningAttempt>('POST', `${base}/${id}/attempts`, attempt)
}
export function completeListening(id: number) {
  return apiFetch<ListeningProgress>('POST', `${base}/${id}/complete`, {})
}
