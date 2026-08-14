import { useEffect, useState } from 'react'
import type { FormEvent } from 'react'
import { api, wsPath } from '../api'

type Agent = {
  id: string
  name: string
  role: string
  provider?: string
  model: string
  system_prompt: string
  enabled: boolean
}

type Provider = { id: string; name: string; model: string; configured: boolean }

const roles = ['content', 'platform_adapt', 'image', 'analytics_advisor', 'reputation', 'research']

export default function AgentsPage() {
  const [agents, setAgents] = useState<Agent[]>([])
  const [providers, setProviders] = useState<Provider[]>([])
  const [name, setName] = useState('Copywriter')
  const [role, setRole] = useState('content')
  const [provider, setProvider] = useState('openai')
  const [prompt, setPrompt] = useState('You are a sharp marketing copywriter. Keep posts concrete and CTA-driven.')
  const [runPrompt, setRunPrompt] = useState('Write a short launch post for a developer tool.')
  const [result, setResult] = useState('')
  const [msg, setMsg] = useState('')

  async function load() {
    const [a, p] = await Promise.all([
      api<Agent[]>(wsPath('/agents')),
      api<Provider[]>(wsPath('/ai/providers')).catch(() => []),
    ])
    setAgents(a || [])
    setProviders(p || [])
  }

  useEffect(() => { load().catch((e) => setMsg(e.message)) }, [])

  async function createAgent(e: FormEvent) {
    e.preventDefault()
    await api(wsPath('/agents'), {
      method: 'POST',
      body: JSON.stringify({
        name,
        role,
        provider,
        system_prompt: prompt,
      }),
    })
    setMsg('Agent created')
    await load()
  }

  async function seedDefaults() {
    const defaults = [
      { name: 'Copywriter', role: 'content', system_prompt: 'Write punchy social posts with clear CTA.' },
      { name: 'Adapter', role: 'platform_adapt', system_prompt: 'Adapt text to platform limits and tone. Return only final text.' },
      { name: 'Art director', role: 'image', system_prompt: 'Clean product marketing illustration, no text overlays.' },
      { name: 'Growth advisor', role: 'analytics_advisor', system_prompt: 'Given metrics and graph, suggest next amplification moves.' },
      { name: 'Reputation', role: 'reputation', system_prompt: 'Handle negative reviews and objections. Acknowledge, stay factual, offer one next step, match the user language. Never threaten or spam.' },
    ]
    for (const d of defaults) {
      await api(wsPath('/agents'), {
        method: 'POST',
        body: JSON.stringify({ ...d, provider: 'openai' }),
      }).catch(() => undefined)
    }
    const res = await api<{ created: number }>(wsPath('/agents/seed-research'), { method: 'POST' })
    setMsg(`Defaults seeded, ${res.created} research agents (OpenAI/Claude/Grok/Gemini)`)
    await load()
  }

  async function runAgent(id: string) {
    setResult('Running…')
    try {
      const res = await api<{ result: { text: string; image_url?: string } }>(wsPath(`/agents/${id}/run`), {
        method: 'POST',
        body: JSON.stringify({ prompt: runPrompt }),
      })
      setResult(res.result.image_url || res.result.text)
    } catch (e) {
      setResult(e instanceof Error ? e.message : 'failed')
    }
  }

  return (
    <div className="space-y-6">
      <div className="flex items-end justify-between gap-4">
        <div>
          <h2 className="text-2xl font-semibold">AI Agents</h2>
          <p className="text-sm text-[var(--muted)]">OpenAI, Claude/Anthropic, Grok/xAI, Gemini. Keys from `.env`.</p>
        </div>
        <button onClick={seedDefaults} className="rounded-lg border border-[var(--line)] px-4 py-2 text-sm">Seed defaults</button>
      </div>
      {msg && <p className="text-[var(--accent2)] text-sm">{msg}</p>}

      <div className="flex flex-wrap gap-2 text-xs">
        {providers.map((p) => (
          <span key={p.id} className={`rounded-lg border px-2 py-1 ${p.configured ? 'border-[var(--accent)] text-[var(--accent)]' : 'border-[var(--line)] text-[var(--muted)]'}`}>
            {p.name} {p.configured ? 'ready' : 'no key'}
          </span>
        ))}
      </div>

      <form onSubmit={createAgent} className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5 space-y-3">
        <div className="grid md:grid-cols-4 gap-3">
          <input className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={name} onChange={(e) => setName(e.target.value)} />
          <select className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={role} onChange={(e) => setRole(e.target.value)}>
            {roles.map((r) => <option key={r} value={r}>{r}</option>)}
          </select>
          <select className="rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={provider} onChange={(e) => setProvider(e.target.value)}>
            {providers.map((p) => <option key={p.id} value={p.id}>{p.name}</option>)}
            {!providers.length && (
              <>
                <option value="openai">OpenAI</option>
                <option value="anthropic">Claude</option>
                <option value="grok">Grok</option>
                <option value="gemini">Gemini</option>
              </>
            )}
          </select>
          <button className="rounded-lg bg-[var(--accent)] text-black font-medium">Create agent</button>
        </div>
        <textarea className="w-full min-h-24 rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={prompt} onChange={(e) => setPrompt(e.target.value)} />
      </form>

      <div className="rounded-xl border border-[var(--line)] bg-[var(--card)] p-5 space-y-3">
        <textarea className="w-full min-h-20 rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={runPrompt} onChange={(e) => setRunPrompt(e.target.value)} />
        <div className="grid gap-2">
          {agents.map((a) => (
            <div key={a.id} className="flex items-center justify-between gap-3 border border-[var(--line)] rounded-lg px-3 py-2">
              <div>
                <div className="font-medium">{a.name}</div>
                <div className="text-xs text-[var(--muted)]">{a.role} · {a.provider || 'openai'} · {a.model}</div>
              </div>
              <button onClick={() => runAgent(a.id)} className="text-sm text-[var(--accent)]">Run</button>
            </div>
          ))}
        </div>
        {result && (
          <pre className="whitespace-pre-wrap text-sm bg-black/30 rounded-lg p-4 border border-[var(--line)]">{result}</pre>
        )}
      </div>
    </div>
  )
}
