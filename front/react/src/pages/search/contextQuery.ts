export function validSearchKeyword(text: string) {
  return !!text && !text.includes('\0') && Array.from(text).length <= 100
}

export function selectionKeyword(text: string) {
  const keyword = text.replace(/\r?\n/g, '').trim().replace(/^[「『（(]+|[」』）)。！？、!?]+$/gu, '').trim()
  return validSearchKeyword(keyword) && /[\p{L}\p{N}]/u.test(keyword) ? keyword : ''
}

export function dictionaryTerms(tokens: { surface: string; base?: string }[]) {
  const terms = new Map<string, { surface: string; keyword: string }>()
  for (const { surface, base } of tokens) {
    const keyword = selectionKeyword(base && base !== '*' ? base : surface)
    if (keyword && !terms.has(surface)) terms.set(surface, { surface, keyword })
  }
  return [...terms.values()]
}

const textSelector = '[data-dictionary-text]'

function asElement(node: Node) {
  return node.nodeType === Node.ELEMENT_NODE ? node as Element : node.parentElement
}

export function getDictionarySelection(selection: Selection | null) {
  if (!selection || selection.isCollapsed || selection.rangeCount !== 1) return ''
  const range = selection.getRangeAt(0)
  const source = asElement(range.startContainer)?.closest(textSelector)
  if (!source || source !== asElement(range.endContainer)?.closest(textSelector)) return ''
  if (asElement(range.commonAncestorContainer)?.closest('rt, rp')) return ''

  const selected = range.cloneContents()
  selected.querySelectorAll('rt, rp').forEach((element) => element.remove())
  return selectionKeyword(selected.textContent ?? '')
}
