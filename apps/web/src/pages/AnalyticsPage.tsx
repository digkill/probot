import { useEffect, useState } from 'react'
import { api, wsPath } from '../api'
import { useI18n } from '../i18n'

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
  const { t } = useI18n()
  const [summary, setSummary] = useState<Summary | null>(null)
  const [advice, setAdvice] = useState('')
  const [msg, setMsg] = useState('')

  async function load() {
    setSummary(await api<Summary>(wsPath('/analytics/summary')))
  }

  useEffect(() => { load().catch((e) => setMsg(e.message)) }, [])

  async function pollStats() {
    await api(wsPath('/analytics/poll-stats'), { method: 'POST' })
    setMsg(t('analytics.polled'))
    setTimeout(() => load().catch(() => undefined), 2500)
  }

  async function advise() {
    setAdvice(t('analytics.thinking'))
    try {
      const res = await api<{ result: { text: string } }>(wsPath('/analytics/advise'), {
        method: 'POST',
        body: JSON.stringify({}),
      })
      setAdvice(res.result.text)
    } catch (e) {
      setAdvice(e instanceof Error ? e.message : t('err.failed'))
    }
  }

  const totals = summary?.totals

  return (
    <div className="space-y-6">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h2 className="text-2xl font-semibold">{t('analytics.title')}</h2>
          <p className="text-sm text-[var(--muted)]">{t('analytics.subtitle')}</p>
        </div>
        <div className="flex gap-2">
          <button onClick={pollStats} className="rounded-lg border border-[var(--line)] px-4 py-2 text-sm">{t('analytics.poll')}</button>
          <button onClick={advise} className="rounded-lg bg-[var(--accent)] text-black font-medium px-4 py-2 text-sm">{t('analytics.advise')}</button>
        </div>
      </div>
      {msg && <p className="text-[var(--accent2)] text-sm">{msg}</p>}

      <div className="grid md:grid-cols-3 gap-3">
        <Card label={t('analytics.reach')} value={totals?.reach ?? 0} />
        <Card label={t('analytics.engagement')} value={(totals?.likes ?? 0) + (totals?.comments ?? 0) + (totals?.shares ?? 0)} />
        <Card label={t('analytics.clicks')} value={totals?.short_link_clicks ?? 0} />
        <Card label={t('analytics.mentions')} value={totals?.mentions ?? 0} />
        <Card label={t('analytics.negative')} value={totals?.negative_mentions ?? 0} />
        <Card label={t('analytics.openNeg')} value={totals?.open_negative ?? 0} />
        <Card label={t('analytics.live')} value={summary?.publication_status?.live ?? 0} />
        <Card label={t('analytics.ok')} value={summary?.channel_health?.ok ?? 0} />
      </div>

      <div className="grid md:grid-cols-2 gap-4">
        <div className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5">
          <h3 className="font-medium mb-3">{t('analytics.pubStatus')}</h3>
          <ul className="text-sm text-[var(--muted)] space-y-1">
            {Object.entries(summary?.publication_status || {}).map(([k, v]) => (
              <li key={k} className="flex justify-between"><span>{t(`status.${k}`)}</span><span>{v}</span></li>
            ))}
            {!Object.keys(summary?.publication_status || {}).length && <li>{t('analytics.noData')}</li>}
          </ul>
        </div>
        <div className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5">
          <h3 className="font-medium mb-3">{t('analytics.health')}</h3>
          <ul className="text-sm text-[var(--muted)] space-y-1">
            {Object.entries(summary?.channel_health || {}).map(([k, v]) => (
              <li key={k} className="flex justify-between"><span>{t(`health.${k}`)}</span><span>{v}</span></li>
            ))}
            {!Object.keys(summary?.channel_health || {}).length && <li>{t('analytics.noChannels')}</li>}
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
