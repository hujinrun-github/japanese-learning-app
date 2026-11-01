import type { DailyPlan, DailyPlanItem } from '@/types/api'

// moduleOrder is the canonical display order used when several plan items
// share the same completion status.
const moduleOrder: Record<string, number> = {
  word: 0,
  grammar: 1,
  speaking: 2,
  writing: 3,
}

/**
 * Picks the module the learner should work on next.
 *
 * Started-but-unfinished modules win over untouched ones so the learner
 * finishes what is already open; ties fall back to the canonical module
 * order. Returns null when the plan is missing or fully complete.
 */
export function pickNextPlanItem(plan: DailyPlan | null): DailyPlanItem | null {
  if (!plan || plan.items.length === 0) {
    return null
  }

  const pending = plan.items.filter(item => item.status !== 'done')
  if (pending.length === 0) {
    return null
  }

  return pending.reduce((best, item) => {
    if (item.status === 'in_progress' && best.status !== 'in_progress') {
      return item
    }
    if (item.status !== 'in_progress' && best.status === 'in_progress') {
      return best
    }
    return (moduleOrder[item.key] ?? 99) < (moduleOrder[best.key] ?? 99) ? item : best
  })
}

/** True when every item in the plan has hit its goal. */
export function isPlanComplete(plan: DailyPlan | null): boolean {
  return !!plan && plan.items.length > 0 && plan.items.every(item => item.status === 'done')
}
