import type { CreateMistakeInput, QuizItemResult, QuizQuestion, Word } from '@/types/api'

// The backend rejects fields longer than these limits, so trim before sending.
const MAX_TEXT = 500
const MAX_EXPLANATION = 1000

export interface DictationAnswer {
  spelling: string
  reading: string
  requiresKanji: boolean
}

function clip(value: string, max: number): string {
  const chars = Array.from(value)
  return chars.length > max ? chars.slice(0, max).join('') : value
}

function firstNonEmpty(...values: string[]): string {
  return values.map(value => value.trim()).find(value => value !== '') ?? ''
}

export function buildWordReadingMistake(word: Word, userAnswer: string): CreateMistakeInput {
  return {
    module: 'word',
    item_key: `word:${word.id}:reading`,
    prompt: clip(firstNonEmpty(word.kanji_form, word.reading, `word ${word.id}`), MAX_TEXT),
    user_answer: clip(userAnswer.trim(), MAX_TEXT),
    correct_answer: clip(word.reading, MAX_TEXT),
    explanation: clip(word.meaning, MAX_EXPLANATION),
    source_path: '/words/review',
  }
}

export function buildWordDictationMistake(word: Word, answer: DictationAnswer): CreateMistakeInput {
  const spelling = answer.spelling.trim()
  const reading = answer.reading.trim()
  const userAnswer = answer.requiresKanji ? `${spelling} / ${reading}` : spelling
  const correctAnswer = answer.requiresKanji ? `${word.kanji_form} / ${word.reading}` : word.kanji_form
  return {
    module: 'word',
    item_key: `word:${word.id}:dictation`,
    prompt: clip(firstNonEmpty(word.meaning, word.kanji_form, word.reading, `word ${word.id}`), MAX_TEXT),
    user_answer: clip(userAnswer, MAX_TEXT),
    correct_answer: clip(correctAnswer, MAX_TEXT),
    explanation: clip(`${word.kanji_form}（${word.reading}）`, MAX_EXPLANATION),
    source_path: '/words/review',
  }
}

export function buildGrammarMistakes(
  pointID: number,
  questions: QuizQuestion[],
  results: QuizItemResult[],
): CreateMistakeInput[] {
  const prompts = new Map(questions.map(question => [question.id, question.prompt]))
  return results
    .filter(result => !result.correct)
    .map(result => ({
      module: 'grammar' as const,
      item_key: `grammar:${pointID}:${result.question_id}`,
      prompt: clip(
        firstNonEmpty(prompts.get(result.question_id) ?? '', `Grammar point ${pointID}, question ${result.question_id}`),
        MAX_TEXT,
      ),
      user_answer: clip(result.user_answer ?? '', MAX_TEXT),
      correct_answer: clip(result.expected ?? '', MAX_TEXT),
      explanation: clip(result.explanation ?? '', MAX_EXPLANATION),
      source_path: `/grammar/${pointID}`,
    }))
}
