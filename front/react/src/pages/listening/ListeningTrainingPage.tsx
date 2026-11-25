import { useEffect, useRef, useState } from 'react'
import { Link, useNavigate, useParams, useSearchParams } from 'react-router-dom'
import { completeListening, getListeningMaterials, getListeningSession, saveListeningProgress, submitListeningAttempt } from '@/api/listening'
import { Badge } from '@/components/ui/Badge'
import { Button } from '@/components/ui/Button'
import { Spinner } from '@/components/ui/Spinner'
import { DictionaryButton } from '@/pages/search/ContextualSearch'
import type { ListeningAttempt, ListeningPhase, ListeningSession, ListeningStage, SaveListeningProgress } from '@/types/listening'
import { answerList, audioSourceLabel, canSubmitAnswers, formatDueDate, getListeningFilters, getListeningStepAction, listeningPath, needsBlindBaseline, nextUnpracticedMaterial, restoreExercise, sentenceBounds, shouldReportPlaybackError, STAGE_LABELS } from './listeningFlow'
import { ListeningQuestions } from './ListeningQuestions'
import styles from './ListeningPage.module.css'

type Exercise = ReturnType<typeof restoreExercise>
const stages: ListeningStage[] = ['blind', 'intensive', 'retest', 'completed']

export function ListeningTrainingPage() {
  const { id } = useParams()
  const [search] = useSearchParams()
  const filters = getListeningFilters(search)
  return <ListeningTraining key={listeningPath(filters, Number(id))} materialID={Number(id)} filters={filters} />
}

