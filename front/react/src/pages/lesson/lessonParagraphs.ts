import type { FuriganaToken, Sentence } from '@/types/api'

export interface ArticleParagraph {
  tokens: FuriganaToken[]
  chinese: string
}

/**
 * Group a flat lesson sentence list into article paragraphs.
 *
 * Consecutive sentences sharing the same `paragraph` number form one running
 * paragraph; a change in the number starts a new one. Sentences inside a
 * paragraph are joined without separators so the article reads as prose.
 * Legacy lessons without the marker all fall back to paragraph 0, i.e. the
 * whole article stays a single paragraph.
 */
export function toArticleParagraphs(sentences: Sentence[]): ArticleParagraph[] {
  if (sentences.length === 0) return []

  const paragraphs: ArticleParagraph[] = []
  let current = sentences[0].paragraph ?? 0
  let tokens: FuriganaToken[] = []
  let chinese = ''

  const flush = () => {
    if (tokens.length === 0 && chinese === '') return
    paragraphs.push({ tokens, chinese })
  }

  for (const sentence of sentences) {
    const paragraph = sentence.paragraph ?? 0
    if (paragraph !== current) {
      flush()
      tokens = []
      chinese = ''
      current = paragraph
    }
    tokens.push(...sentence.tokens)
    if (sentence.chinese !== '') chinese += sentence.chinese
  }
  flush()

  return paragraphs
}
