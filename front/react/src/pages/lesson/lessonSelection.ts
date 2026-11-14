import type { Sentence } from '@/types/api'

export interface TextRange {
  start: number
  end: number
}

export interface LessonSelection extends TextRange {
  quote: string
}

export function indexLessonText(sentences: Sentence[]) {
  let offset = 0
  return sentences.map((sentence) => ({
    ...sentence,
    tokens: sentence.tokens.map((token) => {
      const start = offset
      offset += Array.from(token.surface).length
      return { ...token, start }
    }),
  }))
}

export function splitHighlights<T extends TextRange>(surface: string, start: number, ranges: T[]) {
  const chars = Array.from(surface)
  const end = start + chars.length
  const overlaps = ranges.filter((range) => range.start < end && range.end > start)
  const boundaries = new Set([start, end])
  for (const range of overlaps) {
    boundaries.add(Math.max(start, range.start))
    boundaries.add(Math.min(end, range.end))
  }
  const sorted = [...boundaries].sort((a, b) => a - b)
  return sorted.slice(0, -1).map((offset, index) => ({
    start: offset,
    text: chars.slice(offset - start, sorted[index + 1] - start).join(''),
    highlights: overlaps.filter((range) => range.start <= offset && offset < range.end),
  }))
}

export function getLessonSelection(root: HTMLElement, selection: Selection | null, chars: string[]): LessonSelection | null {
  if (!selection || selection.isCollapsed || selection.rangeCount !== 1) return null
  const range = selection.getRangeAt(0)
  if (!root.contains(range.commonAncestorContainer)) return null

  for (const node of [range.startContainer, range.endContainer]) {
    const element = node.nodeType === Node.ELEMENT_NODE ? node as Element : node.parentElement
    if (!element?.closest('[lang="ja"]')) return null
  }

  let start = Infinity
  let end = 0
  for (const base of root.querySelectorAll<HTMLElement>('[data-lesson-offset]')) {
    if (!range.intersectsNode(base)) continue
    let offset = Number(base.dataset.lessonOffset)
    const walker = document.createTreeWalker(base, NodeFilter.SHOW_TEXT)
    while (walker.nextNode()) {
      const node = walker.currentNode as Text
      if (range.intersectsNode(node)) {
        const from = node === range.startContainer ? range.startOffset : 0
        const to = node === range.endContainer ? range.endOffset : node.length
        const selected = Array.from(node.data.slice(from, to)).length
        if (selected) {
          const first = offset + Array.from(node.data.slice(0, from)).length
          start = Math.min(start, first)
          end = Math.max(end, first + selected)
        }
      }
      offset += Array.from(node.data).length
    }
  }

  if (start >= end || end > chars.length) return null
  const quote = chars.slice(start, end).join('')
  return quote.trim() ? { start, end, quote } : null
}

export function sameTextRange(a: TextRange, b: TextRange) {
  return a.start === b.start && a.end === b.end
}
