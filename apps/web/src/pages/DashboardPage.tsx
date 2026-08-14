import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { api, wsPath } from '../api'
import { useI18n } from '../i18n'

type Brand = { id: string; name: string; slug: string; canonical_url: string }
type Campaign = { id: string; name: string; playbook: string; status: string }
type Publication = { id: string; status: string; external_url: string; sort_order: number }

export default function DashboardPage() {
  const { t } = useI18n()
  const [brands, setBrands] = useState<Brand[]>([])
  const [campaigns, setCampaigns] = useState<Campaign[]>([])
  const [pubs, setPubs] = useState<Publication[]>([])
  const [name, setName] = useState('')
  const [slug, setSlug] = useState('my-product')
  const [url, setUrl] = useState('https://example.com')
  const [tone, setTone] = useState('')
  const [err, setErr] = useState('')
  const [inbox, setInbox] = useState({ open_negative: 0, high: 0, escalated: 0 })

  useEffect(() => {
    if (!name) setName(t('overview.defaultBrand'))
    if (!tone) setTone(t('overview.defaultTone'))
  }, [t])

  async function load() {
    const [b, c, p, box] = await Promise.all([
      api<Brand[]>(wsPath('/brands')),
      api<Campaign[]>(wsPath('/campaigns')),
      api<Publication[]>(wsPath('/publications')),
      api<{ open_negative: number; high: number; escalated: number }>(wsPath('/mentions/summary')).catch(() => ({ open_negative: 0, high: 0, escalated: 0 })),
    ])
    setBrands(b || [])
    setCampaigns(c || [])
    setPubs(p || [])
    setInbox(box)
  }

  useEffect(() => { load().catch((e) => setErr(e.message)) }, [])

  async function createBrand(e: FormEvent) {
    e.preventDefault()
    await api(wsPath('/brands'), {
      method: 'POST',
      body: JSON.stringify({ name, slug, canonical_url: url, tone_of_voice: tone }),
    })
    await load()
  }

  async function saveBrandBrain() {
    if (!brands[0]) return
    await api(wsPath(`/brands/${brands[0].id}`), {
      method: 'PATCH',
      body: JSON.stringify({
        name: brands[0].name,
        slug: brands[0].slug,
        canonical_url: url || brands[0].canonical_url,
        tone_of_voice: tone,
        cta_default: t('overview.defaultCta'),
      }),
    })
    setErr('')
    await load()
  }

  async function createCampaign() {
    if (!brands[0]) return
    await api(wsPath('/campaigns'), {
      method: 'POST',
      body: JSON.stringify({
        brand_id: brands[0].id,
        name: t('overview.defaultCampaign'),
        playbook: 'launch_app',
        status: 'active',
      }),
    })
    await load()
  }

  return (
    <div className="space-y-8">
      <section>
        <h2 className="text-2xl font-semibold mb-1">{t('overview.title')}</h2>
        <p className="text-[var(--muted)] text-sm">{t('overview.subtitle')}</p>
      </section>
      {err && <p className="text-[var(--danger)]">{err}</p>}

      <div className="grid md:grid-cols-3 gap-4">
        <Stat label={t('overview.brands')} value={brands.length} />
        <Stat label={t('overview.campaigns')} value={campaigns.length} />
        <Stat label={t('overview.publications')} value={pubs.length} />
      </div>
      {(inbox.open_negative > 0 || inbox.high > 0) && (
        <Link to="/mentions" className="block rounded-xl border border-red-500/40 bg-red-500/5 p-4">
          <div className="font-medium">{t('overview.alert')}</div>
          <p className="text-sm text-[var(--muted)] mt-1">
            {t('overview.alertBody', { neg: inbox.open_negative, high: inbox.high, esc: inbox.escalated })}
          </p>
        </Link>
      )}

      <form onSubmit={createBrand} className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5 grid md:grid-cols-4 gap-3 items-end">
        <Field label={t('overview.brandName')} value={name} onChange={setName} />
        <Field label={t('overview.slug')} value={slug} onChange={setSlug} />
        <Field label={t('overview.url')} value={url} onChange={setUrl} />
        <Field label={t('overview.tone')} value={tone} onChange={setTone} />
        <button className="rounded-lg bg-[var(--accent)] text-black font-medium py-2">{t('overview.addBrand')}</button>
        <button type="button" onClick={saveBrandBrain} className="rounded-lg border border-[var(--line)] py-2">{t('overview.saveBrain')}</button>
      </form>

      <div className="flex gap-3">
        <button onClick={createCampaign} className="rounded-lg border border-[var(--line)] px-4 py-2 text-sm hover:border-[var(--accent)]">
          {t('overview.createCampaign')}
        </button>
      </div>

      <div className="grid md:grid-cols-2 gap-6">
        <List title={t('overview.brands')} empty={t('overview.empty')} items={brands.map((b) => `${b.name} · ${b.slug}`)} />
        <List title={t('overview.campaigns')} empty={t('overview.empty')} items={campaigns.map((c) => `${c.name} · ${c.playbook || '—'} · ${t(`status.${c.status}`)}`)} />
      </div>

      <List title={t('overview.publications')} empty={t('overview.empty')} items={pubs.map((p) => `#${p.sort_order} ${t(`status.${p.status}`)} ${p.external_url || ''}`)} />
    </div>
  )
}

function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5">
      <div className="text-[var(--muted)] text-sm">{label}</div>
      <div className="text-3xl font-semibold mt-1">{value}</div>
    </div>
  )
}

function Field({ label, value, onChange }: { label: string; value: string; onChange: (v: string) => void }) {
  return (
    <label className="text-sm block">
      <span className="text-[var(--muted)]">{label}</span>
      <input className="mt-1 w-full rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={value} onChange={(e) => onChange(e.target.value)} />
    </label>
  )
}

function List({ title, items, empty }: { title: string; items: string[]; empty: string }) {
  return (
    <div className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5">
      <h3 className="font-medium mb-3">{title}</h3>
      <ul className="space-y-2 text-sm text-[var(--muted)]">
        {items.length === 0 && <li>{empty}</li>}
        {items.map((item) => <li key={item} className="truncate">{item}</li>)}
      </ul>
    </div>
  )
}
