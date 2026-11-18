/**
 * Build the grammar-library destination for a translation feedback annotation.
 * The grammar list page resolves the query to a detail page when the library
 * has an exact entry, and shows related entries otherwise.
 */
export function grammarSearchPath(point: string): string {
  const q = point.trim()
  if (!q) return '/grammar'
  return `/grammar?q=${encodeURIComponent(q)}`
}
