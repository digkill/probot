import { useEffect, useRef, useState } from 'react'
import type { FormEvent } from 'react'
import Papa from 'papaparse'
import { api, wsPath } from '../../api'
import { useI18n } from '../../i18n'

type Contact = { id: string; email: string; phone: string; name: string; tags: string[]; status: string; created_at: string }
type Stats = Record<'total' | 'active' | 'unsubscribed' | 'bounced' | 'complained', number>
type ImportState = { file: string; progress: number; rows: number; created: number; updated: number; invalid: number; running: boolean; error?: string }

const STATUSES = ['active', 'unsubscribed', 'bounced', 'complained'] as const
const PAGE = 50
const BATCH = 2000
const input = 'rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2'

async function sendBatch(columns: string[], rows: string[][]) {
  for (let attempt = 0; ; attempt++) {
    try {
      return await api<{ created: number; updated: number; invalid: number }>(wsPath('/contacts/import'), {
        method: 'POST',
        body: JSON.stringify({ columns, rows }),
      })
    } catch (e) {
      if (attempt >= 2) throw e
      await new Promise((r) => setTimeout(r, 1500 * (attempt + 1)))
    }
  }
}

export default function ContactsPage() {
  const { t, locale } = useI18n()
  const [items, setItems] = useState<Contact[]>([])
  const [total, setTotal] = useState(0)
  const [stats, setStats] = useState<Stats | null>(null)
  const [tags, setTags] = useState<{ tag: string; count: number }[]>([])
  const [query, setQuery] = useState('')
  const [debounced, setDebounced] = useState('')
  const [status, setStatus] = useState('')
  const [tag, setTag] = useState('')
  const [offset, setOffset] = useState(0)
  const [msg, setMsg] = useState('')
  const [showAdd, setShowAdd] = useState(false)
  const [form, setForm] = useState({ email: '', name: '', phone: '', tags: '' })
  const [confirmDelete, setConfirmDelete] = useState('')
  const [imp, setImp] = useState<ImportState | null>(null)
  const cancelRef = useRef(false)
  const fileRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    const id = setTimeout(() => setDebounced(query.trim()), 300)
    return () => clearTimeout(id)
  }, [query])
  useEffect(() => { setOffset(0) }, [debounced, status, tag])

  async function loadList() {
    const p = new URLSearchParams({ limit: String(PAGE), offset: String(offset) })
    if (debounced) p.set('q', debounced)
    if (status) p.set('status', status)
    if (tag) p.set('tag', tag)
    const res = await api<{ items: Contact[]; total: number }>(wsPath(`/contacts?${p}`))
    setItems(res.items)
    setTotal(res.total)
  }

  async function loadMeta() {
    const [s, tg] = await Promise.all([api<Stats>(wsPath('/contacts/stats')), api<{ tag: string; count: number }[]>(wsPath('/contacts/tags'))])
    setStats(s)
    setTags(tg)
  }

  async function reload() {
    await Promise.all([loadList(), loadMeta()])
  }

  useEffect(() => { loadList().catch((e) => setMsg(e.message)) }, [debounced, status, tag, offset])
  useEffect(() => { loadMeta().catch((e) => setMsg(e.message)) }, [])

  async function addContact(e: FormEvent) {
    e.preventDefault()
    try {
      await api(wsPath('/contacts'), {
        method: 'POST',
        body: JSON.stringify({ email: form.email, name: form.name, phone: form.phone, tags: form.tags.split(',') }),
      })
      setForm({ email: '', name: '', phone: '', tags: '' })
      setShowAdd(false)
      setMsg(t('contacts.added'))
      await reload()
    } catch (err) {
      setMsg(err instanceof Error ? err.message : t('err.failed'))
    }
  }

  async function changeStatus(c: Contact, next: string) {
    try {
      await api(wsPath(`/contacts/${c.id}`), { method: 'PATCH', body: JSON.stringify({ status: next }) })
      setItems((list) => list.map((x) => (x.id === c.id ? { ...x, status: next } : x)))
      await loadMeta()
    } catch (err) {
      setMsg(err instanceof Error ? err.message : t('err.failed'))
    }
  }

  async function remove(id: string) {
    setConfirmDelete('')
    try {
      await api(wsPath(`/contacts/${id}`), { method: 'DELETE' })
      await reload()
    } catch (err) {
      setMsg(err instanceof Error ? err.message : t('err.failed'))
    }
  }

  async function exportCsv() {
    try {
      const res = await api<{ url: string }>(wsPath('/contacts/export-link'), { method: 'POST' })
      window.location.href = res.url
    } catch (err) {
      setMsg(err instanceof Error ? err.message : t('err.failed'))
    }
  }

  function startImport(file: File) {
    cancelRef.current = false
    let columns: string[] | null = null
    const pending: string[][] = []
    const state: ImportState = { file: file.name, progress: 0, rows: 0, created: 0, updated: 0, invalid: 0, running: true }
    setImp({ ...state })
    const flush = async (rows: string[][]) => {
      if (!columns || !rows.length) return
      const r = await sendBatch(columns, rows)
      state.rows += rows.length
      state.created += r.created
      state.updated += r.updated
      state.invalid += r.invalid
    }
    const finish = async (error?: string) => {
      state.running = false
      state.error = error
      if (!error) state.progress = 1
      setImp({ ...state })
      await reload().catch(() => undefined)
    }
    Papa.parse<string[]>(file, {
      skipEmptyLines: 'greedy',
      chunkSize: 2 * 1024 * 1024,
      chunk: (results, parser) => {
        parser.pause()
        ;(async () => {
          let rows = results.data
          if (!columns) {
            columns = rows[0] ?? []
            rows = rows.slice(1)
          }
          pending.push(...rows)
          while (pending.length >= BATCH) await flush(pending.splice(0, BATCH))
          state.progress = Math.min(results.meta.cursor / file.size, 0.99)
          setImp({ ...state })
          if (cancelRef.current) {
            parser.abort()
            await finish(t('contacts.importCancelled'))
            return
          }
          parser.resume()
        })().catch(async (err) => {
          parser.abort()
          await finish(err instanceof Error ? err.message : t('err.failed'))
        })
      },
      complete: (results) => {
        if (results.meta.aborted) return
        flush(pending.splice(0))
          .then(() => finish())
          .catch((err) => finish(err instanceof Error ? err.message : t('err.failed')))
      },
      error: (err) => { finish(err.message) },
    })
  }

  const fmt = (n?: number) => (n ?? 0).toLocaleString(locale)
  const cards: { key: keyof Stats; hot?: boolean }[] = [
    { key: 'total' }, { key: 'active' }, { key: 'unsubscribed' }, { key: 'bounced' }, { key: 'complained' },
  ]

  return (
    <div className="space-y-5">
      {msg && <p className="text-[var(--accent2)] text-sm">{msg}</p>}

      <div className="grid grid-cols-2 md:grid-cols-5 gap-3">
        {cards.map((c) => (
          <button
            key={c.key}
            onClick={() => setStatus(c.key === 'total' ? '' : c.key)}
            className={`text-left rounded-xl border p-4 bg-[var(--card)] ${status === (c.key === 'total' ? '' : c.key) ? 'border-[var(--accent)]' : 'border-[var(--line)]'}`}
          >
            <div className="text-[var(--muted)] text-sm">{t(`contacts.stat.${c.key}`)}</div>
            <div className="text-2xl font-semibold mt-1">{fmt(stats?.[c.key])}</div>
          </button>
        ))}
      </div>

      <div className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-4 flex flex-wrap items-center gap-3">
        <input className={`${input} min-w-[16rem] flex-1`} placeholder={t('contacts.search')} value={query} onChange={(e) => setQuery(e.target.value)} />
        <select className={input} value={status} onChange={(e) => setStatus(e.target.value)}>
          <option value="">{t('contacts.allStatuses')}</option>
          {STATUSES.map((s) => <option key={s} value={s}>{t(`contacts.status.${s}`)}</option>)}
        </select>
        <select className={input} value={tag} onChange={(e) => setTag(e.target.value)}>
          <option value="">{t('contacts.allTags')}</option>
          {tags.map((x) => <option key={x.tag} value={x.tag}>{x.tag} ({fmt(x.count)})</option>)}
        </select>
        <button onClick={() => setShowAdd((v) => !v)} className="rounded-lg border border-[var(--line)] px-4 py-2 text-sm">{t('contacts.add')}</button>
        <button disabled={!!imp?.running} onClick={() => fileRef.current?.click()} className="rounded-lg bg-[var(--accent)] text-black font-medium px-4 py-2 text-sm disabled:opacity-50">{t('contacts.import')}</button>
        <button onClick={exportCsv} className="rounded-lg border border-[var(--line)] px-4 py-2 text-sm">{t('contacts.export')}</button>
        <input
          ref={fileRef}
          type="file"
          accept=".csv,text/csv"
          className="hidden"
          onChange={(e) => {
            const f = e.target.files?.[0]
            e.target.value = ''
            if (f) startImport(f)
          }}
        />
      </div>

      {imp && (
        <div className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-4 space-y-2 text-sm">
          <div className="flex flex-wrap justify-between gap-2">
            <span>{t('contacts.importing', { file: imp.file })}</span>
            <span className="text-[var(--muted)]">
              {t('contacts.importStats', { rows: fmt(imp.rows), created: fmt(imp.created), updated: fmt(imp.updated), invalid: fmt(imp.invalid) })}
            </span>
          </div>
          <div className="h-2 rounded bg-black/40 overflow-hidden">
            <div className="h-full bg-[var(--accent)] transition-all" style={{ width: `${Math.round(imp.progress * 100)}%` }} />
          </div>
          <div className="flex items-center gap-3">
            {imp.running && <span className="text-[var(--muted)]">{Math.round(imp.progress * 100)}%</span>}
            {imp.running && <button onClick={() => { cancelRef.current = true }} className="text-red-300">{t('contacts.cancel')}</button>}
            {!imp.running && !imp.error && <span className="text-[var(--accent)]">{t('contacts.importDone')}</span>}
            {imp.error && <span className="text-[var(--danger)]">{imp.error}</span>}
          </div>
          <p className="text-xs text-[var(--muted)]">{t('contacts.importHint')}</p>
        </div>
      )}

      {showAdd && (
        <form onSubmit={addContact} className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-4 grid md:grid-cols-5 gap-3">
          <input required type="email" className={input} placeholder="Email" value={form.email} onChange={(e) => setForm({ ...form, email: e.target.value })} />
          <input className={input} placeholder={t('contacts.name')} value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} />
          <input className={input} placeholder={t('contacts.phone')} value={form.phone} onChange={(e) => setForm({ ...form, phone: e.target.value })} />
          <input className={input} placeholder={t('contacts.tagsPlaceholder')} value={form.tags} onChange={(e) => setForm({ ...form, tags: e.target.value })} />
          <button className="rounded-lg bg-[var(--accent)] text-black font-medium py-2">{t('contacts.save')}</button>
        </form>
      )}

      <div className="rounded-xl border border-[var(--line)] bg-[var(--card)] overflow-x-auto">
        <table className="w-full text-sm">
          <thead className="text-[var(--muted)] text-left">
            <tr className="border-b border-[var(--line)]">
              <th className="p-3 font-normal">Email</th>
              <th className="p-3 font-normal">{t('contacts.name')}</th>
              <th className="p-3 font-normal">{t('contacts.phone')}</th>
              <th className="p-3 font-normal">{t('contacts.tags')}</th>
              <th className="p-3 font-normal">{t('contacts.statusCol')}</th>
              <th className="p-3 font-normal">{t('contacts.addedAt')}</th>
              <th className="p-3" />
            </tr>
          </thead>
          <tbody>
            {items.map((c) => (
              <tr key={c.id} className="border-b border-[var(--line)] last:border-0">
                <td className="p-3 break-all">{c.email}</td>
                <td className="p-3">{c.name}</td>
                <td className="p-3 whitespace-nowrap">{c.phone}</td>
                <td className="p-3">
                  <div className="flex flex-wrap gap-1">
                    {c.tags.map((x) => (
                      <button key={x} onClick={() => setTag(x)} className="rounded border border-[var(--line)] px-1.5 text-xs text-[var(--muted)]">{x}</button>
                    ))}
                  </div>
                </td>
                <td className="p-3">
                  <select className="bg-transparent text-sm" value={c.status} onChange={(e) => changeStatus(c, e.target.value)}>
                    {STATUSES.map((s) => <option key={s} value={s}>{t(`contacts.status.${s}`)}</option>)}
                  </select>
                </td>
                <td className="p-3 whitespace-nowrap text-[var(--muted)]">{new Date(c.created_at).toLocaleDateString(locale)}</td>
                <td className="p-3 whitespace-nowrap text-right">
                  {confirmDelete === c.id ? (
                    <span className="flex gap-2 justify-end">
                      <button onClick={() => remove(c.id)} className="text-red-400">{t('mentions.yes')}</button>
                      <button onClick={() => setConfirmDelete('')} className="text-[var(--muted)]">{t('mentions.no')}</button>
                    </span>
                  ) : (
                    <button onClick={() => setConfirmDelete(c.id)} className="text-red-400">{t('mentions.delete')}</button>
                  )}
                </td>
              </tr>
            ))}
            {!items.length && (
              <tr><td colSpan={7} className="p-6 text-center text-[var(--muted)]">{t('contacts.empty')}</td></tr>
            )}
          </tbody>
        </table>
      </div>

      <div className="flex items-center justify-between text-sm text-[var(--muted)]">
        <span>{total ? t('contacts.range', { from: fmt(offset + 1), to: fmt(Math.min(offset + PAGE, total)), total: fmt(total) }) : ''}</span>
        <span className="flex gap-2">
          <button disabled={offset === 0} onClick={() => setOffset(Math.max(0, offset - PAGE))} className="rounded-lg border border-[var(--line)] px-3 py-1.5 disabled:opacity-40">←</button>
          <button disabled={offset + PAGE >= total} onClick={() => setOffset(offset + PAGE)} className="rounded-lg border border-[var(--line)] px-3 py-1.5 disabled:opacity-40">→</button>
        </span>
      </div>
    </div>
  )
}
