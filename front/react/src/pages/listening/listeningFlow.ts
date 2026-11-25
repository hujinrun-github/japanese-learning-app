import type { ListeningAttempt, ListeningAudioSource, ListeningMaterial, ListeningProgress, ListeningQuestion, ListeningSentence, ListeningStage, ListeningStatus } from '@/types/listening'

export function restoreExercise(progress?: ListeningProgress | null) {
  const answers: Record<'blind' | 'retest', Record<string, number>> = { blind: {}, retest: {} }
  if (progress?.draft_answers) {
    answers[progress.draft_answers.phase] = Object.fromEntries(progress.draft_answers.answers.map((answer) => [answer.question_id, answer.option_index]))
  }
  return {
    stage: (progress?.stage ?? 'blind') as ListeningStage,
    sentenceIndex: progress?.sentence_index ?? 0,
    positionMs: progress?.position_ms ?? 0,
    bookmarks: progress?.bookmarks ?? [],
    answers,
    assisted: { blind: progress?.draft_answers?.phase === 'blind' && progress.draft_answers.assisted === true, retest: progress?.draft_answers?.phase === 'retest' && progress.draft_answers.assisted === true },
  }
}
export function canSubmitAnswers(questions: ListeningQuestion[], answers: Record<string, number>) {
  return questions.length > 0 && questions.every((question) => Number.isInteger(answers[question.id]) && answers[question.id] >= 0 && answers[question.id] < question.options.length)
}
export function answerList(questions: ListeningQuestion[], answers: Record<string, number>) {
  return questions.filter((question) => Number.isInteger(answers[question.id])).map((question) => ({ question_id: question.id, option_index: answers[question.id] }))
}
export function sentenceBounds(sentence: ListeningSentence) {
  return sentence.start_ms >= 0 && sentence.end_ms > sentence.start_ms
    ? { start: sentence.start_ms / 1000, end: sentence.end_ms / 1000 }
    : null
}
export function getListeningFilters(search: URLSearchParams) {
  const level = search.get('level') ?? ''
  const status = search.get('status') ?? 'all'
  return {
    level: ['', 'N5', 'N4', 'N3', 'N2', 'N1'].includes(level) ? level : '',
    status: (['all', 'new', 'practiced', 'due'].includes(status) ? status : 'all') as ListeningStatus,
  }
}
export function listeningPath({ level, status }: ReturnType<typeof getListeningFilters>, id?: number) {
  const query = new URLSearchParams()
  if (level) query.set('level', level)
  if (status !== 'all') query.set('status', status)
  return `/listening${id === undefined ? '' : `/${id}`}${query.size ? `?${query}` : ''}`
}

export function nextUnpracticedMaterial(
  materials: Pick<ListeningMaterial, 'id' | 'status' | 'training_ready'>[],
  currentID: number,
) {
  // The catalogue is ordered by id. Wrap to the first remaining material
  // when the learner opened a later one out of sequence.
  const available = materials.filter(item => item.id !== currentID && item.status === 'new' && item.training_ready === true)
  return available.find(item => item.id > currentID) ?? available[0]
}

export function needsBlindBaseline(trainingReady: boolean, stage: ListeningStage, attempts: Pick<ListeningAttempt, 'phase'>[]) {
  return trainingReady && (stage === 'intensive' || stage === 'retest') && !attempts.some((attempt) => attempt.phase === 'blind')
}
export function getListeningStepAction(
  savedStage: ListeningStage,
  target: ListeningStage,
  trainingReady: boolean,
  hasAudio: boolean,
  attempts: Pick<ListeningAttempt, 'phase'>[],
): 'view' | 'advance' | 'locked' {
  if (target === savedStage || savedStage === 'completed') return 'view'
  const hasBlind = attempts.some(attempt => attempt.phase === 'blind')
  switch (target) {
    case 'blind':
      return needsBlindBaseline(trainingReady, savedStage, attempts) ? 'advance' : 'view'
    case 'intensive':
      if (savedStage === 'retest') return 'view'
      return hasAudio && (!trainingReady || hasBlind) ? 'advance' : 'locked'
    case 'retest':
      return savedStage === 'intensive' && trainingReady && hasAudio && hasBlind ? 'advance' : 'locked'
    case 'completed':
      return savedStage === 'retest' && trainingReady && hasBlind && attempts.some(attempt => attempt.phase === 'retest') ? 'view' : 'locked'
  }
}
export function shouldReportPlaybackError(error: unknown) {
  return !(typeof error === 'object' && error !== null && 'name' in error && error.name === 'AbortError')
}
export const STAGE_LABELS: Record<ListeningStage, string> = { blind: '盲听理解', intensive: '逐句精听', retest: '关文本复测', completed: '训练结果' }
export function audioSourceLabel(source?: ListeningAudioSource) {
  return source === 'synthetic' ? '合成音' : source === 'human' ? '真人录音' : '来源待确认'
}
export function formatDueDate(value?: string | null) {
  if (!value) return '完成后安排'
  return new Intl.DateTimeFormat('zh-CN', { month: 'long', day: 'numeric', timeZone: 'Asia/Shanghai' }).format(new Date(value))
}
