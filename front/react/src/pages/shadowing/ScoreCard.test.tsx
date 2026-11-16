import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type {
  DimensionScore,
  EvaluationIssue,
  EvaluationResult,
} from '@/types/shadowing'
import {
  DIMENSION_ORDER,
  MAX_VISIBLE_ISSUES,
  dimensionLabel,
  orderedDimensions,
  scoreBandLabel,
  unscorableMessage,
  visibleIssues,
} from '@/util/shadowing/scoreCard'
import { ScoreCard } from './ScoreCard'

function dimension(score: number, confidence: number, available = true): DimensionScore {
  return { score, confidence, weight: 0.2, available }
}

function result(overrides: Partial<EvaluationResult> = {}): EvaluationResult {
  return {
    status: 'scoreable',
    mode: 'shadow',
    overall_score: 82,
    partial: true,
    scores: {
      sync: dimension(78, 0.9),
      prosody: dimension(85, 0.9),
      coverage: dimension(0, 0, false),
      fluency: dimension(70, 0.4),
      segmental: dimension(0, 0, false),
    },
    recognized_text: '',
    lag_ms: 620,
    issues: [],
    score_source: 'local_prosody_v1',
    scorer_version: 'local-prosody-1.1',
    weights_version: 'w-shadow-1.1',
    model_version: 'none',
    shadowing_version: 1,
    latency_ms: 42,
    ...overrides,
  }
}

function issue(type: string, confidence: number): EvaluationIssue {
  return { type, confidence, message_zh: `建议：${type}`, action_zh: `动作：${type}` }
}

function advice(type: string, overrides: Partial<EvaluationIssue> = {}): EvaluationIssue {
  return {
    type,
    confidence: 0.9,
    dimension: 'sync',
    evidence_zh: `证据：${type}`,
    message_zh: `建议：${type}`,
    action_zh: `动作：${type}`,
    ...overrides,
  }
}

describe('orderedDimensions', () => {
  it('always returns the five dimensions in the fixed order', () => {
    const got = orderedDimensions(result().scores)
    expect(got.map((d) => d.key)).toEqual([...DIMENSION_ORDER])
  })

  it.each([
    { name: 'unavailable dimension is low confidence', score: dimension(0, 0, false), want: true },
    { name: 'confidence below 0.60 is low confidence', score: dimension(90, 0.3), want: true },
    { name: 'confidence exactly 0.60 is usable', score: dimension(90, 0.6), want: false },
    { name: 'high confidence is usable', score: dimension(90, 0.9), want: false },
  ])('$name', ({ score, want }) => {
    const got = orderedDimensions({ sync: score })
    expect(got[0].lowConfidence).toBe(want)
  })

  it('fills missing dimensions as unavailable', () => {
    const got = orderedDimensions({})
    expect(got).toHaveLength(5)
    expect(got.every((d) => !d.available && d.lowConfidence)).toBe(true)
  })

  it.each([
    { name: 'clamps above 100', raw: 140, want: 100 },
    { name: 'clamps below 0', raw: -20, want: 0 },
    { name: 'keeps in-range score', raw: 63.4, want: 63.4 },
  ])('$name', ({ raw, want }) => {
    expect(orderedDimensions({ sync: dimension(raw, 0.9) })[0].score).toBe(want)
  })

  it('assigns Chinese labels and emoji icons to every dimension', () => {
    for (const d of orderedDimensions({})) {
      expect(d.label.length).toBeGreaterThan(0)
      expect(d.icon.length).toBeGreaterThan(0)
    }
  })

  it('avoids obscure U+23xx symbols that are invisible on some platforms', () => {
    for (const d of orderedDimensions({})) {
      expect(d.icon).not.toMatch(/[\u2300-\u23FF]/)
    }
  })
})

