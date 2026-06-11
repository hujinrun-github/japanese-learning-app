import type { ModuleStat } from '@/types/api'

export function percent(completed: number, total: number) {
  if (total <= 0) return 0
  return Math.max(0, Math.min(100, Math.round((completed / total) * 100)))
}

export function hasDailyGoal(stat: ModuleStat) {
  return stat.daily_goal > 0
}

export function getDailyProgress(stat: ModuleStat) {
  return percent(stat.today_completed, stat.daily_goal)
}

export function getMasteryProgress(stat: ModuleStat) {
  return percent(stat.mastered_count, stat.total_count)
}
