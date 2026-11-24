import { useEffect, useRef, useState, type FormEvent } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useTranslation } from 'react-i18next'
import { SearchResults, searchFilters, type SearchFilter } from './SearchResults'
import { validSearchKeyword } from './contextQuery'
import styles from './SearchPage.module.css'

export default function SearchPage() {
  const { t } = useTranslation()
  const [params, setParams] = useSearchParams()
  const query = (params.get('q') ?? '').trim()
  const type = params.get('type')
  const filter = type === 'words' || type === 'grammar' ? type : 'all'
  const [input, setInput] = useState(query)
  const [submittedInvalid, setSubmittedInvalid] = useState(false)
  const inputRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    setInput(query)
    setSubmittedInvalid(false)
  }, [query])

  const validQuery = !query || validSearchKeyword(query)
  function setSearch(nextQuery: string, nextFilter: SearchFilter) {
    const next = new URLSearchParams()
    if (nextQuery) next.set('q', nextQuery)
    if (nextFilter !== 'all') next.set('type', nextFilter)
    if (next.toString() !== params.toString()) setParams(next)
  }

  function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    const keyword = input.trim()
    if (keyword && !validSearchKeyword(keyword)) {
      setSubmittedInvalid(true)
      return
    }
    setInput(keyword)
    setSearch(keyword, filter)
  }

  function clear() {
    setInput('')
    setSubmittedInvalid(false)
    setSearch('', 'all')
    inputRef.current?.focus()
  }

  return (
    <div className={styles.page}>
      <div className={styles.searchPanel}>
        <h1>{t('search.title')}</h1>
        <form className={styles.form} role="search" onSubmit={submit}>
          <span className={styles.searchIcon} aria-hidden="true">🔍</span>
          <input ref={inputRef} type="search" value={input} aria-label={t('search.placeholder')}
            placeholder={t('search.placeholder')} autoComplete="off" spellCheck={false}
            aria-invalid={submittedInvalid || !validQuery}
            onChange={(event) => { setInput(event.target.value); setSubmittedInvalid(false) }} />
          {input && (
            <button type="button" className={styles.clear} onClick={clear} aria-label={t('search.clear')}>
              <span aria-hidden="true">×</span>
            </button>
          )}
          <button type="submit" className={styles.submit}>{t('search.submit')}</button>
        </form>
        <div className={styles.filters} role="group" aria-label={t('search.filters')}>
          {searchFilters.map((item) => (
            <button key={item} type="button" aria-pressed={filter === item} onClick={() => setSearch(query, item)}>
              {t(`search.${item}`)}
            </button>
          ))}
        </div>
      </div>

      {(submittedInvalid || !validQuery) && <p className={styles.feedback} role="alert">{t('search.tooLong')}</p>}
      {!query && (
        <div className={styles.suggestions}>
          <span>{t('search.try')}</span>
          {['勉強', 'べんきょう', '〜てもいい'].map((term) => (
            <button key={term} type="button" lang="ja" onClick={() => setSearch(term, 'all')}>{term}</button>
          ))}
        </div>
      )}
      {query && validQuery && (
        <SearchResults key={query} query={query} filter={filter} onFilter={(next) => setSearch(query, next)} />
      )}
    </div>
  )
}
