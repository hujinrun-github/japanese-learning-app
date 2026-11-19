import { useEffect, useState } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { apiFetch } from '@/api/client'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { EmptyState } from '@/components/ui/EmptyState'
import { Spinner } from '@/components/ui/Spinner'
import type { JLPTLevel, WritingTopic } from '@/types/api'
import {
  filterWritingTopics,
  summarizeWritingTopics,
  writingPracticePath,
  type PracticeStatusFilter,
  type TopicLevelFilter,
} from './writingTopicList'
import styles from './WritingTopicListPage.module.css'

const levels: JLPTLevel[] = ['N5', 'N4', 'N3', 'N2', 'N1']

export interface WritingTopicListViewProps {
  topics: WritingTopic[]
  loading: boolean
  error: string
  level: TopicLevelFilter
  status: PracticeStatusFilter
  onLevelChange: (level: TopicLevelFilter) => void
  onStatusChange: (status: PracticeStatusFilter) => void
  onRetry: () => void
}

export function WritingTopicCard({ topic }: { topic: WritingTopic }) {
  const { t } = useTranslation()
  const status = topic.practiced ? 'practiced' : 'unpracticed'
  return (
    <Link className={styles.topicCard} to={writingPracticePath(topic.id)}>
      <div className={styles.cardHeader}>
        <span className={styles.cardTitle} lang="ja">{topic.title}</span>
        <Badge level={topic.jlpt_level} size="sm" />
      </div>
      <p className={styles.cardPrompt} lang="ja">{topic.essay_prompt}</p>
      <div className={styles.cardFooter}>
        <span
          className={topic.practiced ? styles.statusPracticed : styles.statusUnpracticed}
          data-status={status}
        >
          {topic.practiced ? '✅' : '📝'} {t(status === 'practiced' ? 'writing.practiced' : 'writing.unpracticed')}
        </span>
        <span className={styles.cardAction}>{t('writing.topicList.cardAction')}</span>
      </div>
    </Link>
  )
}

export function WritingTopicListView({
  topics,
  loading,
  error,
  level,
  status,
  onLevelChange,
  onStatusChange,
  onRetry,
}: WritingTopicListViewProps) {
  const { t } = useTranslation()
  const visible = filterWritingTopics(topics, level, status)
  const summary = summarizeWritingTopics(topics)

  return (
    <div className={styles.page}>
      <div className={styles.header}>
        <h1 className={styles.title}>{t('writing.topicList.title')}</h1>
        <p className={styles.intro}>{t('writing.topicList.intro')}</p>
        <Link className={styles.startLink} to={writingPracticePath()}>
          {t('writing.topicList.startPractice')}
        </Link>
        <div className={styles.filters}>
          <label className={styles.filter} htmlFor="writing-list-level">
            {t('writing.levelFilter')}
            <select
              id="writing-list-level"
              value={level}
              onChange={event => onLevelChange(event.target.value as TopicLevelFilter)}
            >
              <option value="all">{t('writing.allLevels')}</option>
              {levels.map(option => <option key={option} value={option}>{option}</option>)}
            </select>
          </label>
          <label className={styles.filter} htmlFor="writing-list-status">
            {t('writing.statusFilter')}
            <select
              id="writing-list-status"
              value={status}
              onChange={event => onStatusChange(event.target.value as PracticeStatusFilter)}
            >
              <option value="all">{t('writing.allTopics')}</option>
              <option value="unpracticed">{t('writing.unpracticed')}</option>
              <option value="practiced">{t('writing.practiced')}</option>
            </select>
          </label>
        </div>
        <p className={styles.summary}>
          {t('writing.topicList.summary', { total: summary.total, practiced: summary.practiced })}
        </p>
      </div>

      {loading ? (
        <div className={styles.center} role="status" aria-label={t('writing.topicList.loading')}>
          <Spinner size="lg" />
        </div>
      ) : error ? (
        <div className={styles.errorBox} role="alert">
          <p className={styles.error}>{error}</p>
          <Button variant="secondary" onClick={onRetry}>{t('writing.retry')}</Button>
        </div>
      ) : visible.length === 0 ? (
        <EmptyState icon="📝" title={t('writing.noMatchingTopics')} description={t('writing.noMatchingTopicsDesc')} />
      ) : (
        <div className={styles.topicList}>
          {visible.map(topic => <WritingTopicCard key={topic.id} topic={topic} />)}
        </div>
      )}
    </div>
  )
}

export function WritingTopicListPage() {
  const { t } = useTranslation()
  const loadErrorText = t('writing.loadError')
  const [topics, setTopics] = useState<WritingTopic[]>([])
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [level, setLevel] = useState<TopicLevelFilter>('all')
  const [status, setStatus] = useState<PracticeStatusFilter>('all')
  const [attempt, setAttempt] = useState(0)

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError('')
    apiFetch<WritingTopic[]>('GET', '/api/v1/writing/topics', undefined, controller.signal)
      .then(data => {
        if (!controller.signal.aborted) setTopics(data ?? [])
      })
      .catch((err: unknown) => {
        if (controller.signal.aborted) return
        console.error('WritingTopicListPage: load topics failed', err)
        setError(err instanceof Error ? err.message : loadErrorText)
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false)
      })
    return () => controller.abort()
  }, [attempt, loadErrorText])

  return (
    <WritingTopicListView
      topics={topics}
      loading={loading}
      error={error}
      level={level}
      status={status}
      onLevelChange={setLevel}
      onStatusChange={setStatus}
      onRetry={() => setAttempt(value => value + 1)}
    />
  )
}