function ListeningTraining({ materialID, filters }: { materialID: number; filters: ReturnType<typeof getListeningFilters> }) {
  const navigate = useNavigate()
  const backPath = listeningPath(filters)
  const [session, setSession] = useState<ListeningSession | null>(null)
  const [exercise, setExercise] = useState<Exercise>(() => restoreExercise())
  // A review view has its own player and selected sentence.
  const [review, setReview] = useState<{ stage: ListeningStage; sentenceIndex: number } | null>(null)
  const stateRef = useRef(exercise)
  const mediaRef = useRef<HTMLAudioElement>(null)
  const positionRef = useRef(0)
  const lastSavedPosition = useRef(0)
  const rangeRef = useRef<{ start: number; end: number } | null>(null)
  const loopRef = useRef(false)
  const saveChain = useRef<Promise<unknown>>(Promise.resolve())
  const saveVersion = useRef(0)
  const mutatingRef = useRef(false)
  const nextRequest = useRef<AbortController | null>(null)
  const keys = useRef<Record<string, string>>({})
  const [retry, setRetry] = useState(0)
  const [loading, setLoading] = useState(true)
  const [error, setError] = useState('')
  const [mediaError, setMediaError] = useState('')
  const [busy, setBusy] = useState(false)
  const [nextStatus, setNextStatus] = useState<'idle' | 'loading' | 'empty' | 'error'>('idle')
  const [saveStatus, setSaveStatus] = useState<'saved' | 'saving' | 'failed'>('saved')
  const [showJapanese, setShowJapanese] = useState(false)
  const [showChinese, setShowChinese] = useState(false)
  const [rate, setRate] = useState(1)
  const [looping, setLooping] = useState(false)

  useEffect(() => {
    const controller = new AbortController()
    setLoading(true)
    setError('')
    if (!Number.isInteger(materialID) || materialID <= 0) {
      setError('听力素材编号无效')
      setLoading(false)
      return
    }
    getListeningSession(materialID, controller.signal).then((data) => {
      if (controller.signal.aborted) return
      const restored = restoreExercise(data.progress)
      setSession(data)
      publish(restored)
      setReview(null)
      positionRef.current = restored.positionMs
      lastSavedPosition.current = restored.positionMs
      setShowJapanese(false)
      setShowChinese(false)
      setMediaError('')
      setSaveStatus('saved')
      setRate(1)
      stopSentenceLoop()
      keys.current = {}
    }).catch((err: unknown) => {
      if (!controller.signal.aborted) setError(message(err, '听力训练加载失败'))
    }).finally(() => { if (!controller.signal.aborted) setLoading(false) })
    return () => controller.abort()
  }, [materialID, retry])

  useEffect(() => () => {
    nextRequest.current?.abort()
  }, [])

  function publish(next: Exercise) {
    stateRef.current = next
    setExercise(next)
  }

  function snapshot(next: Exercise, restart: boolean, positionMs: number): SaveListeningProgress {
    const phase = next.stage === 'blind' || next.stage === 'retest' ? next.stage : null
    const submitted = phase && session?.attempts.some((attempt) => attempt.phase === phase)
    const maxPosition = Math.max(0, ...(session?.lesson.sentences.map((sentence) => sentence.end_ms) ?? []))
    return {
      expected_cycle: session?.progress.cycle ?? 1,
      ...(restart ? { restart: true } : {}),
      stage: next.stage,
      sentence_index: next.sentenceIndex,
      position_ms: Math.min(maxPosition, Math.max(0, Math.round(positionMs))),
      bookmarks: next.bookmarks,
      draft_answers: phase && !submitted ? { phase, answers: answerList(session?.questions ?? [], next.answers[phase]), assisted: next.assisted[phase] } : null,
    }
  }

  function persist(next = stateRef.current, restart = false, positionMs = positionRef.current) {
    const payload = snapshot(next, restart, positionMs)
    const version = ++saveVersion.current
    setSaveStatus('saving')
    // Serialize writes so an older playback checkpoint cannot overwrite a later answer or stage.
    const task = saveChain.current.catch(() => undefined).then(() => saveListeningProgress(materialID, payload))
    saveChain.current = task
    void task.then(() => {
      if (version === saveVersion.current) setSaveStatus('saved')
    }, () => {
      if (version === saveVersion.current) setSaveStatus('failed')
    })
    return task
  }

  function stopSentenceLoop() {
    rangeRef.current = null
    loopRef.current = false
    setLooping(false)
  }

  function resetPlayer() {
    const media = mediaRef.current
    if (media) {
      media.pause()
      media.playbackRate = 1
    }
    stopSentenceLoop()
    setShowJapanese(false)
    setShowChinese(false)
    setRate(1)
  }

  async function changeStage(stage: ListeningStage) {
    if (mutatingRef.current) return
    mutatingRef.current = true
    setBusy(true)
    setError('')
    resetPlayer()
    const next = { ...stateRef.current, stage }
    try {
      const progress = await persist(next, false, 0)
      positionRef.current = lastSavedPosition.current = progress.position_ms
      setReview(null)
      publish(next)
      setSession((current) => current ? { ...current, progress } : current)
    } catch (err) { setError(message(err, '阶段保存失败，请重试')) }
    finally { mutatingRef.current = false; setBusy(false) }
  }

  function chooseAnswer(questionID: string, option: number) {
    if (review || mutatingRef.current) return
    const current = stateRef.current
    if (current.stage !== 'blind' && current.stage !== 'retest') return
    const phase = current.stage
    const next = { ...current, answers: { ...current.answers, [phase]: { ...current.answers[phase], [questionID]: option } } }
    publish(next)
    void persist(next).catch(() => undefined)
  }

  async function submit(phase: ListeningPhase) {
    if (!session || mutatingRef.current || !canSubmitAnswers(session.questions, stateRef.current.answers[phase])) return
    mutatingRef.current = true
    setBusy(true)
    setError('')
    mediaRef.current?.pause()
    try {
      await saveChain.current.catch(() => undefined)
      const key = `${session.progress.cycle}:${phase}`
      keys.current[key] ??= crypto.randomUUID()
      const attempt = await submitListeningAttempt(materialID, {
        phase, answers: answerList(session.questions, stateRef.current.answers[phase]), assisted: stateRef.current.assisted[phase], idempotency_key: keys.current[key],
      })
      setSession((current) => current ? { ...current, attempts: [...current.attempts.filter((item) => item.phase !== phase), attempt] } : current)
      if (phase === 'blind') {
        const next = { ...stateRef.current, stage: 'intensive' as const }
        const progress = await persist(next)
        publish(next)
        setSession((current) => current ? { ...current, progress } : current)
      }
    } catch (err) { setError(message(err, '提交失败，请重试；重复点击不会重复记录')) }
    finally { mutatingRef.current = false; setBusy(false) }
  }

  async function finish() {
    if (mutatingRef.current) return
    mutatingRef.current = true
    setBusy(true)
    setError('')
    resetPlayer()
    try {
      await saveChain.current.catch(() => undefined)
      const progress = await completeListening(materialID)
      setReview(null)
      publish({ ...stateRef.current, stage: 'completed' })
      setSession((current) => current ? { ...current, progress } : current)
    } catch (err) { setError(message(err, '完成训练失败，请重试')) }
    finally { mutatingRef.current = false; setBusy(false) }
  }

  async function restart() {
    if (mutatingRef.current) return
    mutatingRef.current = true
    setBusy(true)
    setError('')
    positionRef.current = 0
    try {
      const next = { ...restoreExercise(), bookmarks: stateRef.current.bookmarks }
      await persist(next, true)
      setNextStatus('idle')
      setRetry((value) => value + 1)
    } catch (err) { setError(message(err, '新一轮训练创建失败')) }
    finally { mutatingRef.current = false; setBusy(false) }
  }

  async function openNextMaterial() {
    if (mutatingRef.current) return
    const controller = new AbortController()
    nextRequest.current = controller
    mutatingRef.current = true
    setNextStatus('loading')
    try {
      const catalog = await getListeningMaterials(filters.level, 'new', controller.signal)
      if (controller.signal.aborted) return
      const next = nextUnpracticedMaterial(catalog.materials, materialID)
      if (next) {
        navigate(listeningPath(filters, next.id))
        window.scrollTo(0, 0)
      } else {
        setNextStatus('empty')
      }
    } catch (err) {
      if (!controller.signal.aborted) {
        console.error('[listening] failed to load next unpracticed material', err)
        setNextStatus('error')
      }
    } finally {
      mutatingRef.current = false
      nextRequest.current = null
    }
  }

  function playSentence(index: number, slow = false) {
    if (mutatingRef.current) return
    const sentence = session?.lesson.sentences.find((item) => item.index === index)
    const bounds = sentence ? sentenceBounds(sentence) : null
    const media = mediaRef.current
    if (!bounds || !media) return
    if (review) {
      setReview({ ...review, sentenceIndex: index })
    } else {
      publish({ ...stateRef.current, sentenceIndex: index })
      positionRef.current = bounds.start * 1000
      void persist().catch(() => undefined)
    }
    rangeRef.current = bounds
    media.currentTime = bounds.start
    media.playbackRate = slow ? 0.75 : rate
    if (slow) setRate(0.75)
    void media.play().catch((error: unknown) => { if (shouldReportPlaybackError(error)) setMediaError('音频播放失败，请重试或检查音频资源') })
  }

  function onTimeUpdate() {
    const media = mediaRef.current
    if (!media || mutatingRef.current) return
    const range = rangeRef.current
    if (range && media.currentTime >= range.end) {
      if (loopRef.current) media.currentTime = range.start
      else { media.pause(); media.currentTime = range.end; rangeRef.current = null }
    }
    if (review) return
    positionRef.current = media.currentTime * 1000
    if (Math.abs(positionRef.current - lastSavedPosition.current) >= 4000) {
      lastSavedPosition.current = positionRef.current
      void persist().catch(() => undefined)
    }
  }

  function toggleBookmark(index: number) {
    if (mutatingRef.current) return
    const current = stateRef.current
    const next = { ...current, bookmarks: current.bookmarks.includes(index) ? current.bookmarks.filter((item) => item !== index) : [...current.bookmarks, index].sort((a, b) => a - b) }
    publish(next)
    void persist(next).catch(() => undefined)
  }

  if (loading) return <div className={styles.page}><div className={styles.loading}><Spinner /><span>正在恢复听力训练…</span></div></div>
  if (!session) return <div className={styles.page}><Link to={backPath}>返回听力库</Link><div className={styles.error} role="alert"><p>{error}</p><Button onClick={() => setRetry((value) => value + 1)}>重试加载</Button></div></div>

  const { lesson, questions, attempts } = session
  const stage = review?.stage ?? exercise.stage
  const completed = exercise.stage === 'completed'
  const blind = attempts.find((attempt) => attempt.phase === 'blind')
  const retest = attempts.find((attempt) => attempt.phase === 'retest')
  const phase = stage === 'retest' ? 'retest' : 'blind'
  const result = phase === 'blind' ? blind : retest
  const sentenceIndex = review?.sentenceIndex ?? exercise.sentenceIndex
  const currentSentence = lesson.sentences.find((sentence) => sentence.index === sentenceIndex) ?? lesson.sentences[0]
  const aligned = !!currentSentence && !!sentenceBounds(currentSentence)
  const mediaURL = lesson.audio_url || lesson.video_url
  const trainingReady = session.training_ready ?? (!!mediaURL && questions.length > 0 && lesson.sentences.length > 0 && lesson.sentences.every((sentence) => !!sentenceBounds(sentence)))
  const baselineNeeded = needsBlindBaseline(trainingReady, exercise.stage, attempts)
  const intensive = stage === 'intensive'
  const stepAction = (target: ListeningStage) => getListeningStepAction(exercise.stage, target, trainingReady, !!mediaURL, attempts)
  const unavailableHint = questions.length === 0
    ? '这篇素材暂未配置理解题，目前可逐句精听。'
    : '完整训练所需的素材待补充，暂无法复测。'
  const stepHint = completed ? ''
    : !trainingReady ? unavailableHint
    : baselineNeeded ? '理解题已补充，完成首次作答后即可复测。'
    : !blind ? '提交盲听答案后，可进入逐句精听。'
    : !retest ? '提交复测答案后，可查看训练结果。' : ''

  function selectStep(target: ListeningStage) {
    if (target === stage || mutatingRef.current) return
    const action = stepAction(target)
    if (action === 'locked') return
    if (action === 'advance') {
      void changeStage(target)
      return
    }
    const media = mediaRef.current
    if (!review && media && media.readyState >= HTMLMediaElement.HAVE_METADATA) {
      positionRef.current = media.currentTime * 1000
      void persist().catch(() => undefined)
    }
    resetPlayer()
    setReview(target === exercise.stage ? null : { stage: target, sentenceIndex: exercise.sentenceIndex })
  }

  return <div className={styles.page}>
    <div className={styles.practiceNav}><Link to={backPath}>返回听力库</Link><span className={styles.muted} role="status">{saveStatus === 'saving' ? '正在保存…' : saveStatus === 'failed' ? '保存失败' : '进度已保存'}</span></div>
    <header className={styles.header}><div><p className={styles.eyebrow}>🎧 教材精听</p><h1 lang="ja">{lesson.title}</h1></div><Badge level={lesson.jlpt_level} /></header>
    <nav className={styles.stepNav} aria-label="训练步骤" aria-describedby={stepHint ? 'listening-step-hint' : undefined}>
      <ol className={styles.steps}>{stages.map((item, index) => <li key={item}>
        <button
          type="button"
          aria-current={stage === item ? 'step' : undefined}
          aria-disabled={busy || undefined}
          disabled={stepAction(item) === 'locked'}
          onClick={() => selectStep(item)}
        >
          <span aria-hidden="true">{index + 1}</span>{STAGE_LABELS[item]}
        </button>
      </li>)}</ol>
      {stepHint && <p id="listening-step-hint" className={styles.muted}>{stepHint}</p>}
    </nav>
    {error && <div className={styles.error} role="alert">{error}<p>请重试刚才的操作，或重新加载已保存进度。</p><button className={styles.secondaryButton} type="button" onClick={() => setRetry((value) => value + 1)}>重新加载已保存进度</button></div>}
    {saveStatus === 'failed' && <div className={styles.error} role="alert">最新进度尚未保存。<Button variant="secondary" onClick={() => { void persist().catch(() => undefined) }}>重试保存</Button></div>}

    {stage !== 'completed' && <section className={styles.playerPanel} aria-label="听力播放器">
      <div><h2>{STAGE_LABELS[stage]} · {audioSourceLabel(session.audio_source)}</h2><p className={styles.muted}>{intensive ? '选择一句，反复听清楚；日文和中文可以分别打开。' : '先不看文本，听完整段音频，再回答理解题。可重听，本训练不计为 JLPT 模考。'}</p></div>
      {mediaURL ? <audio key={`${stage}:${mediaURL}`} ref={mediaRef} controls preload="metadata" src={mediaURL} aria-label="教材听力音频"
        onLoadedMetadata={({ currentTarget: media }) => {
          if (!review) media.currentTime = positionRef.current / 1000
          media.playbackRate = rate
        }}
        onTimeUpdate={onTimeUpdate}
        onPause={() => {
          if (!review && !mutatingRef.current) void persist().catch(() => undefined)
        }}
        onSeeked={({ currentTarget: media }) => {
          if (review || mutatingRef.current) return
          positionRef.current = media.currentTime * 1000
          void persist().catch(() => undefined)
        }}
        onRateChange={() => {
          const playbackRate = mediaRef.current?.playbackRate ?? 1
          setRate(playbackRate)
          const current = stateRef.current
          if (!review && !mutatingRef.current && playbackRate < 1 && (current.stage === 'blind' || current.stage === 'retest') && !current.assisted[current.stage]) {
            const next = { ...current, assisted: { ...current.assisted, [current.stage]: true } }
            publish(next)
            void persist(next).catch(() => undefined)
          }
        }}
        onError={() => setMediaError('音频无法加载，请重试；若持续失败，请通过教材勘误反馈。')} /> : <p className={styles.error}>这篇教材暂无可用音频，暂时无法训练。</p>}
      {mediaError && <div className={styles.error} role="alert"><p>{mediaError}</p><button className={styles.secondaryButton} type="button" onClick={() => { setMediaError(''); mediaRef.current?.load() }}>重试音频</button></div>}
      {intensive && <div className={styles.playerTools}>
        <label>播放速度 <select aria-label="播放速度" value={rate} onChange={(event) => { const next = Number(event.target.value); setRate(next); if (mediaRef.current) mediaRef.current.playbackRate = next }}>{[0.75, 1, 1.25].map((value) => <option key={value} value={value}>{value}×</option>)}</select></label>
        <button className={styles.secondaryButton} type="button" aria-pressed={looping} disabled={!aligned} onClick={() => { loopRef.current = !looping; setLooping(!looping); if (!looping && currentSentence) playSentence(currentSentence.index) }}>🔁 单句循环{looping ? '：开' : '：关'}</button>
        <button className={styles.secondaryButton} type="button" onClick={() => { stopSentenceLoop(); if (mediaRef.current) { mediaRef.current.currentTime = 0; void mediaRef.current.play().catch((error: unknown) => { if (shouldReportPlaybackError(error)) setMediaError('音频播放失败') }) } }}>▶️ 播放整段</button>
      </div>}
    </section>}

    {(stage === 'blind' || stage === 'retest') && <section aria-label="听力理解题">
      {baselineNeeded ? <div className={styles.empty}><h2>理解题已就绪</h2><p>先完成本轮首次盲听，再进行精听后的复测。</p><Button loading={busy} onClick={() => selectStep('blind')}>开始盲听理解</Button></div> : (trainingReady || result) ? <>
        <ListeningQuestions questions={questions} answers={exercise.answers[phase]} result={result} disabled={busy || !!review} onAnswer={chooseAnswer} />
        <div className={styles.actions}>{result
          ? (stage === 'blind' ? <Button loading={busy} onClick={() => selectStep('intensive')}>进入逐句精听</Button>
            : completed ? <Button onClick={() => selectStep('completed')}>查看训练结果</Button>
            : <Button loading={busy} onClick={() => { void finish() }}>完成训练，安排复习</Button>)
          : <Button disabled={!!review || !mediaURL || !canSubmitAnswers(questions, exercise.answers[phase])} loading={busy} onClick={() => { void submit(phase) }}>{stage === 'blind' ? '提交理解题，进入精听' : '提交复测答案'}</Button>}</div>
      </> : <div className={styles.empty}><h2>{questions.length === 0 ? '这篇素材暂未配置理解题' : '这篇素材暂未具备完整训练条件'}</h2><p>可以精听和收藏难句，音频、时间轴和理解题就绪后可进行完整测评。</p><Button loading={busy} disabled={stepAction('intensive') === 'locked'} onClick={() => selectStep('intensive')}>开始精听</Button></div>}
    </section>}

    {intensive && <section className={styles.intensivePanel} aria-label="逐句精听">
      <div className={styles.sectionHeading}><h2>逐句突破</h2><span className={styles.muted}>已收藏 {exercise.bookmarks.length} 个难句</span></div>
      <div className={styles.textToggles}><button type="button" aria-pressed={showJapanese} onClick={() => setShowJapanese(!showJapanese)}>{showJapanese ? '隐藏' : '显示'}日文</button><button type="button" aria-pressed={showChinese} onClick={() => setShowChinese(!showChinese)}>{showChinese ? '隐藏' : '显示'}中文</button></div>
      <div className={styles.sentences}>{lesson.sentences.map((sentence) => {
        const bounds = sentenceBounds(sentence)
        const selected = sentence.index === sentenceIndex
        const bookmarked = exercise.bookmarks.includes(sentence.index)
        const japanese = sentence.tokens.map((token) => token.surface).join('')
        return <article key={sentence.index} className={selected ? styles.activeSentence : styles.sentence}>
          <div className={styles.sentenceHeader}><strong>第 {sentence.index + 1} 句</strong><button type="button" aria-pressed={bookmarked} onClick={() => toggleBookmark(sentence.index)}>{bookmarked ? '⭐ 已收藏难句' : '⭐ 收藏难句'}</button></div>
          {showJapanese ? <p lang="ja" className={styles.japanese} data-dictionary-text>{japanese}</p> : <p className={styles.hiddenText}>日文已隐藏，先试着听一遍</p>}
          {showChinese && <p className={styles.translation}>{sentence.chinese || '暂无中文译文'}</p>}
          <div className={styles.sentenceActions}><button type="button" disabled={!bounds || !mediaURL} onClick={() => playSentence(sentence.index)}>▶️ 播放这句</button><button type="button" disabled={!bounds || !mediaURL} onClick={() => playSentence(sentence.index, true)}>🐢 0.75× 慢听</button>{showJapanese && japanese.trim() && <DictionaryButton text={japanese} />}{!bounds && <small>时间轴待补充，可使用整段音频</small>}</div>
        </article>
      })}</div>
      {blind && <div className={styles.baseline}><h3>首次理解：{blind.correct_count}/{blind.total} 题</h3><ListeningQuestions questions={questions} answers={{}} result={blind} onAnswer={() => undefined} onEvidence={(index) => playSentence(index)} /></div>}
      {baselineNeeded && <p className={styles.muted}>补做本轮首次理解题后即可复测。</p>}
      {!trainingReady && <p className={styles.muted}>{unavailableHint}</p>}
      <div className={styles.actions}>{baselineNeeded ? <Button loading={busy} onClick={() => selectStep('blind')}>补做首次理解题</Button>
        : stepAction('retest') !== 'locked' ? <Button loading={busy} onClick={() => selectStep('retest')}>{retest ? '查看复测答案' : review ? '返回复测' : '关掉文本，开始复测'}</Button>
        : <Button loading={busy} variant="secondary" onClick={() => { void persist().then(() => navigate(backPath)).catch(() => undefined) }}>保存精听进度，返回素材库</Button>}</div>
    </section>}

    {stage === 'completed' && <section className={styles.resultPanel}>
      {completed && <p className={styles.eyebrow}>✅ 本轮训练已完成</p>}
      <h2>{completed ? '把听懂的内容留住' : '训练结果'}</h2>
      <div className={styles.scoreGrid}><Score label="首次理解" attempt={blind} /><Score label="关文本复测" attempt={retest} /><div><span>难句收藏</span><strong>{exercise.bookmarks.length}<small> 句</small></strong></div></div>
      {completed && <p>下次复习：<strong>{formatDueDate(session.progress.due_at)}</strong>。复习时会重新盲听，再检查是否真正听懂。</p>}
      <p className={styles.muted}>本训练允许重听；首次结果指第一次作答。复测使用同一材料与题目，结果反映本篇掌握情况。{blind?.assisted || retest?.assisted ? '本轮使用过慢放辅助。' : ''}</p>
      {retest && <ListeningQuestions questions={questions} answers={{}} result={retest} onAnswer={() => undefined} />}
      {completed && nextStatus === 'empty' && <p className={styles.muted} role="status">当前范围暂无可开始的未练习素材。</p>}
      {completed && nextStatus === 'error' && <p className={styles.error} role="alert">暂时无法加载下一篇，请重试。</p>}
      <div className={styles.actions}>{completed ? <>
        <Button variant="secondary" loading={busy} disabled={nextStatus === 'loading'} onClick={() => { void restart() }}>再练一轮</Button>
        {nextStatus === 'empty' ? <Link className={styles.secondaryButton} to={backPath}>返回听力库</Link>
          : <Button disabled={busy} loading={nextStatus === 'loading'} onClick={() => { void openNextMaterial() }}>{nextStatus === 'error' ? '重试加载下一篇' : '下一篇未练习'}</Button>}
      </> : <Button loading={busy} onClick={() => { void finish() }}>完成训练，安排复习</Button>}</div>
    </section>}
    <footer className={styles.practiceFooter}><Link to={`/lesson/${lesson.id}`}>📚 查看课文</Link>{lesson.shadowing_enabled && <Link to={`/lesson/${lesson.id}/shadowing`}>🎙️ 进入跟读练习</Link>}<span className={styles.muted}>听力训练与口语评分分别记录</span></footer>
  </div>
}

function Score({ label, attempt }: { label: string; attempt?: ListeningAttempt }) {
  return <div><span>{label}</span><strong>{attempt ? `${attempt.correct_count}/${attempt.total}` : '—'}<small> 题</small></strong></div>
}
function message(error: unknown, fallback: string) { return error instanceof Error ? error.message : fallback }
