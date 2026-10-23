export interface DictationWord {
  kanji_form: string
  reading: string
}

export interface DictationAnswerResult {
  requiresKanji: boolean
  spellingCorrect: boolean
  readingCorrect: boolean
  allCorrect: boolean
}

const HAN_CHARACTER = /\p{Script=Han}/u

export function normalizeDictationAnswer(value: string): string {
  return value.normalize('NFKC').trim()
}

export function wordRequiresKanji(spelling: string): boolean {
  return HAN_CHARACTER.test(spelling)
}

export function evaluateDictationAnswer(
  word: DictationWord,
  spellingAnswer: string,
  readingAnswer: string,
): DictationAnswerResult {
  const requiresKanji = wordRequiresKanji(word.kanji_form)
  const spellingCorrect = normalizeDictationAnswer(spellingAnswer) === normalizeDictationAnswer(word.kanji_form)
  const readingCorrect = requiresKanji
    ? normalizeDictationAnswer(readingAnswer) === normalizeDictationAnswer(word.reading)
    : true

  return {
    requiresKanji,
    spellingCorrect,
    readingCorrect,
    allCorrect: spellingCorrect && readingCorrect,
  }
}
