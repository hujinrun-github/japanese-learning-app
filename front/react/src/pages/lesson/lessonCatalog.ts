import type { JLPTLevel, LessonSummary } from '@/types/api'

export const CATALOG_SIZE = 20
export const CATALOG_LEVELS = ['', 'N5', 'N4', 'N3', 'N2', 'N1'] as const satisfies readonly (JLPTLevel | '')[]

export interface CatalogFilters {
  level: (typeof CATALOG_LEVELS)[number]
  q: string
  page: number
}

export interface LessonCatalogPage {
  items: Pick<LessonSummary, 'id' | 'title' | 'jlpt_level' | 'tags' | 'char_count'>[]
  total: number
  page: number
  size: number
}

export function getCatalogFilters(params: URLSearchParams): CatalogFilters {
  const level = CATALOG_LEVELS.find((candidate) => candidate === params.get('level')) ?? ''
  const rawPage = params.get('page') ?? ''
  const page = Number(rawPage)
  return {
    level,
    q: params.get('q')?.trim() ?? '',
    page: /^\d+$/.test(rawPage) && page >= 1 && page <= 2_147_483_647 ? page : 1,
  }
}

export function catalogParams({ level, q, page }: CatalogFilters): URLSearchParams {
  const params = new URLSearchParams()
  if (level) params.set('level', level)
  if (q) params.set('q', q)
  if (page > 1) params.set('page', String(page))
  return params
}
