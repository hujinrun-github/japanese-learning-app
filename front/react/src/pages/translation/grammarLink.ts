const roundPath = '/translation/practice'

export function translationRoundPath(search: URLSearchParams, subpath = '', grammarQuery = '') {
  const params = new URLSearchParams()
  for (const key of ['mode', 'source_id', 'review']) {
    const value = search.get(key)
    if (value !== null) params.set(key, value)
  }
  if (grammarQuery) params.set('q', grammarQuery)
  return `${roundPath}${subpath}${params.size ? `?${params}` : ''}`
}

export function grammarSearchPath(point: string, search = new URLSearchParams()): string {
  const q = point.trim()
  return translationRoundPath(search, '/grammar', q)
}
