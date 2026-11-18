import type { EvaluationResult } from '@/types/shadowing'

export interface SpeakingEvaluationState {
  materialID: number | null
  requestID: number
  evaluating: boolean
  result: EvaluationResult | null
  error: string
}

export const initialEvaluationState: SpeakingEvaluationState = {
  materialID: null, requestID: 0, evaluating: false, result: null, error: '',
}

type EvaluationAction = {
  type: 'reset' | 'start' | 'success' | 'failure' | 'finish'
  materialID: number | null
  requestID: number
  result?: EvaluationResult
  error?: string
}

export function speakingEvaluationReducer(state: SpeakingEvaluationState, action: EvaluationAction): SpeakingEvaluationState {
  if (action.type === 'reset') {
    return { ...initialEvaluationState, materialID: action.materialID, requestID: action.requestID }
  }
  if (action.requestID !== state.requestID || action.materialID !== state.materialID) return state
  switch (action.type) {
    case 'start': return { ...state, evaluating: true, result: null, error: '' }
    case 'success': return { ...state, result: action.result ?? null }
    case 'failure': return { ...state, error: action.error ?? '' }
    case 'finish': return { ...state, evaluating: false }
  }
}
