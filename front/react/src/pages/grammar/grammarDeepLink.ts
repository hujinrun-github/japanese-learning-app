// 勘误历史页的「返回原题」链接形如 /grammar/12?quiz=1#quiz-34 或 /grammar/12#examples。
// 详情页只在数据加载完成后滚动，且只接受白名单内的锚点，避免任意 hash 影响页面。

export type GrammarDeepLink = { quizOpen: boolean; anchor: string | null }

const SAFE_ANCHOR = /^(examples|(?:example|quiz)-[1-9]\d*)$/

export function grammarDeepLink(search: string, hash: string): GrammarDeepLink {
  const quizOpen = new URLSearchParams(search).get('quiz') === '1'
  const anchor = hash.startsWith('#') ? hash.slice(1) : ''
  return { quizOpen, anchor: SAFE_ANCHOR.test(anchor) ? anchor : null }
}
