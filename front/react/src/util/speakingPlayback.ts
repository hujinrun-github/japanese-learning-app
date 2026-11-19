export interface SpeakingSentence {
  text: string
  /** Offsets in Unicode codepoints of the original material text. */
  start: number
  end: number
}

export function splitSpeakingSentences(text: string): SpeakingSentence[] {
  const chars = Array.from(text)
  const sentences: SpeakingSentence[] = []
  let start = 0
  let depth = 0
  function append(end: number) {
    while (start < end && /\s/u.test(chars[start])) start++
    while (end > start && /\s/u.test(chars[end - 1])) end--
    if (end > start) sentences.push({ text: chars.slice(start, end).join(''), start, end })
  }
  for (let i = 0; i < chars.length; i++) {
    const char = chars[i]
    if (char === '\r' || char === '\n') {
      append(i)
      start = i + 1
      depth = 0
      continue
    }
    if ('「『（('.includes(char)) depth++
    if ('」』）)'.includes(char)) depth = Math.max(0, depth - 1)
    if (depth === 0 && /[。！？!?]/u.test(char) && !/[。！？!?]/u.test(chars[i + 1] ?? '')) {
      append(i + 1)
      start = i + 1
    }
  }
  append(chars.length)
  return sentences
}

export interface SentenceCue {
  index: number
  start: number
  end: number
}

/** Estimated only: reference materials currently have no aligned timestamps. */
export function estimateSentenceCues(sentences: string[], duration: number): SentenceCue[] {
  if (!Number.isFinite(duration) || duration <= 0 || sentences.length === 0) return []
  const weights = sentences.map(sentence => {
    const spoken = sentence.replace(/^\s*\d+[.．、]\s*/u, '').replace(/^[^：:\n]{1,16}[：:]/u, '')
    return Math.max(1, Array.from(spoken.replace(/[^\p{L}\p{N}]/gu, '')).length)
  })
  const total = weights.reduce((sum, weight) => sum + weight, 0)
  let elapsedWeight = 0
  return weights.map((weight, index) => {
    const start = duration * elapsedWeight / total
    elapsedWeight += weight
    return { index, start, end: duration * elapsedWeight / total }
  })
}

export function sentenceAtTime(cues: SentenceCue[], time: number): number | null {
  return cues.find(cue => time >= cue.start && time < cue.end)?.index ?? null
}
