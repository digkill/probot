import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { api, wsPath } from '../api'

type Playbook = {
  id: string
  name: string
  description: string
  steps: { platform_slug: string; delay_minutes: number; required: boolean }[]
}
type Campaign = { id: string; name: string; playbook: string }
type Content = { id: string; title: string }

export default function PlaybooksPage() {
  const [playbooks, setPlaybooks] = useState<Playbook[]>([])
  const [campaigns, setCampaigns] = useState<Campaign[]>([])
  const [content, setContent] = useState<Content[]>([])
  const [playbookId, setPlaybookId] = useState('launch_app')
  const [campaignId, setCampaignId] = useState('')
  const [contentId, setContentId] = useState('')
  const [msg, setMsg] = useState('')
  const [result, setResult] = useState('')

  useEffect(() => {
    Promise.all([
      api<Playbook[]>('/api/v1/playbooks'),
      api<Campaign[]>(wsPath('/campaigns')),
      api<Content[]>(wsPath('/content')),
    ]).then(([p, c, ct]) => {
      setPlaybooks(p || [])
      setCampaigns(c || [])
      setContent(ct || [])
      if (c?.[0]) setCampaignId(c[0].id)
      if (ct?.[0]) setContentId(ct[0].id)
    }).catch((e) => setMsg(e.message))
  }, [])

  async function apply(e: FormEvent) {
    e.preventDefault()
    if (!campaignId || !contentId) return setMsg('Need campaign + content')
    const res = await api<{
      publications: unknown[]
      skipped: string[]
      missing_required: string[]
    }>(wsPath(`/campaigns/${campaignId}/apply-playbook`), {
      method: 'POST',
      body: JSON.stringify({
        playbook_id: playbookId,
        content_id: contentId,
        enqueue: true,
      }),
    })
    setResult(
      `Queued ${res.publications?.length || 0} pubs · skipped: ${(res.skipped || []).join(', ') || '—'} · missing required: ${(res.missing_required || []).join(', ') || '—'}`,
    )
    setMsg('Playbook applied')
  }

  const selected = playbooks.find((p) => p.id === playbookId)

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-semibold">Playbooks</h2>
        <p className="text-sm text-[var(--muted)]">Amplification presets with delays and cross-link order.</p>
      </div>
      {msg && <p className="text-[var(--accent2)] text-sm">{msg}</p>}

      <div className="grid md:grid-cols-3 gap-3">
        {playbooks.map((p) => (
          <button
            key={p.id}
            type="button"
            onClick={() => setPlaybookId(p.id)}
            className={`text-left rounded-xl border p-4 ${playbookId === p.id ? 'border-[var(--accent)] bg-[var(--card)]' : 'border-[var(--line)] bg-[var(--card)]'}`}
          >
            <div className="font-medium">{p.name}</div>
            <div className="text-xs text-[var(--muted)] mt-1">{p.description}</div>
          </button>
        ))}
      </div>

      {selected && (
        <ol className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5 space-y-2 text-sm">
          {selected.steps.map((s, i) => (
            <li key={`${s.platform_slug}-${i}`} className="flex justify-between text-[var(--muted)]">
              <span>{i + 1}. {s.platform_slug}{s.required ? ' *' : ''}</span>
              <span>+{s.delay_minutes}m</span>
            </li>
          ))}
        </ol>
      )}

      <form onSubmit={apply} className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5 grid md:grid-cols-3 gap-3">
        <select className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={campaignId} onChange={(e) => setCampaignId(e.target.value)}>
          {campaigns.map((c) => <option key={c.id} value={c.id}>{c.name}</option>)}
        </select>
        <select className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={contentId} onChange={(e) => setContentId(e.target.value)}>
          {content.map((c) => <option key={c.id} value={c.id}>{c.title || c.id.slice(0, 8)}</option>)}
        </select>
        <button className="rounded-lg bg-[var(--accent)] text-black font-medium">Apply & enqueue</button>
      </form>
      {result && <p className="text-sm text-[var(--muted)]">{result}</p>}
    </div>
  )
}
