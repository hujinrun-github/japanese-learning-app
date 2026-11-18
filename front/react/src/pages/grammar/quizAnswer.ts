import type { QuizType } from '@/types/api'

export interface AnswerParts {
  before: string
  after: string
}

// 填空题把答案嵌回题干，展示完整句子；题干无法安全还原时只展示答案本身。
export function answerPartsFor(type: QuizType, prompt: string): AnswerParts {
  if (type !== 'fill_blank') {
    return { before: '', after: '' }
  }
  const parts = prompt.split(/[＿_]{2,}/)
  if (parts.length !== 2) {
    return { before: '', after: '' }
  }
  return { before: parts[0], after: parts[1] }
}

// 线上题库已把完整答案回填进解析时，前端无需再重复渲染「正确答案」块。
export function hasFullAnswer(explanation: string): boolean {
  return explanation.startsWith('正确答案：')
}
