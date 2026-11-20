import type { JLPTLevel, WritingTopic } from '@/types/api'

export type TopicLevelFilter = JLPTLevel | 'all'
export type PracticeStatusFilter = 'unpracticed' | 'practiced' | 'all'

export function filterWritingTopics(
  topics: WritingTopic[],
  level: TopicLevelFilter,
  status: PracticeStatusFilter,
): WritingTopic[] {
  return topics.filter(topic => (level === 'all' || topic.jlpt_level === level)
    && (status === 'all' || Boolean(topic.practiced) === (status === 'practiced')))
}

export function summarizeWritingTopics(topics: WritingTopic[]) {
  const total = topics.length
  const practiced = topics.filter(topic => topic.practiced).length
  return { total, practiced, unpracticed: total - practiced }
}

export function writingPracticePath(topicID?: string): string {
  return topicID
    ? `/writing/practice?topic_id=${encodeURIComponent(topicID)}`
    : '/writing/practice'
}

export function findWritingTopicIndex(topics: WritingTopic[], topicID: string | null): number {
  if (!topicID) return -1
  return topics.findIndex(topic => topic.id === topicID)
}
