import { apiFetch } from './client'
import type { User, UserStats } from '../types/api'

export async function updateProfile(name: string, email: string, jlptLevels: string[]): Promise<User> {
  return apiFetch<User>('PUT', '/api/v1/users/me/profile', { name, email, jlpt_levels: jlptLevels })
}

export async function changePassword(currentPassword: string, newPassword: string): Promise<void> {
  return apiFetch<void>('PUT', '/api/v1/users/me/password', { current_password: currentPassword, new_password: newPassword })
}

export async function getStats(): Promise<UserStats> {
  return apiFetch<UserStats>('GET', '/api/v1/users/stats')
}

export async function updateDailyGoals(goals: { word: number; grammar: number; speaking: number; writing: number }): Promise<void> {
  return apiFetch<void>('PUT', '/api/v1/users/me/daily-goals', goals)
}
