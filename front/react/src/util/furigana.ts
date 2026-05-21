import { tokenize } from 'kuromojin'
import type { FuriganaToken } from '@/types/api'

const KATAKANA_START = 0x30A1
const HIRAGANA_START = 0x3041
const KATAKANA_END = 0x30F6

function katakanaToHiragana(s: string): string {
  let result = ''
  for (const ch of s) {
    const code = ch.charCodeAt(0)
    if (code >= KATAKANA_START && code <= KATAKANA_END) {
      result += String.fromCharCode(code - KATAKANA_START + HIRAGANA_START)
    } else {
      result += ch
    }
  }
  return result
}

const KANJI_RE = /[\u4E00-\u9FFF\u3400-\u4DBF]/

function hasKanji(s: string): boolean {
  return KANJI_RE.test(s)
}

/**
 * Convert Japanese text to HTML with <ruby> furigana annotations.
 * Uses kuromojin for morphological analysis (browser-compatible).
 */
export async function toFuriganaHTML(text: string): Promise<string> {
  const tokens = await tokenize(text)
  let result = ''
  for (const t of tokens) {
    const reading = katakanaToHiragana(t.reading ?? '')
    if (hasKanji(t.surface_form)) {
      result += `<ruby>${t.surface_form}<rt>${reading}</rt></ruby>`
    } else {
      result += t.surface_form
    }
  }
  return result
}

/**
 * Tokenize Japanese text and return an array of FuriganaToken.
 * Readings are converted from katakana to hiragana.
 */
export async function toFuriganaTokens(text: string): Promise<FuriganaToken[]> {
  const tokens = await tokenize(text)
  return tokens.map(t => ({
    surface: t.surface_form,
    reading: katakanaToHiragana(t.reading ?? ''),
  }))
}

/**
 * Map a character index within the original text to the corresponding token index.
 * Returns the last token index if charIndex is beyond the total surface length.
 */
export function charIndexToTokenIndex(charIndex: number, tokens: FuriganaToken[]): number {
  let offset = 0
  for (let i = 0; i < tokens.length; i++) {
    offset += tokens[i].surface.length
    if (charIndex < offset) {
      return i
    }
  }
  return tokens.length - 1
}
