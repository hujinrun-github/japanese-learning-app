import { APIError, apiFetch } from '@/api/client'
import { getMistakes } from '@/api/mistakes'
import type { GrammarPoint, Mistake, Word } from '@/types/api'
import { wordReviewTargetID } from '@/pages/word/wordReviewTarget'

export interface WeakGrammarRecommendation {
  name: string
  to: string
}

export interface WeakWordRecommendation {
  name: string
  to: string
}

export function wordMistakePath(mistake: Pick<Mistake, 'module' | 'status' | 'item_key' | 'source_path'>): string | null {
  if (mistake.module !== 'word' || mistake.status !== 'unmastered') return null
  const key = /^word:([1-9]\d*)(?::(reading|dictation))?$/.exec(mistake.item_key)
  const source = /^\/words\/review(?:\?|$)/.test(mistake.source_path)
    ? new URLSearchParams(mistake.source_path.split('?')[1] ?? '') : null
  const id = wordReviewTargetID(key?.[1] ?? source?.get('word_id') ?? null)
  if (id === null) return null
  const mode = key ? key[2] : source?.get('mode')
  return `/words/review?word_id=${id}${mode === 'dictation' ? '&mode=dictation' : ''}`
}

export async function loadWeakWordRecommendation(): Promise<WeakWordRecommendation | null> {
  const mistakes = await getMistakes({ module: 'word', status: 'unmastered', page: 1, size: 10 })
  const visited = new Set<string>()
  for (const mistake of mistakes?.items ?? []) {
    const to = wordMistakePath(mistake)
    if (!to) continue
    const id = new URLSearchParams(to.split('?')[1]).get('word_id')!
    if (visited.has(id)) continue
    visited.add(id)
    try {
      const word = await apiFetch<Word>('GET', `/api/v1/words/${id}`)
      if (!word || String(word.id) !== id) continue
      const spelling = word.kanji_form.trim() || word.reading.trim()
      if (!spelling) continue
      return { to, name: word.reading && spelling !== word.reading ? `${spelling}（${word.reading}）` : spelling }
    } catch (err) {
      if (err instanceof APIError && err.status === 404) continue
      throw err
    }
  }
  return null
}

export function grammarMistakePath(mistake: Pick<Mistake, 'module' | 'status' | 'item_key' | 'source_path'>): string | null {
  if (mistake.module !== 'grammar' || mistake.status !== 'unmastered') return null
  const keyMatch = /^grammar:([1-9]\d*)(?::.*)?$/.exec(mistake.item_key)
  const sourceMatch = /^\/grammar\/([1-9]\d*)(?:[?#].*)?$/.exec(mistake.source_path)
  const id = Number(keyMatch?.[1] ?? sourceMatch?.[1])
  return Number.isSafeInteger(id) && id > 0 ? `/grammar/${id}` : null
}

export async function loadWeakGrammarRecommendation(): Promise<WeakGrammarRecommendation | null> {
  const mistakes = await getMistakes({ module: 'grammar', status: 'unmastered', page: 1, size: 10 })
  const visited = new Set<string>()
  for (const mistake of mistakes?.items ?? []) {
    const to = grammarMistakePath(mistake)
    if (!to || visited.has(to)) continue
    visited.add(to)
    try {
      const point = await apiFetch<GrammarPoint>('GET', `/api/v1${to}`)
      if (point && to === `/grammar/${point.id}` && point.name.trim()) {
        return { name: point.name, to }
      }
    } catch (err) {
      // A removed grammar entry must not produce a broken recommendation.
      if (err instanceof APIError && err.status === 404) continue
      throw err
    }
  }
  return null
}
