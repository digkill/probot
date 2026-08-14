import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { api, wsPath } from '../api'
import { useI18n } from '../i18n'

type Channel = { id: string; name: string; health: string }
type Task = {
  id: string
  kind: string
  status: string
  target_url: string
  target_title: string
  draft_text: string
  result_url: string
  points: number
  error_message: string
}
type Partner = {
  id: string
  name: string
  platform: string
  their_url: string
  our_url: string
  status: string
  notes: string
}
type Karma = {
  total_points: number
  today_points: number
  by_kind: Record<string, number>
  today_by_kind: Record<string, number>
  today_task_counts: Record<string, number>
}

export default function KarmaPage() {
  const { t } = useI18n()
  const [channels, setChannels] = useState<Channel[]>([])
  const [tasks, setTasks] = useState<Task[]>([])
  const [partners, setPartners] = useState<Partner[]>([])
  const [karma, setKarma] = useState<Karma | null>(null)
  const [kind, setKind] = useState('comment')
  const [channelId, setChannelId] = useState('')
  const [targetUrl, setTargetUrl] = useState('')
  const [title, setTitle] = useState('')
  const [partnerName, setPartnerName] = useState('')
  const [theirUrl, setTheirUrl] = useState('')
  const [ourUrl, setOurUrl] = useState('')
  const [msg, setMsg] = useState('')

  async function load() {
    const [ch, t, p, k] = await Promise.all([
      api<Channel[]>(wsPath('/channels')),
      api<Task[]>(wsPath('/engagement/tasks')),
      api<Partner[]>(wsPath('/engagement/partners')),
      api<Karma>(wsPath('/engagement/karma')),
    ])
    setChannels(ch || [])
    setTasks(t || [])
    setPartners(p || [])
    setKarma(k)
    if (!channelId && ch?.[0]) setChannelId(ch[0].id)
  }

  useEffect(() => { load().catch((e) => setMsg(e.message)) }, [])

  async function addTask(e: FormEvent) {
    e.preventDefault()
    if (!channelId) return setMsg(t('karma.needChannel'))
    await api(wsPath('/engagement/tasks'), {
      method: 'POST',
      body: JSON.stringify({
        channel_id: channelId,
        kind,
        target_url: targetUrl,
        target_title: title,
        approve: kind === 'like',
      }),
    })
    setTargetUrl('')
    setTitle('')
    setMsg(kind === 'like' ? t('karma.likeQueued') : t('karma.waiting'))
    await load()
  }

  async function addPartner(e: FormEvent) {
    e.preventDefault()
    await api(wsPath('/engagement/partners'), {
      method: 'POST',
      body: JSON.stringify({ name: partnerName, their_url: theirUrl, our_url: ourUrl, platform: 'web', status: 'outreach' }),
    })
    setPartnerName('')
    setTheirUrl('')
    setOurUrl('')
    await load()
  }

  async function draft(id: string) {
    setMsg(t('karma.drafting'))
    try {
      await api(wsPath(`/engagement/tasks/${id}/draft`), { method: 'POST' })
      setMsg(t('karma.draftReady'))
      await load()
    } catch (e) {
      setMsg(e instanceof Error ? e.message : t('err.failed'))
    }
  }

  async function approve(id: string) {
    await api(wsPath(`/engagement/tasks/${id}/approve`), { method: 'POST' })
    setMsg(t('karma.approved'))
    await load()
  }

  async function skip(id: string) {
    await api(wsPath(`/engagement/tasks/${id}/skip`), { method: 'POST' })
    await load()
  }

  async function confirm(id: string) {
    await api(wsPath(`/engagement/tasks/${id}/confirm`), {
      method: 'POST',
      body: JSON.stringify({ result_url: '' }),
    })
    await load()
  }

  async function markPartner(p: Partner, status: string) {
    await api(wsPath(`/engagement/partners/${p.id}`), {
      method: 'PATCH',
      body: JSON.stringify({ ...p, status }),
    })
    await load()
  }

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-semibold">{t('karma.title')}</h2>
        <p className="text-sm text-[var(--muted)]">
          {t('karma.subtitle')}
        </p>
      </div>
      {msg && <p className="text-[var(--accent2)] text-sm">{msg}</p>}

      <div className="grid md:grid-cols-3 gap-3">
        <Stat label={t('karma.total')} value={karma?.total_points ?? 0} />
        <Stat label={t('karma.today')} value={karma?.today_points ?? 0} />
        <Stat label={t('karma.commentsToday')} value={karma?.today_task_counts?.comment ?? 0} />
      </div>

      <form onSubmit={addTask} className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5 grid md:grid-cols-2 gap-3">
        <h3 className="md:col-span-2 font-medium">{t('karma.newTask')}</h3>
        <select className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={kind} onChange={(e) => setKind(e.target.value)}>
          <option value="like">{t('kind.like')}</option>
          <option value="comment">{t('kind.comment')}</option>
          <option value="follow">{t('kind.follow')}</option>
          <option value="share">{t('kind.share')}</option>
          <option value="link_exchange">{t('kind.link_exchange')}</option>
        </select>
        <select className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={channelId} onChange={(e) => setChannelId(e.target.value)}>
          {channels.map((c) => <option key={c.id} value={c.id}>{c.name} ({t(`health.${c.health}`)})</option>)}
        </select>
        <input className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2 md:col-span-2" placeholder={t('karma.target')} value={targetUrl} onChange={(e) => setTargetUrl(e.target.value)} />
        <input className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2 md:col-span-2" placeholder={t('karma.context')} value={title} onChange={(e) => setTitle(e.target.value)} />
        <button className="md:col-span-2 rounded-lg bg-[var(--accent)] text-black font-medium py-2">{t('karma.add')}</button>
      </form>

      <div className="space-y-3">
        <h3 className="font-medium">{t('karma.queue')}</h3>
        {tasks.map((item) => (
          <div key={item.id} className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-4 space-y-2">
            <div className="flex justify-between gap-3 text-sm">
              <span className="font-medium">{t(`kind.${item.kind}`)} · {t(`status.${item.status}`)} · +{item.points}</span>
              <a className="text-[var(--accent)] truncate max-w-[50%]" href={item.target_url} target="_blank" rel="noreferrer">{item.target_title || item.target_url}</a>
            </div>
            {item.draft_text && <pre className="whitespace-pre-wrap text-sm bg-black/30 rounded-lg p-3 border border-[var(--line)]">{item.draft_text}</pre>}
            {item.error_message && <p className="text-[var(--danger)] text-sm">{item.error_message}</p>}
            <div className="flex flex-wrap gap-2 text-sm">
              {(item.kind === 'comment' || item.kind === 'link_exchange') && item.status !== 'done' && (
                <button onClick={() => draft(item.id)} className="text-[var(--accent)]">{t('karma.aiDraft')}</button>
              )}
              {(item.status === 'pending_approval' || item.status === 'draft') && (
                <button onClick={() => approve(item.id)} className="text-[var(--accent)]">{t('karma.approve')}</button>
              )}
              {item.status === 'needs_manual' && (
                <button onClick={() => confirm(item.id)} className="text-[var(--accent2)]">{t('karma.manual')}</button>
              )}
              {item.status !== 'done' && item.status !== 'skipped' && (
                <button onClick={() => skip(item.id)} className="text-[var(--muted)]">{t('karma.skip')}</button>
              )}
            </div>
          </div>
        ))}
        {!tasks.length && <p className="text-sm text-[var(--muted)]">{t('karma.none')}</p>}
      </div>

      <form onSubmit={addPartner} className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5 grid md:grid-cols-2 gap-3">
        <h3 className="md:col-span-2 font-medium">{t('karma.partners')}</h3>
        <input className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" placeholder={t('karma.partnerName')} value={partnerName} onChange={(e) => setPartnerName(e.target.value)} />
        <input className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" placeholder={t('karma.theirUrl')} value={theirUrl} onChange={(e) => setTheirUrl(e.target.value)} />
        <input className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2 md:col-span-2" placeholder={t('karma.ourUrl')} value={ourUrl} onChange={(e) => setOurUrl(e.target.value)} />
        <button className="md:col-span-2 rounded-lg border border-[var(--line)] py-2">{t('karma.addPartner')}</button>
      </form>
      <ul className="space-y-2 text-sm">
        {partners.map((p) => (
          <li key={p.id} className="rounded-lg border border-[var(--line)] px-4 py-3 flex flex-wrap justify-between gap-2">
            <span>{p.name} · {t(`status.${p.status}`)} · {p.their_url}</span>
            <span className="flex gap-2">
              <button onClick={() => markPartner(p, 'we_linked')} className="text-[var(--accent)]">{t('karma.weLinked')}</button>
              <button onClick={() => markPartner(p, 'complete')} className="text-[var(--accent)]">{t('karma.complete')}</button>
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5">
      <div className="text-sm text-[var(--muted)]">{label}</div>
      <div className="text-3xl font-semibold mt-1">{value}</div>
    </div>
  )
}
