import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { api, wsPath } from '../api'

type Mention = {
  id: string
  brand_id?: string
  source: string
  url: string
  title: string
  snippet: string
  status: string
  sentiment: string
  severity: string
  draft_reply?: string
  found_at?: string
}

type Brand = { id: string; name: string; slug: string; canonical_url: string }
type CrawlSource = { id: string; kind: string; query: string; purpose: string; url: string; enabled: boolean; last_run_at?: string }
type Inbox = { total: number; negative: number; open_negative: number; escalated: number; high: number }

export default function MentionsPage() {
  const [mentions, setMentions] = useState<Mention[]>([])
  const [brands, setBrands] = useState<Brand[]>([])
  const [channels, setChannels] = useState<{ id: string; name: string }[]>([])
  const [sources, setSources] = useState<CrawlSource[]>([])
  const [inbox, setInbox] = useState<Inbox | null>(null)
  const [providers, setProviders] = useState<{ id: string; name: string; configured: boolean }[]>([])
  const [brandId, setBrandId] = useState('')
  const [filter, setFilter] = useState<'open' | 'negative' | 'high' | 'escalated' | 'all'>('open')
  const [url, setUrl] = useState('https://hnrss.org/frontpage')
  const [query, setQuery] = useState('')
  const [msg, setMsg] = useState('')
  const [busy, setBusy] = useState('')
  const autoPicked = useRef(false)

  function queryString() {
    const p = new URLSearchParams()
    if (brandId) p.set('brand_id', brandId)
    if (filter === 'open') {
      p.set('sentiment', 'negative')
      p.set('open', '1')
    } else if (filter === 'negative') {
      p.set('sentiment', 'negative')
    } else if (filter === 'high') {
      p.set('severity', 'high')
    } else if (filter === 'escalated') {
      p.set('status', 'escalated')
    }
    const s = p.toString()
    return s ? `?${s}` : ''
  }

  async function load() {
    const inboxQs = brandId ? `?brand_id=${brandId}` : ''
    const [m, ch, b, src, box, prov] = await Promise.all([
      api<Mention[]>(wsPath(`/mentions${queryString()}`)),
      api<{ id: string; name: string }[]>(wsPath('/channels')),
      api<Brand[]>(wsPath('/brands')),
      api<CrawlSource[]>(wsPath('/crawl-sources')),
      api<Inbox>(wsPath(`/mentions/summary${inboxQs}`)),
      api<{ id: string; name: string; configured: boolean }[]>(wsPath('/ai/providers')).catch(() => []),
    ])
    setMentions(m || [])
    setChannels(ch || [])
    setBrands(b || [])
    setSources(src || [])
    setInbox(box)
    setProviders(prov || [])
    if (!autoPicked.current && b?.[0]) {
      autoPicked.current = true
      setBrandId(b[0].id)
    }
  }

  useEffect(() => { load().catch((e) => setMsg(e.message)) }, [filter, brandId])

  async function addSource(e: FormEvent) {
    e.preventDefault()
    const res = await api<{ id: string }>(wsPath('/crawl-sources'), {
      method: 'POST',
      body: JSON.stringify({ kind: 'rss', url, query, interval_sec: 3600, brand_id: brandId || undefined }),
    })
    await api(wsPath(`/crawl-sources/${res.id}/run`), { method: 'POST' })
    setMsg('Crawl queued')
    setTimeout(() => load().catch(() => undefined), 2000)
  }

  async function addManual(e: FormEvent) {
    e.preventDefault()
    await api(wsPath('/mentions'), {
      method: 'POST',
      body: JSON.stringify({
        source: 'manual',
        url: url || 'https://example.com',
        title: query ? query.slice(0, 80) : 'Manual mention',
        snippet: query || 'Added by hand',
        brand_id: brandId || undefined,
      }),
    })
    await load()
  }

  async function watchBrand() {
    if (!brandId) {
      setMsg('Create a brand first (Overview)')
      return
    }
    setBusy('watch')
    setMsg('Seeding review crawlers…')
    try {
      const res = await api<{ created: number; enqueued: number; watch_query: string }>(
        wsPath(`/brands/${brandId}/review-watch`),
        { method: 'POST' },
      )
      setMsg(`Watching “${res.watch_query}”: ${res.enqueued} crawls queued`)
      setTimeout(() => load().catch(() => undefined), 3000)
    } catch (e) {
      setMsg(e instanceof Error ? e.message : 'failed')
    } finally {
      setBusy('')
    }
  }

  async function draftReply(id: string) {
    setBusy(id)
    setMsg('Generating reply…')
    try {
      const res = await api<{ via?: string }>(wsPath(`/mentions/${id}/draft-reply`), {
        method: 'POST',
        body: JSON.stringify({ brand_id: brandId || undefined }),
      })
      setMsg(res.via === 'template' ? 'Template reply ready (no AI key)' : 'Draft reply ready')
      await load()
    } catch (e) {
      setMsg(e instanceof Error ? e.message : 'failed')
    } finally {
      setBusy('')
    }
  }

  async function objection(id: string) {
    setBusy(id)
    setMsg('Writing objection reply…')
    try {
      const res = await api<{ via?: string }>(wsPath(`/mentions/${id}/objection`), {
        method: 'POST',
        body: JSON.stringify({ brand_id: brandId || undefined }),
      })
      setMsg(res.via === 'ai' ? 'Objection draft ready — review before posting' : 'Template objection ready — review before posting')
      await load()
    } catch (e) {
      setMsg(e instanceof Error ? e.message : 'failed')
    } finally {
      setBusy('')
    }
  }

  async function setStatus(id: string, status: string) {
    await api(wsPath(`/mentions/${id}/status`), { method: 'POST', body: JSON.stringify({ status }) })
    await load()
  }

  const brandName = (id?: string) => brands.find((b) => b.id === id)?.name
  const watchSources = sources.filter((s) => s.purpose === 'reviews' && s.enabled !== false)
  const filters = [
    { id: 'open' as const, label: `Open negative (${inbox?.open_negative ?? 0})` },
    { id: 'negative' as const, label: `All negative (${inbox?.negative ?? 0})` },
    { id: 'high' as const, label: `High (${inbox?.high ?? 0})` },
    { id: 'escalated' as const, label: `Escalated (${inbox?.escalated ?? 0})` },
    { id: 'all' as const, label: `All (${inbox?.total ?? 0})` },
  ]

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-semibold">Reputation inbox</h2>
        <p className="text-sm text-[var(--muted)]">
          Collect negative reviews by brand name and site, then handle objections before they spread.
        </p>
      </div>
      {msg && <p className="text-[var(--accent2)] text-sm">{msg}</p>}

      <div className="grid md:grid-cols-4 gap-3">
        <Stat label="Open negative" value={inbox?.open_negative ?? 0} hot />
        <Stat label="High severity" value={inbox?.high ?? 0} hot={(inbox?.high ?? 0) > 0} />
        <Stat label="Escalated" value={inbox?.escalated ?? 0} />
        <Stat label="Watch sources" value={watchSources.length} />
      </div>

      <div className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5 space-y-3">
        <div className="flex flex-wrap items-end gap-3">
          <label className="text-sm">
            <div className="text-[var(--muted)] mb-1">Brand</div>
            <select
              className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2 min-w-[16rem]"
              value={brandId}
              onChange={(e) => setBrandId(e.target.value)}
            >
              <option value="">All brands</option>
              {brands.map((b) => (
                <option key={b.id} value={b.id}>{b.name} {b.canonical_url ? `· ${b.canonical_url}` : ''}</option>
              ))}
            </select>
          </label>
          <button disabled={busy === 'watch'} onClick={watchBrand} className="rounded-lg bg-[var(--accent)] text-black font-medium px-4 py-2 disabled:opacity-50">
            Watch reviews
          </button>
          <button
            disabled={busy === 'ai'}
            onClick={async () => {
              if (!brandId) {
                setMsg('Select a brand')
                return
              }
              setBusy('ai')
              try {
                await api(wsPath(`/brands/${brandId}/ai-research`), { method: 'POST' })
                setMsg('AI research queued (OpenAI / Claude / Grok / Gemini). Ensure worker or crawler is running.')
                setTimeout(() => load().catch(() => undefined), 4000)
              } catch (e) {
                setMsg(e instanceof Error ? e.message : 'failed')
              } finally {
                setBusy('')
              }
            }}
            className="rounded-lg border border-[var(--line)] px-4 py-2 text-sm"
          >
            AI research
          </button>
          <button
            onClick={async () => {
              await api(wsPath('/crawl-sources/run-due'), { method: 'POST' })
              setMsg('Due crawls queued')
              setTimeout(() => load().catch(() => undefined), 2500)
            }}
            className="rounded-lg border border-[var(--line)] px-4 py-2 text-sm"
          >
            Run due now
          </button>
        </div>
        <p className="text-xs text-[var(--muted)]">
          Public crawlers plus AI research via OpenAI, Claude (Anthropic), Grok (xAI), Gemini. Seed research agents on the Agents page and add API keys to `.env`.
          {' '}
          {providers.map((p) => (
            <span key={p.id} className={p.configured ? 'text-[var(--accent)] mr-2' : 'text-[var(--muted)] mr-2'}>
              {p.name}{p.configured ? ' ✓' : ''}
            </span>
          ))}
        </p>
        {watchSources.length > 0 && (
          <ul className="text-xs text-[var(--muted)] grid md:grid-cols-2 gap-1">
            {watchSources.map((s) => (
              <li key={s.id}>{s.kind}{s.last_run_at ? ` · last ${new Date(s.last_run_at).toLocaleString()}` : ' · never run'}</li>
            ))}
          </ul>
        )}
      </div>

      <div className="flex flex-wrap gap-2 text-sm">
        {filters.map((f) => (
          <button
            key={f.id}
            onClick={() => setFilter(f.id)}
            className={`rounded-lg px-3 py-1.5 border ${filter === f.id ? 'border-[var(--accent)] text-[var(--accent)]' : 'border-[var(--line)] text-[var(--muted)]'}`}
          >
            {f.label}
          </button>
        ))}
      </div>

      <form onSubmit={addSource} className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5 grid md:grid-cols-3 gap-3">
        <input className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2 md:col-span-2" value={url} onChange={(e) => setUrl(e.target.value)} placeholder="RSS URL or mention URL" />
        <input className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={query} onChange={(e) => setQuery(e.target.value)} placeholder="filter / complaint text" />
        <button className="md:col-span-2 rounded-lg bg-[var(--accent)] text-black font-medium py-2">Add RSS & crawl</button>
        <button type="button" onClick={addManual} className="rounded-lg border border-[var(--line)] py-2">Add manual mention</button>
      </form>

      <ul className="space-y-3">
        {mentions.map((m) => (
          <li key={m.id} className={`rounded-xl border bg-[var(--card)] p-4 ${m.sentiment === 'negative' ? 'border-red-500/40' : 'border-[var(--line)]'}`}>
            <div className="flex items-start justify-between gap-3">
              <div>
                <a className="font-medium text-[var(--accent)]" href={m.url} target="_blank" rel="noreferrer">{m.title || m.url}</a>
                <div className="text-xs text-[var(--muted)] mt-1 flex flex-wrap gap-2">
                  <span>{m.source} · {m.status}</span>
                  {brandName(m.brand_id) && <span>{brandName(m.brand_id)}</span>}
                  <span className={m.sentiment === 'negative' ? 'text-red-400' : ''}>{m.sentiment}</span>
                  {m.severity !== 'none' && <span>severity: {m.severity}</span>}
                  {m.found_at && <span>{new Date(m.found_at).toLocaleString()}</span>}
                </div>
              </div>
              <div className="flex flex-wrap gap-2 justify-end shrink-0">
                <button disabled={busy === m.id} onClick={() => objection(m.id)} className="text-sm text-red-300">Objection</button>
                <button disabled={busy === m.id} onClick={() => draftReply(m.id)} className="text-sm text-[var(--accent)]">Reply</button>
                {m.status !== 'escalated' && <button onClick={() => setStatus(m.id, 'escalated')} className="text-sm text-[var(--accent2)]">Escalate</button>}
                {m.status !== 'ignored' && <button onClick={() => setStatus(m.id, 'ignored')} className="text-sm text-[var(--muted)]">Ignore</button>}
                {m.status !== 'replied' && m.draft_reply && <button onClick={() => setStatus(m.id, 'replied')} className="text-sm text-[var(--muted)]">Mark replied</button>}
                {m.draft_reply && (
                  <button
                    onClick={() => navigator.clipboard.writeText(m.draft_reply || '')}
                    className="text-sm text-[var(--muted)]"
                  >
                    Copy
                  </button>
                )}
                {channels[0] && (
                  <button
                    onClick={async () => {
                      await api(wsPath(`/mentions/${m.id}/engage`), {
                        method: 'POST',
                        body: JSON.stringify({ channel_id: channels[0].id, kind: 'comment', brand_id: brandId || undefined }),
                      })
                      setMsg('Added to Karma queue for human approval')
                    }}
                    className="text-sm text-[var(--accent2)]"
                  >
                    To karma
                  </button>
                )}
              </div>
            </div>
            <p className="text-sm mt-2 text-[var(--muted)]">{m.snippet}</p>
            {m.draft_reply && (
              <pre className="mt-3 whitespace-pre-wrap text-sm rounded-lg bg-black/30 border border-[var(--line)] p-3">{m.draft_reply}</pre>
            )}
          </li>
        ))}
        {!mentions.length && (
          <li className="text-[var(--muted)] text-sm">
            Empty inbox. Watch a brand or add a mention. Need a content/reputation agent on <Link className="text-[var(--accent)]" to="/agents">Agents</Link> for AI drafts (templates still work).
          </li>
        )}
      </ul>
    </div>
  )
}

function Stat({ label, value, hot }: { label: string; value: number; hot?: boolean }) {
  return (
    <div className={`rounded-xl border p-4 ${hot ? 'border-red-500/40 bg-red-500/5' : 'border-[var(--line)] bg-[var(--card)]'}`}>
      <div className="text-[var(--muted)] text-sm">{label}</div>
      <div className="text-2xl font-semibold mt-1">{value}</div>
    </div>
  )
}
