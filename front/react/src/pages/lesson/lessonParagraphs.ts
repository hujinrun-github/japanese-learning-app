import type { FuriganaToken, Sentence } from '@/types/api'

export interface ArticleParagraph<T = FuriganaToken> {
  tokens: T[]
  chinese: string
}

export interface SentenceParagraph<S extends Sentence = Sentence> {
  sentences: S[]
  chinese: string
}

/**
 * Group a flat lesson sentence list into article paragraphs.
 *
 * Consecutive sentences sharing the same `paragraph` number form one running
 * paragraph; a change in the number starts a new one. Legacy lessons without
 * the marker all fall back to paragraph 0, i.e. one single paragraph.
 *
 * This grouping is deliberately generic: callers may pass sentences that have
 * already been indexed by `indexLessonText`, and every extra token field (in
 * particular the global `start` offset) is preserved. Annotation ranges keep
 * using one continuous offset space, so a paragraph break must never reset
 * offsets back to zero.
 */
export function groupSentencesByParagraph<S extends Sentence>(sentences: S[]): SentenceParagraph<S>[] {
  if (sentences.length === 0) return []

  const paragraphs: SentenceParagraph<S>[] = []
  let current = sentences[0].paragraph ?? 0
  let group: S[] = []
  let chinese = ''
  let tokenCount = 0

  const flush = () => {
    if (tokenCount === 0 && chinese === '') return
    paragraphs.push({ sentences: group, chinese })
  }

  for (const sentence of sentences) {
    const paragraph = sentence.paragraph ?? 0
    if (paragraph !== current) {
      flush()
      group = []
      chinese = ''
      tokenCount = 0
      current = paragraph
    }
    group.push(sentence)
    tokenCount += sentence.tokens.length
    if (sentence.chinese !== '') chinese += sentence.chinese
  }
  flush()

  return paragraphs
}

/**
 * Flatten each paragraph into one continuous run of tokens so the article
 * reads as prose instead of one block per sentence.
 */
export function toArticleParagraphs<S extends Sentence>(sentences: S[]): ArticleParagraph<S['tokens'][number]>[] {
  return groupSentencesByParagraph(sentences).map((group) => ({
    tokens: group.sentences.flatMap((sentence) => sentence.tokens),
    chinese: group.chinese,
  }))
}
