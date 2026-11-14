import { apiFetch } from '@/api/client'
import type { LessonSelection } from './lessonSelection'

export type ReferenceKind = 'word' | 'grammar'

export interface AnnotationReference {
  kind: ReferenceKind
  id: number
  title: string
  reading?: string
  meaning: string
}

export interface LessonAnnotation extends LessonSelection {
  comment: string
  links: AnnotationReference[]
}

export function loadLessonAnnotations(lessonId: number, signal: AbortSignal) {
  return apiFetch<LessonAnnotation[]>('GET', `/api/v1/lessons/${lessonId}/annotations`, undefined, signal)
}

export function saveLessonAnnotation(lessonId: number, annotation: LessonAnnotation) {
  return apiFetch<LessonAnnotation>('PUT', `/api/v1/lessons/${lessonId}/annotations`, annotation)
}

export function removeLessonAnnotation(lessonId: number, { start, end }: LessonSelection) {
  return apiFetch<void>('DELETE', `/api/v1/lessons/${lessonId}/annotations/${start}/${end}`)
}

export function searchAnnotationReferences(kind: ReferenceKind, q: string, signal: AbortSignal) {
  const params = new URLSearchParams({ kind, q })
  return apiFetch<AnnotationReference[]>('GET', `/api/v1/lessons/annotation-references?${params}`, undefined, signal)
}
