// 勘误报告只带 module + item_key 两个定位字段（形态见 internal/module/errata/rules.go）。
// 各练习页的路由与定位参数足以还原原题，因此历史报告无需后端新增字段也能「返回原题」。
// 返回值一律是站内相对路径，避免报告里的定位键把用户带到站外。

export type ErrataSourceTarget = { module: string; item_key: string }

const SAFE_PART = /^[A-Za-z0-9_-]+$/
const NUMERIC_ID = /^[1-9]\d*$/
const MAX_KEY_LENGTH = 80

export function errataSourcePath(target: ErrataSourceTarget): string | null {
  const key = target.item_key
  if (!key || key.length > MAX_KEY_LENGTH) return null
  const parts = key.split(':')
  if (parts.some(part => !SAFE_PART.test(part))) return null
  const [root] = parts

  switch (target.module) {
    case 'grammar': {
      if (!NUMERIC_ID.test(root)) return null
      if (parts.length === 3 && parts[1] === 'quiz' && NUMERIC_ID.test(parts[2])) {
        return `/grammar/${root}?quiz=1#quiz-${parts[2]}`
      }
      if (parts.length === 3 && parts[1] === 'example' && NUMERIC_ID.test(parts[2])) {
        return `/grammar/${root}#example-${parts[2]}`
      }
      return `/grammar/${root}#examples`
    }
    case 'word':
      if (!NUMERIC_ID.test(root)) return null
      if (parts.length === 3 && parts[1] === 'example' && NUMERIC_ID.test(parts[2])) {
        return `/search/words/${root}#example-${parts[2]}`
      }
      return `/words/review?word_id=${root}${parts[1] === 'dictation' ? '&mode=dictation' : ''}`
    case 'lesson':
      if (!NUMERIC_ID.test(root)) return null
      if (parts.length === 3 && parts[1] === 'sentence' && (parts[2] === '0' || NUMERIC_ID.test(parts[2]))) {
        return `/lesson/${root}#sentence-${parts[2]}`
      }
      return `/lesson/${root}`
    case 'shadowing':
      return NUMERIC_ID.test(root) ? `/lesson/${root}/shadowing` : null
    case 'writing':
      return `/writing/practice?topic_id=${encodeURIComponent(root)}`
    case 'speaking':
      return NUMERIC_ID.test(root) ? '/speaking' : null
    case 'translation':
      return NUMERIC_ID.test(root) ? '/translation/practice' : null
    default:
      return null
  }
}
