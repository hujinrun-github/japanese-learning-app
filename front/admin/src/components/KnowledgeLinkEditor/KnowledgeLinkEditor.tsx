import { useEffect, useState, type FormEvent } from 'react'
import { adminFetch } from '@/api/client'
import Modal from '@/components/Modal/Modal'
import styles from './KnowledgeLinkEditor.module.css'

type LinkKind = 'word' | 'grammar'
type KnowledgeLink = { kind: LinkKind; id: number; title: string; reading?: string; meaning: string; jlpt_level: string; hint?: boolean }
type DictionaryEntry = { id: number; kanji_form?: string; reading?: string; name?: string; meaning: string; jlpt_level: string }
type Search = { kind: LinkKind; query: string; page: number }
const pageSize = 20
const maxLinks = 30

function entryKey(entry: KnowledgeLink) {
  return `${entry.kind}:${entry.id}`
}

function EntryLabel({ entry }: { entry: KnowledgeLink }) {
  return <span className={styles.entryLabel}>
    <strong lang="ja">{entry.title}</strong>
    {entry.reading && entry.reading !== entry.title && <span lang="ja">{entry.reading}</span>}
    <span>{entry.kind === 'word' ? 'Word' : 'Grammar'} · {entry.jlpt_level}</span>
    <span>{entry.meaning}</span>
  </span>
}

