import { useEffect, useState, type FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { adminFetch } from '@/api/client'
import KnowledgeLinkEditor from '@/components/KnowledgeLinkEditor/KnowledgeLinkEditor'

const kinds = {
  'listening-material': { label: 'Listening · materials', visibility: 'Entries appear during intensive listening.' },
  'listening-sentence': { label: 'Listening · transcript lines', visibility: 'Entries appear when learners reveal this line in intensive listening.' },
  'listening-question': { label: 'Listening · questions', visibility: 'Entries appear alongside the submitted answer explanations, never during blind listening or retest.' },
  'speaking-material': { label: 'Speaking · materials', visibility: 'Learners can look up entries before recording.' },
  'writing-topic': { label: 'Writing · topics', visibility: 'Entries appear after submission. Mark grammar as a hint to show it before sentence writing only.' },
  'writing-question': { label: 'Writing · questions', visibility: 'Entries appear after submission. Only sentence exercises allow grammar hints before submission.' },
} as const

type Kind = keyof typeof kinds
type Source = { id: string; title: string; context: string; jlpt_level: string; can_hint: boolean }
type Listing = { items: Source[]; has_more: boolean }

export default function PracticeLinksPage() {
  const [listing, setListing] = useState<Listing | null>(null)
  const [error, setError] = useState('')
  const [query, setQuery] = useState('')
  const [source, setSource] = useState<{ kind: Kind; item: Source } | null>(null)
  const [filter, setFilter] = useState<{ kind: Kind; query: string; page: number }>({ kind: 'listening-material', query: '', page: 1 })

  useEffect(() => {
    let active = true
    const params = new URLSearchParams({ search: filter.query, page: String(filter.page) })
    adminFetch<Listing>('GET', `/practice-links/${filter.kind}?${params}`).then(
      (data) => { if (active) setListing(data) },
      (reason: unknown) => { if (active) setError(reason instanceof Error ? reason.message : 'Could not load practice items.') },
    )
    return () => { active = false }
  }, [filter])

  function load(next: typeof filter) {
    setListing(null)
    setError('')
    setFilter(next)
  }

  function search(event: FormEvent<HTMLFormElement>) {
    event.preventDefault()
    load({ ...filter, query: query.trim(), page: 1 })
  }

  return (
    <div className="adm-page">
      <header className="adm-header">
        <div>
          <h1 className="adm-title">Practice links</h1>
          <p>Find a practice item, then choose related words and grammar. For daily or free translation, use <Link to="/translation">Translation</Link>.</p>
        </div>
      </header>
      <form className="adm-toolbar" onSubmit={search}>
        <select aria-label="Practice type" value={filter.kind}
          onChange={(event) => load({ kind: event.target.value as Kind, query: query.trim(), page: 1 })}>
          {Object.entries(kinds).map(([kind, item]) => <option value={kind} key={kind}>{item.label}</option>)}
        </select>
        <input type="search" aria-label="Find a practice item" placeholder="ID, title or text" maxLength={100} value={query} onChange={(event) => setQuery(event.target.value)} />
        <button className="adm-btn" type="submit">Find items</button>
      </form>
      {error && <p className="adm-error" role="alert">{error}</p>}
      {!listing && !error && <p className="adm-loading" role="status">Loading practice items…</p>}
      {listing && (
        <>
          <div className="adm-tableWrap">
            <table className="adm-table">
              <thead><tr><th>ID</th><th>Practice item</th><th>Context</th><th>JLPT</th><th>Actions</th></tr></thead>
              <tbody>{listing.items.length ? listing.items.map((item) => (
                <tr key={item.id}>
                  <td className="adm-id">{item.id}</td>
                  <td className="adm-textCell" title={item.title}>{item.title}</td>
                  <td className="adm-textCell" title={item.context}>{item.context || '—'}</td>
                  <td>{item.jlpt_level}</td>
                  <td className="adm-actions"><button type="button" onClick={() => setSource({ kind: filter.kind, item })}>🔗 Link entries</button></td>
                </tr>
              )) : <tr><td colSpan={5}>No matching practice items.</td></tr>}</tbody>
            </table>
          </div>
          <div className="adm-pagination">
            <button type="button" disabled={filter.page === 1} onClick={() => load({ ...filter, page: filter.page - 1 })}>← Previous</button>
            <span>Page {filter.page}</span>
            <button type="button" disabled={!listing.has_more} onClick={() => load({ ...filter, page: filter.page + 1 })}>Next →</button>
          </div>
        </>
      )}
      {source && <KnowledgeLinkEditor path={`/practice-links/${source.kind}/${encodeURIComponent(source.item.id)}`}
        sourceId={source.item.id} sourceText={source.item.title} description={kinds[source.kind].visibility}
        canHint={source.item.can_hint} onClose={() => setSource(null)} />}
    </div>
  )
}
