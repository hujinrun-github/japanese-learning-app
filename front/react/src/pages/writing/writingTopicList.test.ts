import { describe, expect, it } from 'vitest'
import type { WritingTopic } from '@/types/api'
import type { PracticeStatusFilter, TopicLevelFilter } from './writingTopicList'
import {
  filterWritingTopics,
  findWritingTopicIndex,
  summarizeWritingTopics,
  writingPracticePath,
} from './writingTopicList'

const topics: WritingTopic[] = [
  { id: '1', title: '自己紹介', jlpt_level: 'N5', essay_prompt: '自己紹介を書いてください', sentence_prompt: '〜ですを使う', practiced: false },
  { id: '2', title: '趣味', jlpt_level: 'N4', essay_prompt: '趣味について', sentence_prompt: '〜が好き', practiced: true },
  { id: '3', title: '旅行', jlpt_level: 'N5', essay_prompt: '旅行の思い出', sentence_prompt: '〜に行きたい', practiced: true },
]

describe('filterWritingTopics 按级别与练习状态筛选主题', () => {
  const cases: { name: string; level: TopicLevelFilter; status: PracticeStatusFilter; want: string[] }[] = [
    { name: '全部级别 + 全部状态返回全部主题', level: 'all', status: 'all', want: ['1', '2', '3'] },
    { name: '按 N5 筛选', level: 'N5', status: 'all', want: ['1', '3'] },
    { name: '按 N1 筛选无结果', level: 'N1', status: 'all', want: [] },
    { name: '只看未练习', level: 'all', status: 'unpracticed', want: ['1'] },
    { name: '只看已练习', level: 'all', status: 'practiced', want: ['2', '3'] },
    { name: '级别与状态组合筛选', level: 'N5', status: 'practiced', want: ['3'] },
  ]

  it.each(cases)('$name', ({ level, status, want }) => {
    expect(filterWritingTopics(topics, level, status).map(topic => topic.id)).toEqual(want)
  })
})

describe('summarizeWritingTopics 统计练习进度', () => {
  it.each([
    { name: '空列表', input: [], total: 0, practiced: 0, unpracticed: 0 },
    { name: '混合状态', input: topics, total: 3, practiced: 2, unpracticed: 1 },
    { name: '全部已练习', input: topics.map(topic => ({ ...topic, practiced: true })), total: 3, practiced: 3, unpracticed: 0 },
  ])('$name', ({ input, total, practiced, unpracticed }) => {
    expect(summarizeWritingTopics(input)).toEqual({ total, practiced, unpracticed })
  })
})

describe('writingPracticePath 生成练习页地址', () => {
  it.each([
    { name: '不指定主题时进入默认练习页', topicID: undefined, want: '/writing/practice' },
    { name: '指定主题时带上 topic_id', topicID: '12', want: '/writing/practice?topic_id=12' },
    { name: '主题 ID 需要 URL 编码', topicID: 'a b&c', want: '/writing/practice?topic_id=a%20b%26c' },
  ])('$name', ({ topicID, want }) => {
    expect(writingPracticePath(topicID)).toBe(want)
  })
})

describe('findWritingTopicIndex 定位指定主题', () => {
  it.each([
    { name: '没有 topic_id 时不覆盖默认定位', topicID: null, want: -1 },
    { name: '主题存在时返回下标', topicID: '2', want: 1 },
    { name: '主题不存在时返回 -1', topicID: '99', want: -1 },
  ])('$name', ({ topicID, want }) => {
    expect(findWritingTopicIndex(topics, topicID)).toBe(want)
  })
})
