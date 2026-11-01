import { apiFetch } from './client'
import type {
  NoteSyncBinding,
  NoteSyncResult,
  NoteSyncState,
  NoteSyncTarget,
  NoteSyncTargetInput,
} from '../types/api'

export function listNoteSyncTargets() {
  return apiFetch<NoteSyncTarget[]>('GET', '/api/v1/note-sync/targets')
}

export function createNoteSyncTarget(input: NoteSyncTargetInput) {
  return apiFetch<NoteSyncTarget>('POST', '/api/v1/note-sync/targets', input)
}

export function updateNoteSyncTarget(id: number, input: NoteSyncTargetInput) {
  return apiFetch<NoteSyncTarget>('PUT', `/api/v1/note-sync/targets/${id}`, input)
}

export function deleteNoteSyncTarget(id: number) {
  return apiFetch<void>('DELETE', `/api/v1/note-sync/targets/${id}`)
}

export function testNoteSyncTarget(input: NoteSyncTargetInput) {
  return apiFetch<{ ok: boolean }>('POST', '/api/v1/note-sync/targets/test', input)
}

export function getNoteSyncBinding(noteID: number) {
  return apiFetch<NoteSyncBinding | null>('GET', `/api/v1/notes/${noteID}/sync-binding`)
}

export function setNoteSyncBinding(noteID: number, targetID: number | null) {
  return apiFetch<NoteSyncBinding | null>('PUT', `/api/v1/notes/${noteID}/sync-binding`, { target_id: targetID })
}

export function clearNoteSyncBinding(noteID: number) {
  return apiFetch<void>('DELETE', `/api/v1/notes/${noteID}/sync-binding`)
}

export function getNoteSyncState(noteID: number) {
  return apiFetch<NoteSyncState | null>('GET', `/api/v1/notes/${noteID}/sync-state`)
}

export function pushNoteToNotion(noteID: number) {
  return apiFetch<NoteSyncState>('POST', `/api/v1/note-sync/notes/${noteID}/push`)
}

export function pushNoteSyncTarget(targetID: number) {
  return apiFetch<NoteSyncResult>('POST', `/api/v1/note-sync/targets/${targetID}/push`)
}

export function pullNoteSyncTarget(targetID: number) {
  return apiFetch<NoteSyncResult>('POST', `/api/v1/note-sync/targets/${targetID}/pull`)
}

export function listNoteSyncDeletions(targetID: number) {
  return apiFetch<NoteSyncState[]>('GET', `/api/v1/note-sync/targets/${targetID}/deletions`)
}

export function confirmNoteSyncDeletion(targetID: number, noteID: number) {
  return apiFetch<void>('POST', `/api/v1/note-sync/targets/${targetID}/deletions/${noteID}/confirm`)
}

export function restoreNoteSyncDeletion(targetID: number, noteID: number) {
  return apiFetch<NoteSyncState>('POST', `/api/v1/note-sync/targets/${targetID}/deletions/${noteID}/restore`)
}
