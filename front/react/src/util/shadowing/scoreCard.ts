import type { DimensionScore, EvaluationIssue } from '@/types/shadowing'

export const DIMENSION_ORDER = ['sync', 'prosody', 'coverage', 'fluency', 'segmental'] as const
export type DimensionKey = (typeof DIMENSION_ORDER)[number]

export const MIN_DIMENSION_CONFIDENCE = 0.6
export const MIN_ISSUE_CONFIDENCE = 0.6
export const MAX_VISIBLE_ISSUES = 3

const DIMENSION_META: Record<DimensionKey, { label: string; icon: string }> = {
  sync: { label: '同步', icon: '🔄' },
  prosody: { label: '韵律', icon: '🎵' },
  coverage: { label: '完整度', icon: '📝' },
  fluency: { label: '流利度', icon: '🗣️' },
  segmental: { label: '音段', icon: '🔤' },
}

const UNSCORABLE_MESSAGES: Record<string, string> = {
  silence: '没有录到声音，请检查麦克风后再试一次。',
  too_short: '录音太短，可能只跟读了半句。',
  bleed_through: '录到了原音，请戴上耳机并调低外放音量。',
  content_mismatch: '跟读内容与当前句子不一致。',
  low_confidence: '这次录音证据不足，无法给出可靠评分。',
  provider_unavailable: '评分服务暂时不可用，请稍后重试。',
}

export interface DimensionView {
  key: DimensionKey
  label: string
  icon: string
  score: number
  confidence: number
  weight: number
  available: boolean
  lowConfidence: boolean
}

function clampScore(value: number): number {
  if (!Number.isFinite(value)) return 0
  return Math.max(0, Math.min(100, value))
}

// Always return all five dimensions in a stable order. Missing or unavailable
// dimensions stay in the list but are marked as insufficient evidence so the
// UI never silently drops a scoring dimension.
export function orderedDimensions(scores: Record<string, DimensionScore>): DimensionView[] {
  return DIMENSION_ORDER.map((key) => {
    const raw = scores[key]
    const available = raw?.available ?? false
    const confidence = raw?.confidence ?? 0
    const meta = DIMENSION_META[key]
    return {
      key,
      label: meta.label,
      icon: meta.icon,
      score: clampScore(raw?.score ?? 0),
      confidence,
      weight: raw?.weight ?? 0,
      available,
      lowConfidence: !available || confidence < MIN_DIMENSION_CONFIDENCE,
    }
  })
}

export function unscorableMessage(code: string): string {
  return UNSCORABLE_MESSAGES[code] ?? '本次不评分，请再录一次。'
}

export function scoreBandLabel(score: number): string {
  if (score >= 85) return '很接近原音，保持这个节奏。'
  if (score >= 70) return '节奏基本跟上，再抠一下细节。'
  if (score >= 55) return '还需要多练几遍。'
  return '建议先放慢速度跟读。'
}

// The scorer already orders advice by expected benefit, so the client only
// filters weak evidence and caps the list. Re-sorting by confidence here would
// put a confident-but-minor tip ahead of the learner's biggest weakness.
export function visibleIssues(issues: EvaluationIssue[]): EvaluationIssue[] {
  return issues
    .filter((item) => item.confidence >= MIN_ISSUE_CONFIDENCE)
    .slice(0, MAX_VISIBLE_ISSUES)
}

export function dimensionLabel(key?: string): string {
  if (!key) return ''
  return DIMENSION_ORDER.includes(key as DimensionKey)
    ? DIMENSION_META[key as DimensionKey].label
    : ''
}
