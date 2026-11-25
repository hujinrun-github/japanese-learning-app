import { describe, expect, it } from 'vitest'
import type { ListeningMaterial, ListeningPhase, ListeningStage } from '@/types/listening'
import { answerList, canSubmitAnswers, getListeningFilters, getListeningStepAction, listeningPath, needsBlindBaseline, nextUnpracticedMaterial, restoreExercise, sentenceBounds, shouldReportPlaybackError } from './listeningFlow'

describe('next listening material', () => {
  const ready = (id: number, status: ListeningMaterial['status'] = 'new') => ({ id, status, training_ready: true })
  const materials = [ready(1), ready(2, 'practiced'), ready(3, 'due'), { ...ready(4), training_ready: false }, ready(5)]

  it.each([
    { current: 1, expected: 5 },
    { current: 2, expected: 5 },
    { current: 5, expected: 1 },
  ])('continues from $current to $expected in catalogue order', ({ current, expected }) => {
    expect(nextUnpracticedMaterial(materials, current)?.id).toBe(expected)
  })

  it('leaves a completed result open when there is no eligible material', () => {
    expect(nextUnpracticedMaterial(materials.slice(0, 4), 1)).toBeUndefined()
    expect(nextUnpracticedMaterial([], 1)).toBeUndefined()
  })

  it.each([
    ['', '/listening/5', '/listening'],
    ['level=N2&status=due', '/listening/5?level=N2&status=due', '/listening?level=N2&status=due'],
    ['level=unknown&status=unknown', '/listening/5', '/listening'],
  ])('keeps the active catalogue filters from %s', (search, materialPath, cataloguePath) => {
    const filters = getListeningFilters(new URLSearchParams(search))
    expect(listeningPath(filters, 5)).toBe(materialPath)
    expect(listeningPath(filters)).toBe(cataloguePath)
  })
})

describe('listening step navigation', () => {
  it.each<{ saved: ListeningStage; target: ListeningStage; phases: ListeningPhase[]; ready?: boolean; hasAudio?: boolean; expected: string }>([
    { saved: 'blind', target: 'intensive', phases: [], expected: 'locked' },
    { saved: 'blind', target: 'retest', phases: [], expected: 'locked' },
    { saved: 'blind', target: 'intensive', phases: ['blind'], expected: 'advance' },
    { saved: 'blind', target: 'intensive', phases: [], ready: false, expected: 'advance' },
    { saved: 'blind', target: 'intensive', phases: [], ready: false, hasAudio: false, expected: 'locked' },
    { saved: 'intensive', target: 'blind', phases: [], expected: 'advance' },
    { saved: 'intensive', target: 'blind', phases: ['blind'], expected: 'view' },
    { saved: 'intensive', target: 'retest', phases: [], expected: 'locked' },
    { saved: 'intensive', target: 'retest', phases: ['blind'], expected: 'advance' },
    { saved: 'intensive', target: 'retest', phases: ['blind'], ready: false, expected: 'locked' },
    { saved: 'retest', target: 'intensive', phases: ['blind'], expected: 'view' },
    { saved: 'retest', target: 'completed', phases: ['blind'], expected: 'locked' },
    { saved: 'retest', target: 'completed', phases: ['blind', 'retest'], expected: 'view' },
    { saved: 'completed', target: 'blind', phases: ['blind', 'retest'], expected: 'view' },
    { saved: 'completed', target: 'intensive', phases: ['blind', 'retest'], expected: 'view' },
    { saved: 'completed', target: 'retest', phases: ['blind', 'retest'], expected: 'view' },
    { saved: 'completed', target: 'completed', phases: ['blind', 'retest'], expected: 'view' },
  ])('resolves $saved → $target with $phases and readiness $ready', ({ saved, target, phases, ready = true, hasAudio = true, expected }) => {
    expect(getListeningStepAction(saved, target, ready, hasAudio, phases.map(phase => ({ phase })))).toBe(expected)
  })
})

