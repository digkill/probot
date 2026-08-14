import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { api, wsPath } from '../api'
import { useI18n } from '../i18n'

type Brand = { id: string; name: string }
type Campaign = { id: string; name: string }
type Channel = { id: string; name: string }
type Content = { id: string; title: string; body: string; status: string }

export default function ContentPage() {
  const { t } = useI18n()
  const [brands, setBrands] = useState<Brand[]>([])
  const [campaigns, setCampaigns] = useState<Campaign[]>([])
  const [channels, setChannels] = useState<Channel[]>([])
  const [content, setContent] = useState<Content[]>([])
  const [title, setTitle] = useState('')
  const [body, setBody] = useState('')
  const [msg, setMsg] = useState('')

  useEffect(() => {
    if (!title) setTitle(t('content.defaultTitle'))
    if (!body) setBody(t('content.defaultBody'))
  }, [t])

  async function load() {
    const [b, c, ch, ct] = await Promise.all([
      api<Brand[]>(wsPath('/brands')),
      api<Campaign[]>(wsPath('/campaigns')),
      api<Channel[]>(wsPath('/channels')),
      api<Content[]>(wsPath('/content')),
    ])
    setBrands(b || [])
    setCampaigns(c || [])
    setChannels(ch || [])
    setContent(ct || [])
  }

  useEffect(() => { load().catch((e) => setMsg(e.message)) }, [])

  async function createContent(e: FormEvent) {
    e.preventDefault()
    if (!brands[0]) return setMsg(t('content.needBrand'))
    await api(wsPath('/content'), {
      method: 'POST',
      body: JSON.stringify({
        brand_id: brands[0].id,
        campaign_id: campaigns[0]?.id,
        title,
        body,
        cta_url: 'https://example.com',
        utm_source: 'probot',
        utm_medium: 'social',
        utm_campaign: campaigns[0]?.name || 'launch',
        status: 'ready',
      }),
    })
    setMsg(t('content.created'))
    await load()
  }

  async function scheduleAll() {
    const piece = content[0]
    if (!piece) return setMsg(t('content.noContent'))
    if (!channels.length) return setMsg(t('content.needChannels'))
    for (let i = 0; i < channels.length; i++) {
      await api(wsPath('/publications'), {
        method: 'POST',
        body: JSON.stringify({
          content_id: piece.id,
          channel_id: channels[i].id,
          campaign_id: campaigns[0]?.id,
          sort_order: i,
          enqueue: true,
        }),
      })
    }
    setMsg(t('content.enqueued', { n: channels.length }))
  }

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-semibold">{t('content.title')}</h2>
        <p className="text-sm text-[var(--muted)]">{t('content.subtitle')}</p>
      </div>
      {msg && <p className="text-[var(--accent2)] text-sm">{msg}</p>}
      <form onSubmit={createContent} className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5 space-y-3">
        <input className="w-full rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={title} onChange={(e) => setTitle(e.target.value)} placeholder={t('content.placeholderTitle')} />
        <textarea className="w-full min-h-32 rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={body} onChange={(e) => setBody(e.target.value)} />
        <div className="flex gap-3">
          <button className="rounded-lg bg-[var(--accent)] text-black font-medium px-4 py-2">{t('content.save')}</button>
          <button type="button" onClick={scheduleAll} className="rounded-lg border border-[var(--line)] px-4 py-2">{t('content.enqueue')}</button>
        </div>
      </form>
      <ul className="space-y-2 text-sm text-[var(--muted)]">
        {content.map((c) => (
          <li key={c.id} className="rounded-lg border border-[var(--line)] px-4 py-3">
            <div className="text-white font-medium">{c.title}</div>
            <div className="truncate">{c.body}</div>
          </li>
        ))}
      </ul>
    </div>
  )
}