describe('unscorableMessage', () => {
  it.each([
    { name: 'silence', code: 'silence', want: '没有录到声音' },
    { name: 'too short', code: 'too_short', want: '录音太短' },
    { name: 'bleed through', code: 'bleed_through', want: '录到了原音' },
    { name: 'content mismatch', code: 'content_mismatch', want: '不一致' },
    { name: 'low confidence', code: 'low_confidence', want: '证据不足' },
    { name: 'provider unavailable', code: 'provider_unavailable', want: '暂时不可用' },
    { name: 'unknown code falls back', code: 'something_new', want: '本次不评分' },
    { name: 'empty code falls back', code: '', want: '本次不评分' },
  ])('$name', ({ code, want }) => {
    expect(unscorableMessage(code)).toContain(want)
  })
})

describe('scoreBandLabel', () => {
  it.each([
    { name: '100 is excellent', score: 100, want: '很接近原音' },
    { name: '85 is excellent boundary', score: 85, want: '很接近原音' },
    { name: '84 is good', score: 84, want: '节奏基本跟上' },
    { name: '70 is good boundary', score: 70, want: '节奏基本跟上' },
    { name: '69 needs practice', score: 69, want: '还需要多练' },
    { name: '55 needs practice boundary', score: 55, want: '还需要多练' },
    { name: '54 suggests slowing down', score: 54, want: '放慢速度' },
    { name: '0 suggests slowing down', score: 0, want: '放慢速度' },
  ])('$name', ({ score, want }) => {
    expect(scoreBandLabel(score)).toContain(want)
  })
})

describe('dimensionLabel', () => {
  it.each([
    { key: 'sync', want: '同步' },
    { key: 'prosody', want: '韵律' },
    { key: 'coverage', want: '完整度' },
    { key: 'fluency', want: '流利度' },
    { key: 'segmental', want: '音段' },
  ])('$key maps to $want', ({ key, want }) => {
    expect(dimensionLabel(key)).toBe(want)
  })

  it('returns an empty label for unknown or missing dimensions', () => {
    expect(dimensionLabel('unknown')).toBe('')
    expect(dimensionLabel(undefined)).toBe('')
  })
})

describe('visibleIssues', () => {
  it('allows exactly three suggestions', () => {
    expect(MAX_VISIBLE_ISSUES).toBe(3)
  })

  it('drops low-confidence issues', () => {
    const got = visibleIssues([issue('weak', 0.4), issue('strong', 0.8)])
    expect(got.map((i) => i.type)).toEqual(['strong'])
  })

  it('keeps the priority order decided by the scorer instead of re-sorting by confidence', () => {
    const got = visibleIssues([issue('first', 0.7), issue('second', 0.95), issue('third', 0.6)])
    expect(got.map((i) => i.type)).toEqual(['first', 'second', 'third'])
  })

  it('keeps at most three issues', () => {
    const got = visibleIssues([issue('a', 0.9), issue('b', 0.8), issue('c', 0.7), issue('d', 0.6)])
    expect(got).toHaveLength(3)
    expect(got.map((i) => i.type)).toEqual(['a', 'b', 'c'])
  })

  it('returns empty for no issues', () => {
    expect(visibleIssues([])).toEqual([])
  })
})

