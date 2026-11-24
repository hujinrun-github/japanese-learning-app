import type { ListeningAttempt, ListeningQuestion } from '@/types/listening'
import styles from './ListeningPage.module.css'

interface Props {
  questions: ListeningQuestion[]
  answers: Record<string, number>
  result?: ListeningAttempt
  disabled?: boolean
  onAnswer: (questionID: string, option: number) => void
  onEvidence?: (index: number) => void
}
export function ListeningQuestions({ questions, answers, result, disabled, onAnswer, onEvidence }: Props) {
  return <div className={styles.questions}>{questions.map((question, index) => {
    const feedback = result?.results.find((item) => item.question_id === question.id)
    const selected = feedback?.option_index ?? answers[question.id]
    return <fieldset key={question.id} className={styles.question}>
      <legend>{index + 1}. {question.prompt}</legend>
      <div className={styles.options}>{question.options.map((option, optionIndex) => <label key={optionIndex} className={selected === optionIndex ? styles.selectedOption : styles.option}>
        <input type="radio" name={`question-${question.id}`} checked={selected === optionIndex} disabled={disabled || !!result} onChange={() => onAnswer(question.id, optionIndex)} />
        <span>{String.fromCharCode(65 + optionIndex)}. <span data-dictionary-text={result ? '' : undefined}>{option}</span></span>
      </label>)}</div>
      {feedback && <div className={feedback.correct ? styles.correct : styles.incorrect}>
        <strong>{feedback.correct ? '✅ 回答正确' : `正确答案：${question.options[feedback.correct_option_index]}`}</strong>
        <p data-dictionary-text>{feedback.explanation}</p>
        {onEvidence && feedback.evidence_sentence_index >= 0 && <button type="button" className={styles.textLink} onClick={() => onEvidence(feedback.evidence_sentence_index)}>🎧 回听第 {feedback.evidence_sentence_index + 1} 句</button>}
      </div>}
    </fieldset>
  })}</div>
}