describe('listening exercise recovery and submission', () => {
  it.each([
    { stage: 'blind', expected: 'blind' },
    { stage: 'intensive', expected: 'intensive' },
    { stage: 'retest', expected: 'retest' },
    { stage: 'completed', expected: 'completed' },
  ])('restores the saved $stage stage without resetting practice', ({ stage, expected }) => {
    const restored = restoreExercise({ stage, sentence_index: 4, position_ms: 34000, bookmarks: [2, 4], draft_answers: { phase: 'retest', assisted: true, answers: [{ question_id: 'q1', option_index: 2 }] } } as never)
    expect(restored.stage).toBe(expected)
    expect(restored.sentenceIndex).toBe(4)
    expect(restored.positionMs).toBe(34000)
    expect(restored.bookmarks).toEqual([2, 4])
    expect(restored.answers.retest).toEqual({ q1: 2 })
    expect(restored.assisted).toEqual({ blind: false, retest: true })
  })

  it.each([
    { answers: {}, expected: false },
    { answers: { q1: 0 }, expected: false },
    { answers: { q1: 0, q2: 1 }, expected: true },
    { answers: { q1: -1, q2: 1 }, expected: false },
    { answers: { q1: 0, q2: 3 }, expected: false },
  ])('requires a valid option for every comprehension question: $answers', ({ answers, expected }) => {
    expect(canSubmitAnswers([{ id: 'q1', options: ['a', 'b'] }, { id: 'q2', options: ['a', 'b'] }] as never, answers as Record<string, number>)).toBe(expected)
  })

  it('submits only question ids from the current material, including option zero', () => {
    expect(answerList([{ id: 'q1' }, { id: 'q2' }] as never, { q1: 0, q2: 1, stale: 3 })).toEqual([
      { question_id: 'q1', option_index: 0 }, { question_id: 'q2', option_index: 1 },
    ])
  })

  it.each([
    { start_ms: 1000, end_ms: 3300, expected: { start: 1, end: 3.3 } },
    { start_ms: 0, end_ms: 0, expected: null },
    { start_ms: 4000, end_ms: 1000, expected: null },
  ])('never plays a whole article as an unaligned single sentence', ({ start_ms, end_ms, expected }) => {
    expect(sentenceBounds({ start_ms, end_ms } as never)).toEqual(expected)
  })

  it('defaults invalid filters without changing the selected level', () => {
    expect(getListeningFilters(new URLSearchParams('level=N2&status=invalid'))).toEqual({ level: 'N2', status: 'all' })
  })

  it.each([
    { ready: true, stage: 'intensive', phases: [], expected: true },
    { ready: true, stage: 'retest', phases: [], expected: true },
    { ready: true, stage: 'retest', phases: ['retest'], expected: true },
    { ready: true, stage: 'intensive', phases: ['blind'], expected: false },
    { ready: true, stage: 'retest', phases: ['blind'], expected: false },
    { ready: false, stage: 'intensive', phases: [], expected: false },
    { ready: true, stage: 'blind', phases: [], expected: false },
    { ready: true, stage: 'completed', phases: [], expected: false },
  ])('requires a baseline after questions are added: $stage/$ready/$phases', ({ ready, stage, phases, expected }) => {
    expect(needsBlindBaseline(ready, stage as never, phases.map((phase) => ({ phase })) as never)).toBe(expected)
  })

  it.each([
    { name: 'a user pausing or switching sentences', error: new DOMException('Playback was interrupted', 'AbortError'), expected: false },
    { name: 'an unsupported audio format', error: new DOMException('Format not supported', 'NotSupportedError'), expected: true },
    { name: 'a network failure', error: new DOMException('Network request failed', 'NetworkError'), expected: true },
    { name: 'an ordinary playback exception', error: new Error('Cannot play'), expected: true },
    { name: 'an unknown failure', error: null, expected: true },
  ])('reports $name appropriately', ({ error, expected }) => {
    expect(shouldReportPlaybackError(error)).toBe(expected)
  })
})
