import { DailyPracticeProgress } from '@/components/ui/DailyPracticeProgress'
import { useDailyPracticeProgress } from '@/hooks/useDailyPracticeProgress'
import { useState, useEffect } from 'react'
import { useNavigate, useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Badge } from '@/components/ui/Badge'
import { StatusBadge } from '@/components/ui/StatusBadge'
import { Spinner } from '@/components/ui/Spinner'
import { EmptyState } from '@/components/ui/EmptyState'
import type { GrammarPointWithStatus, GrammarSearchResult, JLPTLevel } from '@/types/api'
import { exactMatchPath, grammarSearchApiPath } from './grammarSearch'
import styles from './GrammarListPage.module.css'

const LEVELS: JLPTLevel[] = ['N5', 'N4', 'N3', 'N2', 'N1']

type FilterKey = '' | 'learning' | 'mastered' | 'unlearned'

const FILTERS: { key: FilterKey; labelKey: string }[] = [
  { key: '', labelKey: 'grammar.list.filter.all' },
  { key: 'learning', labelKey: 'grammar.list.filter.learning' },
  { key: 'mastered', labelKey: 'grammar.list.filter.mastered' },
  { key: 'unlearned', labelKey: 'grammar.list.filter.unlearned' },
]

function filterPoints(items: GrammarPointWithStatus[], f: FilterKey): GrammarPointWithStatus[] {
  if (f === 'learning') return items.filter((p) => p.user_status === 'learning')
  if (f === 'mastered') return items.filter((p) => p.user_status === 'mastered')
  if (f === 'unlearned') return items.filter((p) => p.user_status === 'unlearned')
  return items
}

