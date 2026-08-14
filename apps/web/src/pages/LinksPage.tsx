import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { api, wsPath } from '../api'

type Link = {
  id: string
  code: string
  target_url: string
  label: string
  clicks: number
  short_url: string
}

export default function LinksPage() {
  const [links, setLinks] = useState<Link[]>([])
  const [target, setTarget] = useState('https://example.com')
  const [label, setLabel] = useState('landing')
  const [msg, setMsg] = useState('')

  async function load() {
    setLinks(await api<Link[]>(wsPath('/short-links')) || [])
  }

  useEffect(() => { load().catch((e) => setMsg(e.message)) }, [])

  async function create(e: FormEvent) {
    e.preventDefault()
    const res = await api<{ short_url: string }>(wsPath('/short-links'), {
      method: 'POST',
      body: JSON.stringify({ target_url: target, label }),
    })
    setMsg(`Created ${res.short_url}`)
    await load()
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-semibold">Short links</h2>
        <p className="text-sm text-[var(--muted)]">Click hub for honest CTR on cross-links.</p>
      </div>
      {msg && <p className="text-[var(--accent2)] text-sm">{msg}</p>}
      <form onSubmit={create} className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5 grid md:grid-cols-3 gap-3">
        <input className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2 md:col-span-2" value={target} onChange={(e) => setTarget(e.target.value)} placeholder="Target URL" />
        <input className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={label} onChange={(e) => setLabel(e.target.value)} placeholder="Label" />
        <button className="md:col-span-3 rounded-lg bg-[var(--accent)] text-black font-medium py-2">Create short link</button>
      </form>
      <div className="rounded-xl border border-[var(--line)] overflow-hidden">
        <table className="w-full text-sm">
          <thead className="bg-black/30 text-[var(--muted)]">
            <tr>
              <th className="text-left px-4 py-3">Short</th>
              <th className="text-left px-4 py-3">Target</th>
              <th className="text-left px-4 py-3">Clicks</th>
            </tr>
          </thead>
          <tbody>
            {links.map((l) => (
              <tr key={l.id} className="border-t border-[var(--line)]">
                <td className="px-4 py-3">
                  <a className="text-[var(--accent)]" href={l.short_url} target="_blank" rel="noreferrer">{l.short_url}</a>
                  <div className="text-xs text-[var(--muted)]">{l.label}</div>
                </td>
                <td className="px-4 py-3 truncate max-w-xs text-[var(--muted)]">{l.target_url}</td>
                <td className="px-4 py-3 font-medium">{l.clicks}</td>
              </tr>
            ))}
            {!links.length && (
              <tr><td colSpan={3} className="px-4 py-6 text-[var(--muted)]">No links yet</td></tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}
