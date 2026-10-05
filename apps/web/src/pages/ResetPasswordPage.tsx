import { useState } from 'react'
import type { FormEvent } from 'react'
import { Link, useSearchParams } from 'react-router-dom'
import { api } from '../api'
import { LangSwitch, useI18n } from '../i18n'

export default function ResetPasswordPage() {
  const { t } = useI18n()
  const [params] = useSearchParams()
  const token = params.get('token') || ''
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [done, setDone] = useState(false)
  const [loading, setLoading] = useState(false)

  async function onSubmit(e: FormEvent) {
    e.preventDefault()
    setError('')
    if (password !== confirm) {
      setError(t('reset.mismatch'))
      return
    }
    setLoading(true)
    try {
      await api('/api/v1/auth/password/reset', {
        method: 'POST',
        body: JSON.stringify({ token, password }),
      })
      setDone(true)
    } catch (err) {
      setError(err instanceof Error ? err.message : t('err.generic'))
    } finally {
      setLoading(false)
    }
  }

  const input = 'mt-1 w-full rounded-lg bg-black/30 border border-[var(--line)] px-3 py-2'

  return (
    <div className="min-h-screen grid place-items-center px-4">
      <form onSubmit={onSubmit} className="w-full max-w-md rounded-2xl border border-[var(--line)] bg-[var(--card)] p-8 shadow-2xl">
        <div className="flex items-start justify-between mb-2">
          <h1 className="text-3xl font-semibold">
            <span className="text-[var(--accent)]">P</span>Robot
          </h1>
          <LangSwitch />
        </div>
        <div className="font-medium mb-6 mt-4">{t('reset.title')}</div>

        {!token ? (
          <p className="text-[var(--danger)] text-sm mb-3">{t('reset.noToken')}</p>
        ) : done ? (
          <p className="text-[var(--accent)] text-sm mb-3">{t('reset.done')}</p>
        ) : (
          <>
            <label className="block text-sm mb-3">
              <span className="text-[var(--muted)]">{t('reset.password')}</span>
              <input type="password" autoComplete="new-password" className={input} value={password} onChange={(e) => setPassword(e.target.value)} />
            </label>
            <label className="block text-sm mb-3">
              <span className="text-[var(--muted)]">{t('reset.confirm')}</span>
              <input type="password" autoComplete="new-password" className={input} value={confirm} onChange={(e) => setConfirm(e.target.value)} />
            </label>
            {error && <p className="text-[var(--danger)] text-sm mb-3">{error}</p>}
            <button disabled={loading} className="w-full rounded-lg bg-[var(--accent)] text-black font-semibold py-2.5 disabled:opacity-60">
              {loading ? '…' : t('reset.submit')}
            </button>
          </>
        )}

        <Link to="/login" className="block mt-4 text-sm text-[var(--muted)] hover:text-white">
          {done ? t('reset.toLogin') : t('login.back')}
        </Link>
      </form>
    </div>
  )
}
