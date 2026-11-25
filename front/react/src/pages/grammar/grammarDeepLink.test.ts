import { describe, expect, it } from 'vitest'
import { grammarDeepLink } from './grammarDeepLink'

// 勘误历史页的「返回原题」链接会带 ?quiz=1 与 #examples / #quiz-{id}，
// 详情页据此展开测验并滚动到被反馈的内容，方便核对是否真的修复。
describe('grammarDeepLink 解析勘误直达链接', () => {
  it.each([
    { name: '例句 / 音频报告直达例句区', search: '', hash: '#examples', quizOpen: false, anchor: 'examples' },
    { name: '例句报告定位到具体例句', search: '', hash: '#example-34', quizOpen: false, anchor: 'example-34' },
    { name: '测验题报告展开测验并定位题目', search: '?quiz=1', hash: '#quiz-34', quizOpen: true, anchor: 'quiz-34' },
    { name: '只展开测验不定位', search: '?quiz=1', hash: '', quizOpen: true, anchor: null },
    { name: '普通访问不展开不定位', search: '', hash: '', quizOpen: false, anchor: null },
    { name: 'quiz=0 不展开', search: '?quiz=0', hash: '#examples', quizOpen: false, anchor: 'examples' },
    { name: '带其它查询参数仍可展开', search: '?from=errata&quiz=1', hash: '#quiz-7', quizOpen: true, anchor: 'quiz-7' },
    { name: '非法题目锚点被拒绝', search: '?quiz=1', hash: '#quiz-abc', quizOpen: true, anchor: null },
    { name: '路径穿越式锚点被拒绝', search: '', hash: '#../admin', quizOpen: false, anchor: null },
    { name: '无 # 前缀的锚点被拒绝', search: '', hash: 'examples', quizOpen: false, anchor: null },
    { name: '空题库题号被拒绝', search: '?quiz=1', hash: '#quiz-0', quizOpen: true, anchor: null },
  ])('$name', ({ search, hash, quizOpen, anchor }) => {
    expect(grammarDeepLink(search, hash)).toEqual({ quizOpen, anchor })
  })
})
