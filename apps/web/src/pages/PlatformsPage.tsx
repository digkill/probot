import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { api, wsPath } from '../api'

type Platform = { id: string; slug: string; name: string; kind: string; publish_mode: string; audience_fit: number }
type Brand = { id: string; name: string }
type Channel = { id: string; name: string; external_ref: string; health: string }

export default function PlatformsPage() {
  const [platforms, setPlatforms] = useState<Platform[]>([])
  const [brands, setBrands] = useState<Brand[]>([])
  const [channels, setChannels] = useState<Channel[]>([])
  const [slug, setSlug] = useState('telegram')
  const [name, setName] = useState('TG Main')
  const [externalRef, setExternalRef] = useState('')
  const [token, setToken] = useState('')
  const [customName, setCustomName] = useState('My forum')
  const [customSlug, setCustomSlug] = useState('my-forum')
  const [webhook, setWebhook] = useState('')
  const [msg, setMsg] = useState('')

  async function load() {
    const [p, b, c] = await Promise.all([
      api<Platform[]>(wsPath('/platforms')),
      api<Brand[]>(wsPath('/brands')),
      api<Channel[]>(wsPath('/channels')),
    ])
    setPlatforms(p || [])
    setBrands(b || [])
    setChannels(c || [])
  }

  useEffect(() => { load().catch((e) => setMsg(e.message)) }, [])

  async function connectChannel(e: FormEvent) {
    e.preventDefault()
    if (!brands[0]) return setMsg('Create brand first')
    await api(wsPath('/channels'), {
      method: 'POST',
      body: JSON.stringify({
        brand_id: brands[0].id,
        platform_slug: slug,
        name,
        external_ref: externalRef,
        token,
        meta: slug === 'bluesky' ? { handle: externalRef } : {},
      }),
    })
    setMsg('Channel connected')
    await load()
  }

  async function addCustom(e: FormEvent) {
    e.preventDefault()
    await api(wsPath('/custom-platforms'), {
      method: 'POST',
      body: JSON.stringify({
        name: customName,
        slug: customSlug,
        publish_mode: webhook ? 'webhook' : 'manual',
        webhook_url: webhook,
      }),
    })
    setMsg('Custom platform added')
  }

  return (
    <div className="space-y-8">
      <div>
        <h2 className="text-2xl font-semibold">Platforms</h2>
        <p className="text-sm text-[var(--muted)]">Catalog + channels + custom sinks.</p>
      </div>
      {msg && <p className="text-[var(--accent2)] text-sm">{msg}</p>}

      <div className="grid md:grid-cols-2 gap-3">
        {platforms.map((p) => (
          <div key={p.id} className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-4">
            <div className="font-medium">{p.name}</div>
            <div className="text-xs text-[var(--muted)] mt-1">{p.slug} · {p.kind} · {p.publish_mode} · fit {p.audience_fit}</div>
          </div>
        ))}
      </div>

      <form onSubmit={connectChannel} className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5 grid md:grid-cols-2 gap-3">
        <h3 className="md:col-span-2 font-medium">Connect channel</h3>
        <select className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={slug} onChange={(e) => setSlug(e.target.value)}>
          {platforms.map((p) => <option key={p.slug} value={p.slug}>{p.name}</option>)}
        </select>
        <input className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" placeholder="Channel name" value={name} onChange={(e) => setName(e.target.value)} />
        <input className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" placeholder="external ref (chat_id / owner_id / handle)" value={externalRef} onChange={(e) => setExternalRef(e.target.value)} />
        <input className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" placeholder="token / app password" value={token} onChange={(e) => setToken(e.target.value)} />
        <button className="md:col-span-2 rounded-lg bg-[var(--accent)] text-black font-medium py-2">Connect</button>
      </form>

      <form onSubmit={addCustom} className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5 grid md:grid-cols-3 gap-3">
        <h3 className="md:col-span-3 font-medium">Add custom platform</h3>
        <input className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={customName} onChange={(e) => setCustomName(e.target.value)} />
        <input className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={customSlug} onChange={(e) => setCustomSlug(e.target.value)} />
        <input className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" placeholder="webhook url (optional)" value={webhook} onChange={(e) => setWebhook(e.target.value)} />
        <button className="md:col-span-3 rounded-lg border border-[var(--line)] py-2">Save custom platform</button>
      </form>

      <div className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5">
        <h3 className="font-medium mb-3">Connected channels</h3>
        <ul className="text-sm text-[var(--muted)] space-y-2">
          {channels.map((c) => (
            <li key={c.id} className="flex items-center justify-between gap-3">
              <span>{c.name} · {c.external_ref} · <span className={c.health === 'paused' ? 'text-[var(--danger)]' : c.health === 'warn' ? 'text-[var(--accent2)]' : ''}>{c.health}</span></span>
              {c.health !== 'ok' && (
                <button
                  type="button"
                  className="text-[var(--accent)]"
                  onClick={async () => {
                    await api(wsPath(`/channels/${c.id}/health`), {
                      method: 'PATCH',
                      body: JSON.stringify({ health: 'ok' }),
                    })
                    setMsg('Channel resumed')
                    await load()
                  }}
                >
                  Resume
                </button>
              )}
            </li>
          ))}
          {!channels.length && <li>None yet</li>}
        </ul>
      </div>
    </div>
  )
}
