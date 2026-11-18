import { describe, expect, it } from 'vitest'
import {
  allowsLabelEdit,
  buildEditPayload,
  canDelete,
  canEdit,
  type EditDraft,
  type ErrataReport,
} from './errataEditRules'

const base: ErrataReport = {
  id: 1,
  module: 'grammar',
  item_key: '3:quiz:12',
  item_label: '原文摘要',
  category: 'prompt',
  description: '原始描述',
  status: 'pending',
  created_at: '2026-11-18T00:00:00Z',
}

const draft = (overrides: Partial<EditDraft> = {}): EditDraft => ({
  category: 'prompt',
  itemLabel: '原文摘要',
  description: '原始描述',
  ...overrides,
})

describe('按问题类别区分可编辑字段', () => {
  it.each([
    { category: 'prompt', expected: true },
    { category: 'answer', expected: true },
    { category: 'explanation', expected: true },
    { category: 'audio', expected: false },
    { category: 'other', expected: false },
    { category: 'unknown', expected: false },
  ])('$category 可修改题面摘要 = $expected', ({ category, expected }) => {
    expect(allowsLabelEdit(category)).toBe(expected)
  })
})

describe('按状态与类别区分可操作按钮', () => {
  it.each([
    { name: '待处理的文本类可编辑可删除', status: 'pending', category: 'prompt', edit: true, remove: true },
    { name: '待处理的答案类可编辑可删除', status: 'pending', category: 'answer', edit: true, remove: true },
    { name: '待处理的解析类可编辑可删除', status: 'pending', category: 'explanation', edit: true, remove: true },
    { name: '待处理的其他类可编辑可删除', status: 'pending', category: 'other', edit: true, remove: true },
    { name: '待处理的音视频类只可编辑', status: 'pending', category: 'audio', edit: true, remove: false },
    { name: '已处理的文本类不可操作', status: 'resolved', category: 'prompt', edit: false, remove: false },
    { name: '已处理的音视频类不可操作', status: 'resolved', category: 'audio', edit: false, remove: false },
  ])('$name', ({ status, category, edit, remove }) => {
    const report = { ...base, status, category } as ErrataReport
    expect(canEdit(report)).toBe(edit)
    expect(canDelete(report)).toBe(remove)
  })
})

describe('buildEditPayload 只提交允许且确实变化的字段', () => {
  it.each([
    { name: '没有任何修改时不提交', input: draft(), expected: {} },
    { name: '只改说明', input: draft({ description: '新的说明' }), expected: { description: '新的说明' } },
    {
      name: '改类别和说明',
      input: draft({ category: 'answer', description: '答案写错了' }),
      expected: { category: 'answer', description: '答案写错了' },
    },
    { name: '文本类可改题面摘要', input: draft({ itemLabel: '新的题面' }), expected: { item_label: '新的题面' } },
    {
      name: '切换到音视频类时丢弃题面摘要修改',
      input: draft({ category: 'audio', itemLabel: '新的题面', description: '音频不同步' }),
      expected: { category: 'audio', description: '音频不同步' },
    },
    { name: '音视频类不允许改题面摘要', input: draft({ category: 'other', itemLabel: '新的题面' }), expected: { category: 'other' } },
    { name: '去除首尾空白', input: draft({ description: '  修正说明  ', itemLabel: '  新题面  ' }), expected: { description: '修正说明', item_label: '新题面' } },
    { name: '空白说明不会被提交', input: draft({ description: '   ' }), expected: {} },
  ])('$name', ({ input, expected }) => {
    expect(buildEditPayload(input, base)).toEqual(expected)
  })

  it('音视频类原报告改说明时按新类别规则提交', () => {
    const audio = { ...base, category: 'audio' }
    expect(buildEditPayload(draft({ category: 'audio', description: '重新描述' }), audio)).toEqual({ description: '重新描述' })
  })
})
