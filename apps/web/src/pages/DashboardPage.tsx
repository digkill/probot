import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { Link } from 'react-router-dom'
import { api, wsPath } from '../api'

type Brand = { id: string; name: string; slug: string; canonical_url: string }
type Campaign = { id: string; name: string; playbook: string; status: string }
type Publication = { id: string; status: string; external_url: string; sort_order: number }

export default function DashboardPage() {
  const [brands, setBrands] = useState<Brand[]>([])
  const [campaigns, setCampaigns] = useState<Campaign[]>([])
  const [pubs, setPubs] = useState<Publication[]>([])
  const [name, setName] = useState('My Product')
  const [slug, setSlug] = useState('my-product')
  const [url, setUrl] = useState('https://example.com')
  const [tone, setTone] = useState('clear, confident, no hype')
  const [err, setErr] = useState('')
  const [inbox, setInbox] = useState({ open_negative: 0, high: 0, escalated: 0 })

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
        cta_default: 'Try it now',
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
        name: 'Launch wave',
        playbook: 'launch_app',
        status: 'active',
      }),
    })
    await load()
  }

  return (
    <div className="space-y-8">
      <section>
        <h2 className="text-2xl font-semibold mb-1">Overview</h2>
        <p className="text-[var(--muted)] text-sm">Brands, campaigns, publication pipeline.</p>
      </section>
      {err && <p className="text-[var(--danger)]">{err}</p>}

      <div className="grid md:grid-cols-3 gap-4">
        <Stat label="Brands" value={brands.length} />
        <Stat label="Campaigns" value={campaigns.length} />
        <Stat label="Publications" value={pubs.length} />
      </div>
      {(inbox.open_negative > 0 || inbox.high > 0) && (
        <Link to="/mentions" className="block rounded-xl border border-red-500/40 bg-red-500/5 p-4">
          <div className="font-medium">Reputation alert</div>
          <p className="text-sm text-[var(--muted)] mt-1">
            {inbox.open_negative} open negative mentions · {inbox.high} high severity · {inbox.escalated} escalated. Handle objections now.
          </p>
        </Link>
      )}

      <form onSubmit={createBrand} className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5 grid md:grid-cols-4 gap-3 items-end">
        <Field label="Brand name" value={name} onChange={setName} />
        <Field label="Slug" value={slug} onChange={setSlug} />
        <Field label="Canonical URL" value={url} onChange={setUrl} />
        <Field label="Tone of voice" value={tone} onChange={setTone} />
        <button className="rounded-lg bg-[var(--accent)] text-black font-medium py-2">Add brand</button>
        <button type="button" onClick={saveBrandBrain} className="rounded-lg border border-[var(--line)] py-2">Save brand brain</button>
      </form>

      <div className="flex gap-3">
        <button onClick={createCampaign} className="rounded-lg border border-[var(--line)] px-4 py-2 text-sm hover:border-[var(--accent)]">
          Create launch campaign
        </button>
      </div>

      <div className="grid md:grid-cols-2 gap-6">
        <List title="Brands" items={brands.map((b) => `${b.name} · ${b.slug}`)} />
        <List title="Campaigns" items={campaigns.map((c) => `${c.name} · ${c.playbook || '—'} · ${c.status}`)} />
      </div>

      <List title="Publications" items={pubs.map((p) => `#${p.sort_order} ${p.status} ${p.external_url || ''}`)} />
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

function List({ title, items }: { title: string; items: string[] }) {
  return (
    <div className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5">
      <h3 className="font-medium mb-3">{title}</h3>
      <ul className="space-y-2 text-sm text-[var(--muted)]">
        {items.length === 0 && <li>Empty</li>}
        {items.map((item) => <li key={item} className="truncate">{item}</li>)}
      </ul>
    </div>
  )
}
