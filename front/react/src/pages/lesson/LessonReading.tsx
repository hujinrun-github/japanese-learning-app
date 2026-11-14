import { createPortal } from 'react-dom'
import { useEffect, useId, useMemo, useRef, useState, type MouseEvent } from 'react'
import { useTranslation } from 'react-i18next'
import { ErrataButton } from '@/components/ui/ErrataButton'
import type { Sentence } from '@/types/api'
import { LessonAnnotationEditor } from './LessonAnnotationEditor'
import {
  loadLessonAnnotations,
  removeLessonAnnotation,
  saveLessonAnnotation,
  type LessonAnnotation,
} from './lessonAnnotations'
import { getLessonSelection, indexLessonText, sameTextRange, splitHighlights, type LessonSelection } from './lessonSelection'
import annotationStyles from './LessonAnnotation.module.css'
import styles from './LessonReading.module.css'

interface LessonReadingProps {
  lessonId: number
  sentences?: Sentence[] | null
}

type IndexedSentence = ReturnType<typeof indexLessonText>[number]

function JapaneseText({ tokens, showReadings, annotations }: {
  tokens: IndexedSentence['tokens']
  showReadings: boolean
  annotations: LessonAnnotation[]
}) {
  return tokens.map((token, index) => {
    const parts = splitHighlights(token.surface, token.start, annotations)
    const surface = (
      <span data-lesson-offset={token.start}>
        {parts.map(({ start, text, highlights }) => highlights.length > 0 ? (
          <mark key={start} data-annotation-offset={start}>{text}</mark>
        ) : (
          <span key={start}>{text}</span>
        ))}
      </span>
    )
    return showReadings && token.reading ? (
      <ruby key={index}>{surface}<rt>{token.reading}</rt></ruby>
    ) : (
      <span key={index}>{surface}</span>
    )
  })
}

