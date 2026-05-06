#!/usr/bin/env node
/**
 * Pre-compute furigana (<ruby>) HTML for example sentences in seed JSON files.
 * Uses kuromoji (works in Node.js, not browser) to tokenize and generate readings.
 *
 * Usage: node generate-furigana.mjs <path-to-words.json>
 * Output: updated JSON written back to the same file.
 */
import { readFileSync, writeFileSync } from 'fs'
import kuromoji from 'kuromoji'

const KATAKANA_START = 0x30A1
const HIRAGANA_START = 0x3041
const KATAKANA_END = 0x30F6
const KANJI_RE = /[\u4E00-\u9FFF\u3400-\u4DBF]/

function katakanaToHiragana(s) {
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

function hasKanji(s) {
  return KANJI_RE.test(s)
}

async function main() {
  const filePath = process.argv[2]
  if (!filePath) {
    console.error('Usage: node generate-furigana.mjs <path-to-words.json>')
    process.exit(1)
  }

  const raw = readFileSync(filePath, 'utf-8')
  const words = JSON.parse(raw)

  const tokenizer = await new Promise((resolve, reject) => {
    kuromoji.builder({ dicPath: 'node_modules/kuromoji/dict/' }).build((err, t) => {
      if (err) return reject(err)
      resolve(t)
    })
  })

  let count = 0
  for (const word of words) {
    if (!word.examples) continue
    for (const ex of word.examples) {
      if (ex.furigana_html) continue // already generated
      const tokens = tokenizer.tokenize(ex.japanese)
      let html = ''
      for (const t of tokens) {
        const reading = katakanaToHiragana(t.reading ?? '')
        if (hasKanji(t.surface_form)) {
          html += `<ruby>${t.surface_form}<rt>${reading}</rt></ruby>`
        } else {
          html += t.surface_form
        }
      }
      ex.furigana_html = html
      count++
    }
  }

  writeFileSync(filePath, JSON.stringify(words, null, 2), 'utf-8')
  console.log(`Done. Generated furigana for ${count} examples in ${filePath}`)
}

main().catch((e) => {
  console.error(e)
  process.exit(1)
})
