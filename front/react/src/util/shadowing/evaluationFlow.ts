import { APIError } from '@/api/client'

export type EvaluationFailureKind = 'stale' | 'other'

export interface EvaluationFailure {
  kind: EvaluationFailureKind
  message: string
}

// A stale shadowing version means the lesson content changed; the caller
// should refresh the session instead of showing a generic scoring failure.
export function classifyEvaluationError(err: unknown): EvaluationFailure {
  if (err instanceof APIError && err.code === 'ERR_SHADOWING_VERSION_STALE') {
    return { kind: 'stale', message: '教材内容已更新，练习记录已刷新，请再录一次。' }
  }
  if (err instanceof Error && err.message) {
    return { kind: 'other', message: err.message }
  }
  return { kind: 'other', message: '评分失败，请稍后重试。' }
}

// Only common emoji here: U+23xx symbols (stop squares, timers) are known to
// be invisible on some platforms.
export function recordButtonLabel(isRecording: boolean, isEvaluating: boolean): string {
  if (isRecording) return '🔴 Stop recording'
  if (isEvaluating) return '🎯 Scoring...'
  return '🎤 Record'
}