export function LessonReading({ lessonId, sentences }: LessonReadingProps) {
  const { t } = useTranslation()
  const bodyId = useId()
  const hintId = useId()
  const annotationsId = useId()
  const body = useRef<HTMLDivElement>(null)
  const lastSelection = useRef<LessonSelection | null>(null)
  const [parallel, setParallel] = useState(false)
  const [showReadings, setShowReadings] = useState(true)
  const [annotations, setAnnotations] = useState<LessonAnnotation[]>([])
  const [loadState, setLoadState] = useState<'loading' | 'loaded' | 'failed'>('loading')
  const [retry, setRetry] = useState(0)
  const [selection, setSelection] = useState<LessonSelection | null>(null)
  const [draft, setDraft] = useState<LessonAnnotation | null>(null)
  const [overlapping, setOverlapping] = useState<LessonAnnotation[]>([])

  const indexed = useMemo(() => indexLessonText(sentences ?? []), [sentences])
  const chars = useMemo(() => Array.from(indexed.flatMap((s) => s.tokens.map((token) => token.surface)).join('')), [indexed])
  const activeAnnotations = useMemo(() => annotations.filter((item) =>
    chars.slice(item.start, item.end).join('') === item.quote), [annotations, chars])
  const canAnnotate = loadState === 'loaded' && indexed.length > 0

  useEffect(() => {
    if (indexed.length === 0) return
    const controller = new AbortController()
    loadLessonAnnotations(lessonId, controller.signal)
      .then((items) => {
        if (controller.signal.aborted) return
        if (!Array.isArray(items)) {
          setLoadState('failed')
          return
        }
        setAnnotations(items)
        setLoadState('loaded')
      })
      .catch(() => { if (!controller.signal.aborted) setLoadState('failed') })
    return () => controller.abort()
  }, [lessonId, indexed.length, retry])

  useEffect(() => {
    if (!canAnnotate || draft) return
    function updateSelection() {
      const selection = window.getSelection()
      const next = body.current ? getLessonSelection(body.current, selection, chars) : null
      if (next) lastSelection.current = next
      setSelection((previous) => previous && next && sameTextRange(previous, next) ? previous : next)
    }
    document.addEventListener('selectionchange', updateSelection)
    return () => document.removeEventListener('selectionchange', updateSelection)
  }, [canAnnotate, chars, draft])

  function openAnnotation(next: LessonAnnotation) {
    window.getSelection()?.removeAllRanges()
    lastSelection.current = null
    setSelection(null)
    setDraft(next)
    setOverlapping([])
  }

  function annotateSelection() {
    const selected = selection ?? lastSelection.current
    if (!selected) return
    const existing = activeAnnotations.find((item) => sameTextRange(selected, item))
    openAnnotation(existing ?? { ...selected, comment: '', links: [] })
  }

  function openHighlight(event: MouseEvent) {
    if (!window.getSelection()?.isCollapsed || !(event.target instanceof Element)) return
    const mark = event.target.closest<HTMLElement>('[data-annotation-offset]')
    if (!mark) return
    const offset = Number(mark.dataset.annotationOffset)
    const matching = activeAnnotations.filter((item) => item.start <= offset && offset < item.end)
    if (matching.length === 1) openAnnotation(matching[0])
    if (matching.length > 1) {
      setOverlapping(matching)
      document.getElementById(annotationsId)?.scrollIntoView({ block: 'nearest' })
    }
  }

  function switchLayout(value: boolean) {
    if (value === parallel) return
    window.getSelection()?.removeAllRanges()
    lastSelection.current = null
    setSelection(null)
    setParallel(value)
  }

  async function saveAnnotation(next: LessonAnnotation) {
    const saved = await saveLessonAnnotation(lessonId, next)
    if (!saved) throw new Error('Missing saved annotation')
    setAnnotations((items) => [...items.filter((item) => !sameTextRange(item, saved)), saved].sort((a, b) => a.start - b.start || a.end - b.end))
  }

  async function removeAnnotation(item: LessonAnnotation) {
    await removeLessonAnnotation(lessonId, item)
    setAnnotations((items) => items.filter((candidate) => !sameTextRange(candidate, item)))
  }

  if (!indexed.length) {
    return <p className={styles.empty}>{t('lesson.reading.empty')}</p>
  }

  return (
    <section className={styles.reader} aria-label={t('lesson.reading.label')}>
      <div className={styles.toolbar}>
        <div className={styles.modes} role="group" aria-label={t('lesson.reading.layout')}>
          <button type="button" aria-pressed={!parallel} aria-controls={bodyId} onClick={() => switchLayout(false)}>
            {t('lesson.reading.vertical')}
          </button>
          <button type="button" aria-pressed={parallel} aria-controls={bodyId} onClick={() => switchLayout(true)}>
            {t('lesson.reading.parallel')}
          </button>
        </div>
        <label className={styles.readings}>
          <input type="checkbox" checked={showReadings} onChange={(event) => setShowReadings(event.target.checked)} />
          {t('lesson.reading.furigana')}
        </label>
      </div>

      {loadState === 'failed' && (
        <p className={styles.hint} role="alert">
          {t('lesson.annotations.loadFailed')}{' '}
          <button type="button" className={styles.retry} onClick={() => {
            setLoadState('loading')
            setRetry((value) => value + 1)
          }}>{t('lesson.annotations.retry')}</button>
        </p>
      )}
      {(!parallel || canAnnotate) && (
        <p id={hintId} className={styles.hint}>
          {!parallel && t('lesson.reading.direction')}
          {!parallel && canAnnotate && ' '}
          {canAnnotate && t('lesson.annotations.hint')}
        </p>
      )}

      {parallel ? (
        <div ref={body} id={bodyId} className={styles.parallel} onClick={openHighlight}>
          {indexed.map((sentence) => (
            <div key={sentence.index} className={styles.pair}>
              <p lang="ja"><JapaneseText tokens={sentence.tokens} showReadings={showReadings} annotations={activeAnnotations} /></p>
              {sentence.chinese && <p className={styles.translation} lang="zh-Hans">{sentence.chinese}</p>}
              <ErrataButton
                module="lesson"
                itemKey={`${lessonId}:sentence:${sentence.index}`}
                label={sentence.tokens.map((token) => token.surface).join('')}
              />
            </div>
          ))}
        </div>
      ) : (
        <div ref={body} id={bodyId} className={styles.paper} role="region" aria-label={t('lesson.reading.original')} aria-describedby={hintId} tabIndex={0} lang="ja" onClick={openHighlight}>
          <p className={styles.verticalText}>
            {indexed.map((sentence) => (
              <JapaneseText key={sentence.index} tokens={sentence.tokens} showReadings={showReadings} annotations={activeAnnotations} />
            ))}
          </p>
        </div>
      )}

      {activeAnnotations.length > 0 && (
        <section id={annotationsId} className={annotationStyles.list} aria-label={t('lesson.annotations.saved')}>
          <h2>{t('lesson.annotations.saved')}</h2>
          {overlapping.length > 0 && <p role="status">{t('lesson.annotations.overlapping')}</p>}
          <ul>
            {activeAnnotations.map((item) => (
              <li key={`${item.start}:${item.end}`}>
                <button type="button" className={overlapping.some((match) => sameTextRange(match, item)) ? annotationStyles.matching : undefined} onClick={() => openAnnotation(item)}>
                  <mark lang="ja">{item.quote}</mark>
                  {item.comment && <small>{item.comment}</small>}
                  {item.links.length > 0 && <small>{t('lesson.annotations.linkCount', { count: item.links.length })}</small>}
                </button>
              </li>
            ))}
          </ul>
        </section>
      )}
      {selection && createPortal(
        <button type="button" className={annotationStyles.selectionAction} onPointerDown={(event) => event.preventDefault()} onClick={annotateSelection}>
          {t('lesson.annotations.create')}
        </button>,
        document.body,
      )}
      {draft && (
        <LessonAnnotationEditor
          annotation={draft}
          saved={activeAnnotations.some((item) => sameTextRange(draft, item))}
          onSave={saveAnnotation}
          onRemove={() => removeAnnotation(draft)}
          onClose={() => setDraft(null)}
        />
      )}
    </section>
  )
}