describe('ScoreCard rendering', () => {
  it('renders the overall score, all five dimensions and partial notice', () => {
    const html = renderToStaticMarkup(<ScoreCard result={result()} />)
    expect(html).toContain('data-testid="overall-score"')
    expect(html).toContain('82')
    expect(html).toContain('部分维度')
    expect(html).toContain('同步')
    expect(html).toContain('韵律')
    expect(html).toContain('完整度')
    expect(html).toContain('流利度')
    expect(html).toContain('音段')
    expect((html.match(/data-testid="dimension-row"/g) ?? []).length).toBe(5)
  })

  it('marks low-confidence dimensions as insufficient evidence', () => {
    const html = renderToStaticMarkup(<ScoreCard result={result()} />)
    expect(html).toContain('证据不足')
  })

  it('renders at most three issues with their actions', () => {
    const html = renderToStaticMarkup(
      <ScoreCard
        result={result({ issues: [issue('a', 0.9), issue('b', 0.8), issue('c', 0.7), issue('d', 0.6)] })}
      />,
    )
    expect(html).toContain('建议：a')
    expect(html).toContain('动作：a')
    expect(html).toContain('建议：c')
    expect(html).not.toContain('建议：d')
  })

  it('renders a dedicated improvement section with measured evidence and a dimension tag', () => {
    const html = renderToStaticMarkup(
      <ScoreCard
        result={result({
          issues: [
            advice('sync_lag_late', {
              dimension: 'sync',
              evidence_zh: '起步延迟 1500 ms，目标约 600 ms，晚了 900 ms',
              message_zh: '开口比原音晚了不少',
              action_zh: '听到第一个音就立刻开口',
            }),
          ],
        })}
      />,
    )
    expect(html).toContain('下一步这样改')
    expect(html).toContain('data-testid="advice-item"')
    expect(html).toContain('data-testid="advice-dimension"')
    expect(html).toContain('data-testid="advice-evidence"')
    expect(html).toContain('起步延迟 1500 ms，目标约 600 ms，晚了 900 ms')
    expect(html).toContain('听到第一个音就立刻开口')
  })

  it('omits evidence and dimension tags when the scorer did not provide them', () => {
    const html = renderToStaticMarkup(
      <ScoreCard result={result({ issues: [issue('plain', 0.9)] })} />,
    )
    expect(html).toContain('下一步这样改')
    expect(html).not.toContain('data-testid="advice-dimension"')
    expect(html).not.toContain('data-testid="advice-evidence"')
  })

  it('omits the improvement section when there is no advice', () => {
    const html = renderToStaticMarkup(<ScoreCard result={result({ issues: [] })} />)
    expect(html).not.toContain('下一步这样改')
  })

  it('shows a specific fix on the unscorable branch too', () => {
    const html = renderToStaticMarkup(
      <ScoreCard
        result={result({
          status: 'unscorable',
          unscorable_code: 'silence',
          overall_score: undefined,
          issues: [
            advice('unscorable_silence', {
              dimension: undefined,
              evidence_zh: '没有检测到有效人声（有效音占比 0%）',
              message_zh: '这次录音几乎是静音',
              action_zh: '检查浏览器麦克风权限和系统输入音量',
            }),
          ],
        })}
      />,
    )
    expect(html).toContain('下一步这样改')
    expect(html).toContain('没有检测到有效人声（有效音占比 0%）')
    expect(html).toContain('检查浏览器麦克风权限')
    expect(html).not.toContain('data-testid="overall-score"')
    expect(html).not.toContain('data-testid="dimension-row"')
  })

  it('shows the unscorable reason without any numeric score', () => {
    const html = renderToStaticMarkup(
      <ScoreCard
        result={result({
          status: 'unscorable',
          unscorable_code: 'bleed_through',
          overall_score: undefined,
        })}
      />,
    )
    expect(html).toContain('录到了原音')
    expect(html).not.toContain('data-testid="overall-score"')
    expect(html).not.toContain('data-testid="dimension-row"')
    expect(html).not.toContain('总分')
    expect(html).not.toContain('82')
  })

  it('renders nothing when there is no result', () => {
    expect(renderToStaticMarkup(<ScoreCard result={null} />)).toBe('')
  })

  it('offers a retry button and the recording playback when provided', () => {
    const html = renderToStaticMarkup(
      <ScoreCard result={result()} recordingURL="blob:recording" onRetry={() => {}} />,
    )
    expect(html).toContain('再录一次')
    expect(html).toContain('blob:recording')
    expect(html).toContain('<audio')
  })

  it('does not render the retry button without a handler', () => {
    const html = renderToStaticMarkup(<ScoreCard result={result()} />)
    expect(html).not.toContain('再录一次')
  })
})
