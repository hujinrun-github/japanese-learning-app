import { useEffect, useId, useRef, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { Badge } from '@/components/ui/Badge'
import type { GrammarPoint, Word } from '@/types/api'
import type { ContextualSearchTarget } from './ContextualSearch'
import { dictionaryTerms, getDictionarySelection, validSearchKeyword } from './contextQuery'
import { SearchResults, searchFilters, type SearchEntry, type SearchFilter } from './SearchResults'
import { useLookup } from './useLookup'
import { WordEntry } from './WordEntryPage'
import styles from './ContextualSearch.module.css'
import searchStyles from './SearchPage.module.css'

export default function ContextualSearchDialog({ target, onClose }: { target: ContextualSearchTarget; onClose: () => void }) {
  const { t } = useTranslation()
  const title = useId()
  const dialog = useRef<HTMLDialogElement>(null)
  const close = useRef<HTMLButtonElement>(null)
  const inputRef = useRef<HTMLInputElement>(null)
  const [input, setInput] = useState(target.keyword ?? '')
  const [query, setQuery] = useState(target.keyword ?? '')
  const [filter, setFilter] = useState<SearchFilter>('all')
  const [entry, setEntry] = useState<SearchEntry | null>(null)
  const invalid = !!input.trim() && !validSearchKeyword(input.trim())
  const globalLookup = !target.source && !target.keyword
  const fullSearchParams = new URLSearchParams()
  if (query) fullSearchParams.set('q', query)
  if (filter !== 'all') fullSearchParams.set('type', filter)

  useEffect(() => {
    if (!dialog.current?.open) dialog.current?.showModal()
    if (globalLookup) inputRef.current?.focus()
    else close.current?.focus()
  }, [globalLookup])

  function search(keyword: string) {
    setInput(keyword)
    setQuery(keyword)
    setFilter('all')
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (!invalid) search(input.trim())
  }

  return (
    <dialog ref={dialog} className={styles.dialog} aria-labelledby={title} onClose={onClose}
      onClick={(event) => { if (event.target === event.currentTarget) dialog.current?.close() }}>
      <div className={styles.panel}>
        <header className={styles.heading}>
          <h2 id={title}>{t(globalLookup ? 'search.title' : 'search.inContext')}</h2>
          {globalLookup && (
            <Link className={styles.fullSearch} to={{ pathname: '/search', search: fullSearchParams.toString() }}>
              {t('search.openFullPage')}
            </Link>
          )}
          <button className={styles.close} ref={close} type="button" aria-label={t('search.close')}
            onClick={() => dialog.current?.close()}>×</button>
        </header>
        <div className={styles.body}>
          <div hidden={entry !== null}>
            {target.source && <SourceTerms source={target.source} keyword={query} onSelect={search} />}
            <form className={searchStyles.form} role="search" onSubmit={submit}>
              <input ref={inputRef} type="search" value={input} onChange={(event) => setInput(event.target.value)}
                placeholder={t('search.placeholder')} aria-label={t('search.placeholder')} aria-invalid={invalid}
                spellCheck={false} autoComplete="off" />
              <button type="submit" className={searchStyles.submit} disabled={invalid}>{t('search.submit')}</button>
            </form>
            {invalid && <p role="alert">{t('search.tooLong')}</p>}
            {query && (
              <>
                <div className={searchStyles.filters} role="group" aria-label={t('search.filters')}>
                  {searchFilters.map((item) => (
                    <button key={item} type="button" aria-pressed={filter === item} onClick={() => setFilter(item)}>
                      {t(`search.${item}`)}
                    </button>
                  ))}
                </div>
                <SearchResults key={query} query={query} filter={filter} onFilter={setFilter}
                  onOpenEntry={(next) => { setEntry(next); close.current?.focus() }} />
              </>
            )}
          </div>
          {entry && (
            <>
              <button type="button" className={searchStyles.back}
                onClick={() => { setEntry(null); close.current?.focus() }}>← {t('search.back')}</button>
              {entry.type === 'words' ? <WordPreview id={entry.id} /> : <GrammarPreview id={entry.id} />}
            </>
          )}
        </div>
      </div>
    </dialog>
  )
}

function SourceTerms({ source, keyword, onSelect }: { source: string; keyword: string; onSelect: (term: string) => void }) {
  const { t } = useTranslation()
  const [terms, setTerms] = useState<ReturnType<typeof dictionaryTerms> | null>(null)
  const [selection, setSelection] = useState('')

  useEffect(() => {
    function update() { setSelection(getDictionarySelection(window.getSelection())) }
    document.addEventListener('selectionchange', update)
    return () => document.removeEventListener('selectionchange', update)
  }, [])

  useEffect(() => {
    let active = true
    import('kuromojin').then(({ tokenize }) => tokenize(source)).then((tokens) => {
      if (active) setTerms(dictionaryTerms(tokens.map((token) => ({ surface: token.surface_form, base: token.basic_form }))))
    }).catch((error: unknown) => {
      if (!active) return
      console.warn('[dictionary] could not split study text', error)
      setTerms([])
    })
    return () => { active = false }
  }, [source])

  return (
    <div className={styles.source}>
      <p lang="ja" data-dictionary-text>{source}</p>
      {selection && (
        <button type="button" className={styles.trigger} onPointerDown={(event) => event.preventDefault()}
          onClick={() => {
            onSelect(selection)
            window.getSelection()?.removeAllRanges()
          }}>{t('search.findSelection')}</button>
      )}
      {!terms && <small role="status">{t('common.loading')}</small>}
      {!!terms?.length && (
        <div className={styles.terms} role="group" aria-label={t('search.pickTerm')}>
          {terms.map(({ surface, keyword: term }) => (
            <button key={surface} type="button" lang="ja" aria-pressed={keyword === term}
              onClick={() => onSelect(term)}>{surface}</button>
          ))}
        </div>
      )}
    </div>
  )
}

function WordPreview({ id }: { id: number }) {
  const { result, retry } = useLookup<Word>(`/api/v1/words/${id}`)
  return result.status === 'ready' ? <WordEntry word={result.data} contextualLookup={false} />
    : <PreviewStatus status={result.status} retry={retry} />
}

function GrammarPreview({ id }: { id: number }) {
  const { t } = useTranslation()
  const { result, retry } = useLookup<GrammarPoint>(`/api/v1/grammar/${id}`)
  if (result.status !== 'ready') return <PreviewStatus status={result.status} retry={retry} />
  const point = result.data
  return (
    <article className={searchStyles.entry}>
      <header className={searchStyles.entryHead}>
        <h3 lang="ja">{point.name}</h3><Badge level={point.jlpt_level} />
      </header>
      <p>{point.meaning}</p>
      {point.conjunction_rule && <p><strong>{t('grammar.detail.conjunction')}</strong> {point.conjunction_rule}</p>}
      {point.usage_note && <p>{point.usage_note}</p>}
      {point.examples?.length > 0 && (
        <section className={searchStyles.examples}>
          <h3>{t('search.examples')}</h3>
          <ol>{point.examples.map((example, index) => (
            <li key={index}><div><p lang="ja">{example.japanese}</p><p>{example.chinese}</p></div></li>
          ))}</ol>
        </section>
      )}
    </article>
  )
}

function PreviewStatus({ status, retry }: { status: 'loading' | 'error'; retry: () => void }) {
  const { t } = useTranslation()
  if (status === 'loading') return <p role="status">{t('common.loading')}</p>
  return <div role="alert"><p>{t('search.entryUnavailable')}</p><button type="button" onClick={retry}>{t('search.retry')}</button></div>
}
