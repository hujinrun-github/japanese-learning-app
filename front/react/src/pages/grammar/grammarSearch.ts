import type { GrammarSearchResult } from '@/types/api'

/** Encode a grammar annotation for the cross-level search endpoint. */
export function grammarSearchApiPath(query: string): string {
  return `/api/v1/grammar/search?q=${encodeURIComponent(query.trim())}`
}

/**
 * When the library has an exact entry for the annotation, go straight to its
 * detail page (full meaning, conjugation rule, examples and quiz). Otherwise
 * stay on the list and show the related hits.
 */
export function exactMatchPath(result: GrammarSearchResult | null): string | null {
  if (!result || result.exact_match_id <= 0) return null
  return `/grammar/${result.exact_match_id}`
}
