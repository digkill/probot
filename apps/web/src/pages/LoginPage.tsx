import { useState } from 'react'
import type { FormEvent } from 'react'
import { useNavigate } from 'react-router-dom'
import { api, setToken, setWorkspaceId } from '../api'
import { LangSwitch, useI18n } from '../i18n'

export default function LoginPage() {
  const nav = useNavigate()
  const { t } = useI18n()
  const [mode, setMode] = useState<'login' | 'register' | 'forgot'>('login')
  const [email, setEmail] = useState('')
  const [password, setPassword] = useState('')
  const [workspaceSlug, setWorkspaceSlug] = useState('mediarise')
  const [error, setError] = useState('')
  const [notice, setNotice] = useState('')
  const [loading, setLoading] = useState(false)

  function switchMode(next: 'login' | 'register' | 'forgot') {
    setMode(next)
    setError('')
    setNotice('')
  }

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setLoading(true)
    setError('')
    setNotice('')
    try {
      if (mode === 'forgot') {
        await api('/api/v1/auth/password/forgot', { method: 'POST', body: JSON.stringify({ email }) })
        setNotice(t('forgot.sent'))
        return
      }
      if (mode === 'register') {
        const res = await api<{ token: string; workspace: { id: string } }>('/api/v1/auth/register', {
          method: 'POST',
          body: JSON.stringify({
            email,
            password,
            name: 'Owner',
            workspace_name: workspaceSlug,
            workspace_slug: workspaceSlug,
          }),
        })
        setToken(res.token)
        setWorkspaceId(res.workspace.id)
      } else {
        const res = await api<{ token: string; workspaces: { id: string }[] }>('/api/v1/auth/login', {
          method: 'POST',
          body: JSON.stringify({ email, password }),
        })
        setToken(res.token)
        if (!res.workspaces?.length) throw new Error(t('login.noWorkspaces'))
        setWorkspaceId(res.workspaces[0].id)
      }
      nav('/')
    } catch (err) {
      const msg = err instanceof Error ? err.message : t('err.generic')
      setError(msg)
      if (msg.toLowerCase().includes('зарегистрирован') || msg.toLowerCase().includes('already registered')) {
        setMode('login')
      }
    } finally {
      setLoading(false)
    }
  }

  return (
    <div className="min-h-screen grid place-items-center px-4">
      <form onSubmit={onSubmit} className="w-full max-w-md rounded-2xl border border-[var(--line)] bg-[var(--card)] p-8 shadow-2xl">
        <div className="flex items-start justify-between mb-2">
          <h1 className="text-3xl font-semibold">
            <span className="text-[var(--accent)]">P</span>Robot
          </h1>
          <LangSwitch />
        </div>
        <p className="text-[var(--muted)] mb-6 text-sm">{t('login.tagline')}</p>
        {mode === 'forgot' ? (
          <div className="mb-6 text-sm">
            <div className="font-medium text-base">{t('forgot.title')}</div>
            <p className="text-[var(--muted)] mt-1">{t('forgot.hint')}</p>
          </div>
        ) : (
          <div className="flex gap-2 mb-6 text-sm">
            <button type="button" onClick={() => switchMode('register')} className={mode === 'register' ? 'text-[var(--accent)]' : 'text-[var(--muted)]'}>{t('login.register')}</button>
            <button type="button" onClick={() => switchMode('login')} className={mode === 'login' ? 'text-[var(--accent)]' : 'text-[var(--muted)]'}>{t('login.login')}</button>
          </div>
        )}
        <label className="block text-sm mb-3">
          <span className="text-[var(--muted)]">{t('login.email')}</span>
          <input className="mt-1 w-full rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={email} onChange={(e) => setEmail(e.target.value)} />
        </label>
        {mode !== 'forgot' && (
          <label className="block text-sm mb-3">
            <span className="text-[var(--muted)]">{t('login.password')}</span>
            <input type="password" className="mt-1 w-full rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={password} onChange={(e) => setPassword(e.target.value)} />
          </label>
        )}
        {mode === 'register' && (
          <label className="block text-sm mb-3">
            <span className="text-[var(--muted)]">{t('login.workspace')}</span>
            <input className="mt-1 w-full rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2" value={workspaceSlug} onChange={(e) => setWorkspaceSlug(e.target.value)} />
          </label>
        )}
        {error && <p className="text-[var(--danger)] text-sm mb-3">{error}</p>}
        {notice && <p className="text-[var(--accent)] text-sm mb-3">{notice}</p>}
        <button disabled={loading} className="w-full rounded-lg bg-[var(--accent)] text-black font-semibold py-2.5 disabled:opacity-60">
          {loading ? '…' : mode === 'register' ? t('login.create') : mode === 'forgot' ? t('forgot.send') : t('login.signin')}
        </button>
        {mode === 'login' && (
          <button type="button" onClick={() => switchMode('forgot')} className="mt-4 text-sm text-[var(--muted)] hover:text-white">
            {t('login.forgot')}
          </button>
        )}
        {mode === 'forgot' && (
          <button type="button" onClick={() => switchMode('login')} className="mt-4 text-sm text-[var(--muted)] hover:text-white">
            {t('login.back')}
          </button>
        )}
      </form>
    </div>
  )
}
