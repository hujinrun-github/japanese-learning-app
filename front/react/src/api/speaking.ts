import { apiFetch } from './client'
import type { EvaluationResult } from '@/types/shadowing'

export interface EvaluateSpeakingRequest {
  audio: Blob
  material_id: number
}

// Field names must match POST /api/v1/speaking/evaluate. apiFetch lets the
// browser set the multipart boundary, so no Content-Type is added here.
export function buildSpeakingEvaluationFormData(req: EvaluateSpeakingRequest): FormData {
  const form = new FormData()
  form.append('audio', req.audio, 'attempt.wav')
  form.append('material_id', String(req.material_id))
  return form
}

export function evaluateSpeaking(req: EvaluateSpeakingRequest, signal?: AbortSignal) {
  return apiFetch<EvaluationResult>(
    'POST',
    '/api/v1/speaking/evaluate',
    buildSpeakingEvaluationFormData(req),
    signal,
  )
}
