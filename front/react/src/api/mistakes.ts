import { apiFetch } from './client'
import type { CreateMistakeInput, MistakeList, MistakeStatus } from '@/types/api'

export interface MistakeQuery {
  module?: string
  status?: string
  page?: number
  size?: number
}

export function createMistake(input: CreateMistakeInput): Promise<{ id: number }> {
  return apiFetch('POST', '/api/v1/mistakes', input)
}

export function getMistakes(query: MistakeQuery = {}): Promise<MistakeList> {
  const params = new URLSearchParams()
  if (query.module) params.set('module', query.module)
  if (query.status) params.set('status', query.status)
  if (query.page) params.set('page', String(query.page))
  if (query.size) params.set('size', String(query.size))
  const suffix = params.toString()
  return apiFetch('GET', `/api/v1/mistakes${suffix ? `?${suffix}` : ''}`)
}

export function setMistakeStatus(id: number, status: MistakeStatus): Promise<{ id: number; status: MistakeStatus }> {
  return apiFetch('PATCH', `/api/v1/mistakes/${id}`, { status })
}