export function GrammarListPage() {
  const { t } = useTranslation()
  const { progress } = useDailyPracticeProgress('grammar')
  const navigate = useNavigate()
  const [searchParams, setSearchParams] = useSearchParams()
  const query = (searchParams.get('q') ?? '').trim()
  const isSearching = query !== ''

  const level = LEVELS.find(item => item === searchParams.get('level')) ?? 'N5'
  const [filter, setFilter] = useState<FilterKey>('')
  const [points, setPoints] = useState<GrammarPointWithStatus[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')

  const [searchInput, setSearchInput] = useState(query)
  const [searchResult, setSearchResult] = useState<GrammarSearchResult | null>(null)
  const [searchLoading, setSearchLoading] = useState(false)
  const [searchError, setSearchError] = useState('')

  useEffect(() => {
    if (isSearching) return
    const controller = new AbortController()
    void loadPoints(level, controller.signal)
    return () => controller.abort()
  }, [level, isSearching])

  async function loadPoints(lv: JLPTLevel, signal: AbortSignal) {
    setLoading(true)
    setError('')
    try {
      const data = await apiFetch<GrammarPointWithStatus[]>('GET', `/api/v1/grammar?level=${lv}`, undefined, signal)
      if (!signal.aborted) setPoints(data ?? [])
    } catch (err) {
      if (!signal.aborted) setError(err instanceof Error ? err.message : 'Failed to load')
    } finally {
      if (!signal.aborted) setLoading(false)
    }
  }

  // A grammar annotation coming from translation practice is resolved against
  // the grammar library. An exact entry goes straight to its detail page;
  // otherwise related entries are listed so the user is never left at a dead end.
  useEffect(() => {
    if (!query) return
    const controller = new AbortController()
    setSearchInput(query)
    setSearchResult(null)
    setSearchError('')
    setSearchLoading(true)
    console.debug('[grammar] resolving annotation', query)

    async function run() {
      try {
        const data = await apiFetch<GrammarSearchResult>(
          'GET', grammarSearchApiPath(query), undefined, controller.signal,
        )
        if (controller.signal.aborted) return
        const result = data ?? null
        const target = exactMatchPath(result)
        if (target) {
          console.debug('[grammar] exact library entry found', query, target)
          navigate(target, { replace: true })
          return
        }
        console.debug('[grammar] no exact entry, showing related hits', query, result?.points.length ?? 0)
        setSearchResult(result)
      } catch (err) {
        if (controller.signal.aborted) return
        console.error('[grammar] annotation search failed', query, err)
        setSearchError(err instanceof Error ? err.message : '搜索语法失败')
      } finally {
        if (!controller.signal.aborted) setSearchLoading(false)
      }
    }

    void run()
    return () => controller.abort()
  }, [query, navigate])

  function submitSearch() {
    const next = searchInput.trim()
    const params = new URLSearchParams(searchParams)
    if (next) params.set('q', next)
    else params.delete('q')
    setSearchParams(params)
  }

  function clearSearch() {
    const params = new URLSearchParams(searchParams)
    params.delete('q')
    setSearchParams(params)
  }

  function changeLevel(next: JLPTLevel) {
    const params = new URLSearchParams(searchParams)
    params.set('level', next)
    setSearchParams(params)
  }

  const hits = searchResult?.points ?? []
  const filtered = filterPoints(points, filter)

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.title}>{isSearching ? '语法搜索' : t('grammar.list.title')}</h1>
        <DailyPracticeProgress progress={progress} />
      </div>

      {isSearching ? (
        <>
          <div className={styles.searchBar}>
            <input
              className={styles.searchInput}
              value={searchInput}
              onChange={e => setSearchInput(e.target.value)}
              onKeyDown={e => { if (e.key === 'Enter') submitSearch() }}
              placeholder="搜索语法点…"
              aria-label="搜索语法点"
            />
            <button className={styles.searchBtn} onClick={submitSearch}>搜索</button>
            <button className={styles.clearBtn} onClick={clearSearch}>清除</button>
          </div>
          <p className={styles.searchHint}>来自翻译练习的语法标注「{query}」</p>

          {searchError && <p className={styles.error} role="alert">{searchError}</p>}

          {searchLoading ? (
            <div style={{ display: 'flex', justifyContent: 'center', paddingTop: '60px' }}>
              <Spinner size="lg" />
            </div>
          ) : hits.length === 0 ? (
            <>
              <EmptyState icon="📝" title="语法库中暂无相关语法点" description="可以换个关键词，或浏览全部语法。" />
              <button className={styles.browseAllBtn} onClick={clearSearch}>浏览全部语法 →</button>
            </>
          ) : (
            <>
              <p className={styles.noExactHint}>
                语法库中暂无「{query}」的完整条目，以下是可能相关的语法点：
              </p>
              <div className={styles.list}>
                {hits.map(p => (
                  <button key={p.id} className={styles.item} onClick={() => navigate(`/grammar/${p.id}`)}>
                    <div className={styles.itemLeft}>
                      <div className={styles.itemName}>{p.name}</div>
                      <div className={styles.itemMeaning}>{p.meaning}</div>
                    </div>
                    <div className={styles.itemRight}>
                      <Badge level={p.jlpt_level} size="sm" />
                      <span className={styles.arrow}>›</span>
                    </div>
                  </button>
                ))}
              </div>
              <button className={styles.browseAllBtn} onClick={clearSearch}>浏览全部语法 →</button>
            </>
          )}
        </>
      ) : (
        <>
          <div className={styles.tabs}>
            {LEVELS.map((lv) => (
              <button
                key={lv}
                className={`${styles.tab} ${level === lv ? styles.tabActive : ''}`}
                onClick={() => changeLevel(lv)}
                aria-pressed={level === lv}
              >
                {lv}
              </button>
            ))}
          </div>

          <div className={styles.filterBar}>
            {FILTERS.map((f) => (
              <button
                key={f.key}
                className={`${styles.filterBtn} ${filter === f.key ? styles.filterBtnActive : ''}`}
                onClick={() => setFilter(f.key)}
              >
                {t(f.labelKey)}
              </button>
            ))}
          </div>

          {error && <p className={styles.error} role="alert">{error}</p>}

          {loading ? (
            <div style={{ display: 'flex', justifyContent: 'center', paddingTop: '60px' }}>
              <Spinner size="lg" />
            </div>
          ) : filtered.length === 0 ? (
            <EmptyState icon="📝" title={t('grammar.list.empty')} description="" />
          ) : (
            <div className={styles.list}>
              {filtered.map((p) => (
                <button key={p.id} className={styles.item} onClick={() => navigate(`/grammar/${p.id}`)}>
                  <div className={styles.itemLeft}>
                    <div className={styles.itemName}>{p.name}</div>
                    <div className={styles.itemMeaning}>{p.meaning}</div>
                  </div>
                  <div className={styles.itemRight}>
                    <StatusBadge status={p.user_status} />
                    {p.last_quiz_score >= 0 && p.last_quiz_score < 100 && (
                      <span className={styles.quizFailBadge} title={`Last quiz: ${p.last_quiz_score}/100`}>
                        {p.last_quiz_score}
                      </span>
                    )}
                    <Badge level={p.jlpt_level} size="sm" />
                    <span className={styles.arrow}>›</span>
                  </div>
                </button>
              ))}
            </div>
          )}
        </>
      )}
    </div>
  )
}