export default function KnowledgeLinkEditor({ path, sourceId, sourceText, description, canHint, onClose }: {
  path: string; sourceId: string | number; sourceText: string; description: string; canHint?: boolean; onClose: () => void
}) {
  const [links, setLinks] = useState<KnowledgeLink[] | null>(null)
  const [loadAttempt, setLoadAttempt] = useState(0)
  const [kind, setKind] = useState<LinkKind>('word')
  const [query, setQuery] = useState('')
  const [search, setSearch] = useState<Search | null>(null)
  const [results, setResults] = useState<{ entries: KnowledgeLink[]; total: number } | null>(null)
  const [loadError, setLoadError] = useState('')
  const [searchError, setSearchError] = useState('')
  const [saveError, setSaveError] = useState('')
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    let active = true
    adminFetch<{ links: KnowledgeLink[] }>('GET', path)
      .then((response) => { if (active) setLinks(response.links) })
      .catch((reason: unknown) => {
        if (active) setLoadError(reason instanceof Error ? reason.message : 'Could not load linked entries.')
      })
    return () => { active = false }
  }, [path, loadAttempt])

  useEffect(() => {
    if (!search) return
    let active = true
    const params = new URLSearchParams({ search: search.query, size: String(pageSize), page: String(search.page) })
    adminFetch<{ items: DictionaryEntry[] | null; total: number }>('GET', `/${search.kind === 'word' ? 'words' : 'grammar'}?${params}`)
      .then(({ items, total }) => {
        if (!active) return
        const entries = (items ?? []).map((item) => ({
          kind: search.kind,
          id: item.id,
          title: item.kanji_form || item.name || item.reading || String(item.id),
          reading: item.reading,
          meaning: item.meaning,
          jlpt_level: item.jlpt_level,
        }))
        setResults({ entries, total })
      })
      .catch((reason: unknown) => {
        if (active) setSearchError(reason instanceof Error ? reason.message : 'Search failed.')
      })
    return () => { active = false }
  }, [search])

  function runSearch(next: Search | null) {
    setSearchError('')
    setResults(null)
    setSearch(next)
  }

  function submitSearch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    if (query.trim()) runSearch({ kind, query: query.trim(), page: 1 })
  }

  function add(entry: KnowledgeLink) {
    setLinks((current) => current && current.length < maxLinks && !current.some((item) => entryKey(item) === entryKey(entry))
      ? [...current, entry] : current)
  }

  function move(index: number, step: number) {
    setLinks((current) => {
      const destination = index + step
      if (!current || destination < 0 || destination >= current.length) return current
      const ordered = [...current]
      const [entry] = ordered.splice(index, 1)
      ordered.splice(destination, 0, entry)
      return ordered
    })
  }

  async function save() {
    if (!links || saving) return
    setSaving(true)
    setSaveError('')
    try {
      await adminFetch('PUT', path, { links: links.map(({ kind, id, hint }) => ({ kind, id, ...(canHint && hint ? { hint } : {}) })) })
      onClose()
    } catch (reason) {
      setSaveError(reason instanceof Error ? reason.message : 'Could not save linked entries.')
    } finally { setSaving(false) }
  }

  return (
    <Modal open title={`Link dictionary entries · #${sourceId}`} onClose={() => { if (!saving) onClose() }}>
      <p className={styles.sourceText}>{sourceText}</p>
      <p>{description}</p>
      {loadError ? (
        <div role="alert" className="adm-error">{loadError} <button className="adm-btn" type="button" onClick={() => {
          setLoadError(''); setLoadAttempt((attempt) => attempt + 1)
        }}>Retry</button></div>
      ) : !links ? <p role="status">Loading linked entries…</p> : (
        <>
          <form className={styles.linkSearch} onSubmit={submitSearch}>
            <label>Entry type
              <select value={kind} onChange={(event) => { setKind(event.target.value as LinkKind); runSearch(null) }}>
                <option value="word">Words</option>
                <option value="grammar">Grammar</option>
              </select>
            </label>
            <label>Search dictionary
              <input type="search" value={query} onChange={(event) => setQuery(event.target.value)} placeholder="Japanese, kana or meaning" />
            </label>
            <button type="submit" className="adm-btn" disabled={!query.trim()}>Search</button>
          </form>
          {searchError && <p role="alert" className="adm-error">{searchError}</p>}
          {search && !results && !searchError && <p role="status">Searching…</p>}
          {search && results && (
            <section aria-label="Dictionary search results">
              {results.entries.length ? <ul className={`${styles.linkList} ${styles.results}`}>
                {results.entries.map((entry) => {
                  const added = links.some((item) => entryKey(item) === entryKey(entry))
                  return <li key={entryKey(entry)}>
                    <EntryLabel entry={entry} />
                    <button type="button" className="adm-btn" disabled={added || saving || links.length >= maxLinks}
                      aria-label={added ? `Added ${entry.title}` : `Add ${entry.title}`}
                      onClick={() => add(entry)}>{added ? 'Added' : 'Add'}</button>
                  </li>
                })}
              </ul> : <p role="status">No matching entries.</p>}
              {results.total > pageSize && (
                <div className={styles.linkActions}>
                  <button type="button" className="adm-btn" disabled={search.page <= 1}
                    onClick={() => runSearch({ ...search, page: search.page - 1 })}>Previous</button>
                  <span>{search.page} / {Math.ceil(results.total / pageSize)}</span>
                  <button type="button" className="adm-btn" disabled={search.page * pageSize >= results.total}
                    onClick={() => runSearch({ ...search, page: search.page + 1 })}>Next</button>
                </div>
              )}
            </section>
          )}
          <section aria-label="Selected entries">
            <h3>Selected entries ({links.length}/{maxLinks})</h3>
            {!links.length && <p>No entries linked.</p>}
            <ol className={styles.linkList}>
              {links.map((entry, index) => (
                <li key={entryKey(entry)}>
                  <EntryLabel entry={entry} />
                  {canHint && entry.kind === 'grammar' && (
                    <label className={styles.hint}>
                      <input type="checkbox" checked={!!entry.hint} disabled={saving}
                        onChange={(event) => {
                          const hint = event.target.checked
                          setLinks((current) => current?.map((item) => entryKey(item) === entryKey(entry) ? { ...item, hint } : item) ?? null)
                        }} />
                      Before sentence writing
                    </label>
                  )}
                  <div className={styles.linkActions}>
                    <button type="button" className="adm-btn" disabled={saving || index === 0}
                      aria-label={`Move ${entry.title} up`} onClick={() => move(index, -1)}>↑</button>
                    <button type="button" className="adm-btn" disabled={saving || index === links.length - 1}
                      aria-label={`Move ${entry.title} down`} onClick={() => move(index, 1)}>↓</button>
                    <button type="button" className="adm-btn" disabled={saving} aria-label={`Remove ${entry.title}`}
                      onClick={() => setLinks((current) => current?.filter((item) => entryKey(item) !== entryKey(entry)) ?? null)}>Remove</button>
                  </div>
                </li>
              ))}
            </ol>
          </section>
          {saveError && <p role="alert" className="adm-error">{saveError}</p>}
          <button type="button" className="adm-saveBtn" disabled={saving} onClick={() => void save()}>{saving ? 'Saving…' : 'Save links'}</button>
        </>
      )}
    </Modal>
  )
}
