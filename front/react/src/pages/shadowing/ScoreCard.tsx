import type { EvaluationIssue, EvaluationResult } from '@/types/shadowing'
import {
  dimensionLabel,
  orderedDimensions,
  scoreBandLabel,
  unscorableMessage,
  visibleIssues,
} from '@/util/shadowing/scoreCard'
import styles from './ScoreCard.module.css'

interface ScoreCardProps {
  result: EvaluationResult | null
  recordingURL?: string | null
  onRetry?: () => void
}

function AdviceItem({ item }: { item: EvaluationIssue }) {
  const label = dimensionLabel(item.dimension)
  return (
    <div className={styles.issue} data-testid="advice-item">
      {(label || item.evidence_zh) && (
        <div className={styles.adviceHead}>
          {label && (
            <span className={styles.dimTag} data-testid="advice-dimension">{label}</span>
          )}
          {item.evidence_zh && (
            <span className={styles.evidence} data-testid="advice-evidence">{item.evidence_zh}</span>
          )}
        </div>
      )}
      <p className={styles.issueMessage}>💡 {item.message_zh}</p>
      {item.action_zh && <p className={styles.issueAction}>👉 {item.action_zh}</p>}
    </div>
  )
}

export function ScoreCard({ result, recordingURL, onRetry }: ScoreCardProps) {
  if (!result) return null

  const overall = result.overall_score
  const scoreable = result.status === 'scoreable' && typeof overall === 'number'
  const score = typeof overall === 'number' ? overall : 0
  const dimensions = orderedDimensions(result.scores)
  const advice = visibleIssues(result.issues)

  return (
    <section className={styles.card} aria-label="本次跟读评分">
      <header className={styles.header}>
        <h2 className={styles.title}>🎯 本次跟读评分</h2>
        {scoreable && result.partial && (
          <span className={styles.partialBadge}>部分维度暂无数据</span>
        )}
      </header>

      {scoreable ? (
        <>
          <div className={styles.overall}>
            <span data-testid="overall-score" className={styles.overallScore}>{score}</span>
            <span className={styles.overallUnit}>分</span>
            <span className={styles.band}>{scoreBandLabel(score)}</span>
          </div>

          <ul className={styles.dimensions}>
            {dimensions.map((item) => (
              <li
                key={item.key}
                data-testid="dimension-row"
                className={`${styles.dimensionRow} ${item.lowConfidence ? styles.dimensionMuted : ''}`}
              >
                <span className={styles.dimIcon} aria-hidden="true">{item.icon}</span>
                <span className={styles.dimLabel}>{item.label}</span>
                <span className={styles.bar}>
                  <span className={styles.barFill} style={{ width: `${item.score}%` }} />
                </span>
                {item.lowConfidence ? (
                  <span className={styles.insufficient}>证据不足</span>
                ) : (
                  <span className={styles.dimScore}>{Math.round(item.score)}</span>
                )}
              </li>
            ))}
          </ul>
        </>
      ) : (
        <div className={styles.unscorable}>
          <p className={styles.reason}>⚠️ {unscorableMessage(result.unscorable_code ?? '')}</p>
          <p className={styles.hint}>本次不显示分数，按下面的建议调整后再录一次。</p>
        </div>
      )}

      {advice.length > 0 && (
        <div className={styles.advice}>
          <h3 className={styles.adviceTitle}>📋 下一步这样改</h3>
          {advice.map((item) => <AdviceItem key={item.type} item={item} />)}
        </div>
      )}

      {scoreable && (
        <p className={styles.meta}>
          评分版本 {result.scorer_version} · 权重 {result.weights_version}
        </p>
      )}

      {recordingURL && (
        <div className={styles.playback}>
          <span className={styles.playbackLabel}>🎧 我的录音</span>
          <audio controls src={recordingURL} />
        </div>
      )}

      {onRetry && (
        <button type="button" className={styles.retry} onClick={onRetry}>
          🔁 再录一次
        </button>
      )}
    </section>
  )
}
