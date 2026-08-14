import { useEffect, useState } from 'react'
import { api, wsPath } from '../api'

type Summary = {
  totals: {
    reach: number
    likes: number
    comments: number
    shares: number
    short_link_clicks: number
    mentions: number
    negative_mentions?: number
    open_negative?: number
    escalated_mentions?: number
  }
  publication_status: Record<string, number>
  channel_health: Record<string, number>
}

export default function AnalyticsPage() {
  const [summary, setSummary] = useState<Summary | null>(null)
  const [advice, setAdvice] = useState('')
  const [msg, setMsg] = useState('')

  async function load() {
    setSummary(await api<Summary>(wsPath('/analytics/summary')))
  }

  useEffect(() => { load().catch((e) => setMsg(e.message)) }, [])

  async function pollStats() {
    await api(wsPath('/analytics/poll-stats'), { method: 'POST' })
    setMsg('Stats poll enqueued — ensure worker is running')
    setTimeout(() => load().catch(() => undefined), 2500)
  }

  async function advise() {
    setAdvice('Thinking…')
    try {
      const res = await api<{ result: { text: string } }>(wsPath('/analytics/advise'), {
        method: 'POST',
        body: JSON.stringify({}),
      })
      setAdvice(res.result.text)
    } catch (e) {
      setAdvice(e instanceof Error ? e.message : 'failed')
    }
  }

  const t = summary?.totals

  return (
    <div className="space-y-6">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h2 className="text-2xl font-semibold">Analytics</h2>
          <p className="text-sm text-[var(--muted)]">Reach, clicks, health — plus AI next moves.</p>
        </div>
        <div className="flex gap-2">
          <button onClick={pollStats} className="rounded-lg border border-[var(--line)] px-4 py-2 text-sm">Poll stats</button>
          <button onClick={advise} className="rounded-lg bg-[var(--accent)] text-black font-medium px-4 py-2 text-sm">Ask advisor</button>
        </div>
      </div>
      {msg && <p className="text-[var(--accent2)] text-sm">{msg}</p>}

      <div className="grid md:grid-cols-3 gap-3">
        <Card label="Reach" value={t?.reach ?? 0} />
        <Card label="Engagement" value={(t?.likes ?? 0) + (t?.comments ?? 0) + (t?.shares ?? 0)} />
        <Card label="Short-link clicks" value={t?.short_link_clicks ?? 0} />
        <Card label="Mentions" value={t?.mentions ?? 0} />
        <Card label="Negative reviews" value={t?.negative_mentions ?? 0} />
        <Card label="Open negative" value={t?.open_negative ?? 0} />
        <Card label="Live pubs" value={summary?.publication_status?.live ?? 0} />
        <Card label="Channels ok" value={summary?.channel_health?.ok ?? 0} />
      </div>

      <div className="grid md:grid-cols-2 gap-4">
        <div className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5">
          <h3 className="font-medium mb-3">Publication status</h3>
          <ul className="text-sm text-[var(--muted)] space-y-1">
            {Object.entries(summary?.publication_status || {}).map(([k, v]) => (
              <li key={k} className="flex justify-between"><span>{k}</span><span>{v}</span></li>
            ))}
            {!Object.keys(summary?.publication_status || {}).length && <li>No data</li>}
          </ul>
        </div>
        <div className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5">
          <h3 className="font-medium mb-3">Channel health</h3>
          <ul className="text-sm text-[var(--muted)] space-y-1">
            {Object.entries(summary?.channel_health || {}).map(([k, v]) => (
              <li key={k} className="flex justify-between"><span>{k}</span><span>{v}</span></li>
            ))}
            {!Object.keys(summary?.channel_health || {}).length && <li>No channels</li>}
          </ul>
        </div>
      </div>

      {advice && (
        <pre className="whitespace-pre-wrap text-sm rounded-xl border border-[var(--line)] bg-black/30 p-5">{advice}</pre>
      )}
    </div>
  )
}

function Card({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5">
      <div className="text-sm text-[var(--muted)]">{label}</div>
      <div className="text-3xl font-semibold mt-1">{value}</div>
    </div>
  )
}
