import { useEffect, useState } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { getListeningMaterials } from '@/api/listening'
import { Badge } from '@/components/ui/Badge'
import { Spinner } from '@/components/ui/Spinner'
import type { ListeningCatalog, ListeningMaterial, ListeningStatus } from '@/types/listening'
import { audioSourceLabel, formatDueDate, getListeningFilters, listeningPath, STAGE_LABELS } from './listeningFlow'
import styles from './ListeningPage.module.css'

const statuses: [ListeningStatus, string][] = [['all', '全部'], ['new', '未练习'], ['practiced', '已练习'], ['due', '待复习']]

export function ListeningPage() {
  const [search, setSearch] = useSearchParams()
  const filters = getListeningFilters(search)
  const { level, status } = filters
  const [retry, setRetry] = useState(0)
  const requestKey = `${level}:${status}:${retry}`
  const [result, setResult] = useState<{ key: string; data?: ListeningCatalog; error?: string }>()
  const current = result?.key === requestKey ? result : undefined
  const data = current?.data
  const suggestion = data?.materials.find((material) => material.status === 'due')
    ?? data?.materials.find((material) => hasStarted(material) && material.progress?.stage !== 'completed')
    ?? data?.materials[0]

  useEffect(() => {
    const controller = new AbortController()
    getListeningMaterials(level, status, controller.signal)
      .then((data) => { if (!controller.signal.aborted) setResult({ key: requestKey, data }) })
      .catch((error: unknown) => { if (!controller.signal.aborted) setResult({ key: requestKey, error: error instanceof Error ? error.message : '听力素材加载失败' }) })
    return () => controller.abort()
  }, [level, status, requestKey])

  function filter(nextLevel: string, nextStatus: ListeningStatus) {
    const params = new URLSearchParams({ status: nextStatus })
    if (nextLevel) params.set('level', nextLevel)
    setSearch(params)
  }

  return <div className={styles.page}>
    <header className={styles.header}>
      <div><p className={styles.eyebrow}>每天听懂一点</p><h1>🎧 听力训练</h1><p className={styles.subtitle}>先听懂大意，再逐句突破，最后关掉文本验证。</p></div>
      <Link className={styles.textLink} to="/study-plan">📅 学习计划</Link>
    </header>

    {data && <section className={styles.overview} aria-label="今日听力建议">
      <div className={styles.recommendation}>
        <span className={styles.eyebrow}>今日建议 · 约 15–20 分钟</span>
        <h2>{suggestion ? (suggestion.status === 'due' ? '先复习，再听新内容' : '完成一篇教材精听') : '暂时没有可训练素材'}</h2>
        <p>{suggestion ? suggestion.title : '试试其他级别，或稍后再来。'}</p>
        {suggestion && <Link className={styles.primaryLink} to={listeningPath(filters, suggestion.id)}>{hasStarted(suggestion) && suggestion.progress?.stage !== 'completed' ? '继续训练' : suggestion.status === 'due' ? '开始复习' : suggestion.training_ready === false ? '开始精听' : '开始听力训练'}</Link>}
      </div>
      <Link className={styles.reviewCard} to={listeningPath({ level, status: 'due' })}>
        <span>🔁 待复习</span><strong>{data.stats.due}</strong><span>篇素材 · 重新盲听检查掌握情况</span>
      </Link>
    </section>}

    <section className={styles.catalog} aria-labelledby="listening-catalog-title">
      <div className={styles.sectionHeading}><h2 id="listening-catalog-title">教材听力库</h2>{data && <span className={styles.muted}>{data.stats.total} 篇素材</span>}</div>
      <div className={styles.filters} role="group" aria-label="听力级别">
        {['', 'N5', 'N4', 'N3', 'N2', 'N1'].map((option) => <button key={option} type="button" aria-pressed={level === option} onClick={() => filter(option, status)}>{option || '全部级别'}</button>)}
      </div>
      <div className={styles.filters} role="group" aria-label="听力练习状态">
        {statuses.map(([value, label]) => <button key={value} type="button" aria-pressed={status === value} onClick={() => filter(level, value)}>{label}{value !== 'all' && data ? ` ${data.stats[value]}` : ''}</button>)}
      </div>
      {!current ? <div className={styles.loading}><Spinner /><span>正在加载听力素材…</span></div>
        : current.error ? <div className={styles.error} role="alert"><p>{current.error}</p><button className={styles.secondaryButton} type="button" onClick={() => setRetry((value) => value + 1)}>重试加载</button></div>
          : !data?.materials.length ? <div className={styles.empty}>当前条件下暂无听力素材，试试其他级别或状态。</div>
            : <div className={styles.materialList}>{data.materials.map((material) => <MaterialCard key={material.id} material={material} to={listeningPath(filters, material.id)} />)}</div>}
    </section>
  </div>
}

function MaterialCard({ material, to }: { material: ListeningMaterial; to: string }) {
  const status = material.status === 'due' ? '待复习' : material.status === 'practiced' ? '已练习' : '未练习'
  return <Link className={styles.materialCard} to={to}>
    <div className={styles.materialIcon} aria-hidden="true">🎧</div>
    <div className={styles.materialBody}>
      <h3 lang="ja">{material.title}</h3>
      <p>{material.sentence_count} 句{material.duration_ms > 0 ? ` · ${Math.max(1, Math.round(material.duration_ms / 60000))} 分钟音频` : ''} · {audioSourceLabel(material.audio_source)} · {material.question_count > 0 ? material.training_ready === false ? '精听素材 · 训练条件待补充' : `${material.question_count} 道理解题` : '精听素材 · 理解题待补充'}</p>
      {material.progress && hasStarted(material) && <small>{material.progress.stage === 'completed' ? `下次复习：${formatDueDate(material.progress.due_at)}` : `继续：${STAGE_LABELS[material.progress.stage]}`}</small>}
    </div>
    <div className={styles.materialMeta}><Badge level={material.jlpt_level} size="sm" /><span className={material.status === 'due' ? styles.due : styles.muted}>{status}</span><span>进入训练</span></div>
  </Link>
}

function hasStarted(material: ListeningMaterial) {
  return !!material.progress && (material.progress.stage !== 'blind' || material.progress.position_ms > 0 || !!material.progress.draft_answers?.answers.length)
}
