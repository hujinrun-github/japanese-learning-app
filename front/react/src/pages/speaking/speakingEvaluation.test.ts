import { describe, expect, it } from 'vitest'
import type { EvaluationResult } from '@/types/shadowing'
import { initialEvaluationState, speakingEvaluationReducer } from './speakingEvaluation'

const result = { status: 'scoreable', overall_score: 83 } as EvaluationResult

describe('speaking evaluation ownership', () => {
  it.each(['success', 'failure', 'finish'] as const)('ignores late %s from a previous material', type => {
    let state = speakingEvaluationReducer(initialEvaluationState, { type: 'reset', materialID: 1, requestID: 1 })
    state = speakingEvaluationReducer(state, { type: 'start', materialID: 1, requestID: 1 })
    state = speakingEvaluationReducer(state, { type: 'reset', materialID: 2, requestID: 2 })
    state = speakingEvaluationReducer(state, { type: 'start', materialID: 2, requestID: 2 })
    const late = speakingEvaluationReducer(state, { type, materialID: 1, requestID: 1, result, error: 'late error' })
    expect(late).toBe(state)
    expect(late.evaluating).toBe(true)
    expect(late.result).toBeNull()
  })

  it('ignores a previous take of the same material and accepts the current take', () => {
    let state = speakingEvaluationReducer(initialEvaluationState, { type: 'reset', materialID: 1, requestID: 2 })
    state = speakingEvaluationReducer(state, { type: 'start', materialID: 1, requestID: 2 })
    state = speakingEvaluationReducer(state, { type: 'success', materialID: 1, requestID: 1, result })
    expect(state.result).toBeNull()
    state = speakingEvaluationReducer(state, { type: 'success', materialID: 1, requestID: 2, result })
    expect(state.result).toBe(result)
    state = speakingEvaluationReducer(state, { type: 'reset', materialID: null, requestID: 3 })
    expect(state.result).toBeNull()
    expect(state.evaluating).toBe(false)
  })
})
