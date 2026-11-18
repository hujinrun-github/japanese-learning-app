// 与后端 internal/module/errata/rules.go 保持一致的分类规则：
// 处理状态决定能否操作，问题类别决定能改哪些字段、能否自行撤回。

export type ErrataModule = 'word' | 'grammar' | 'lesson' | 'speaking' | 'shadowing' | 'writing' | 'translation'
export type ErrataStatus = 'pending' | 'resolved'

export type ErrataReport = {
  id: number
  module: ErrataModule
  item_key: string
  item_label: string
  category: string
  description: string
  status: ErrataStatus
  created_at: string
}

export type EditDraft = {
  category: string
  itemLabel: string
  description: string
}

export const errataCategories = ['prompt', 'answer', 'explanation', 'audio', 'other'] as const

// 文本类问题（题面、答案、解析）允许修正题面摘要；
// 音视频与其他类的定位信息由客户端采集，用户只能补充说明。
export function allowsLabelEdit(category: string): boolean {
  return category === 'prompt' || category === 'answer' || category === 'explanation'
}

// 只有待处理的报告可以修改。
export function canEdit(report: ErrataReport): boolean {
  return report.status === 'pending'
}

// 音视频类勘误会触发媒体资源核查，交给管理员跟进，提交者不能自行撤回。
export function canDelete(report: ErrataReport): boolean {
  return report.status === 'pending' && report.category !== 'audio'
}

// 只提交允许修改且确实变化的字段；切换到新类别时按新类别的规则过滤。
export function buildEditPayload(draft: EditDraft, report: ErrataReport): Record<string, string> {
  const payload: Record<string, string> = {}
  const category = draft.category.trim()
  const description = draft.description.trim()
  const itemLabel = draft.itemLabel.trim()

  if (category && category !== report.category) {
    payload.category = category
  }
  if (description && description !== report.description) {
    payload.description = description
  }
  if (itemLabel && allowsLabelEdit(category) && itemLabel !== report.item_label) {
    payload.item_label = itemLabel
  }
  return payload
}
