import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { ListeningAttempt } from '@/types/listening'
import { ListeningQuestions } from './ListeningQuestions'

const questions = [{ id: 'q1', type: 'main_idea', prompt: '何を買いますか。', options: ['本', '新聞'] }]
const attempt: ListeningAttempt = {
  id: 1, phase: 'blind', correct_count: 0, total: 1, assisted: false, created_at: '',
  results: [{ question_id: 'q1', option_index: 0, correct_option_index: 1, correct: false, explanation: '新聞を買うと言っています。', evidence_sentence_index: 1 }],
}

describe('listening question feedback', () => {
  it('does not reveal answers or explanation before submission', () => {
    const html = renderToStaticMarkup(<ListeningQuestions questions={questions} answers={{ q1: 0 }} onAnswer={() => undefined} />)
    expect(html).not.toContain('正确答案')
    expect(html).not.toContain(attempt.results[0].explanation)
    expect(html).toContain('checked=""')
    expect(html).not.toContain('disabled=""')
  })

  it('locks submitted options while keeping the evidence replay button operable', () => {
    const html = renderToStaticMarkup(<ListeningQuestions questions={questions} answers={{}} result={attempt} onAnswer={() => undefined} onEvidence={() => undefined} />)
    expect(html).toContain('正确答案：新聞')
    expect(html).toContain(attempt.results[0].explanation)
    expect(html.match(/disabled=""/g)).toHaveLength(2)
    expect(html).not.toMatch(/<fieldset[^>]*disabled/)
    expect(html).toMatch(/<button type="button" class="[^"]*">🎧 回听第 2 句<\/button>/)
  })
})
