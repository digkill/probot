import { createContext, useCallback, useContext, useMemo, useState } from 'react'
import type { ReactNode } from 'react'
import { en, ru, type Locale } from './dict'

const LANG_KEY = 'probot_lang'
const dicts: Record<Locale, Record<string, string>> = { ru, en }

function readLocale(): Locale {
  try {
    const saved = localStorage.getItem(LANG_KEY)
    if (saved === 'ru' || saved === 'en') return saved
  } catch {
    /* ignore */
  }
  return 'ru'
}

export function currentLocale(): Locale {
  return readLocale()
}

export function translate(locale: Locale, key: string, vars?: Record<string, string | number>): string {
  let s = dicts[locale]?.[key] ?? dicts.en[key] ?? key
  if (vars) {
    for (const [k, v] of Object.entries(vars)) {
      s = s.replaceAll(`{${k}}`, String(v))
    }
  }
  return s
}

export function localizeKnownError(msg: string, locale: Locale): string {
  const table: [string, string][] = [
    ['This email is already registered', 'err.emailTaken'],
    ['Wrong email or password', 'err.wrongPass'],
    ['email already registered', 'err.emailTaken'],
    ['This workspace slug is already taken', 'err.slugTaken'],
    ['This value is already in use', 'err.valueTaken'],
    ["You don't have access", 'err.forbidden'],
    ['reset link is invalid', 'err.resetInvalid'],
    ['Password must be at least', 'err.passwordShort'],
    ['Password is too long', 'err.passwordLong'],
    ['Email is required', 'err.emailRequired'],
    ['Session expired', 'err.session'],
    ['Please sign in', 'err.session'],
    ['Not found', 'err.notFound'],
    ['Something went wrong', 'err.generic'],
    ['no workspaces', 'login.noWorkspaces'],
  ]
  const lower = msg.toLowerCase()
  for (const [needle, key] of table) {
    if (lower.includes(needle.toLowerCase())) return translate(locale, key)
  }
  return msg
}

type Ctx = {
  locale: Locale
  setLocale: (l: Locale) => void
  t: (key: string, vars?: Record<string, string | number>) => string
}

const I18nContext = createContext<Ctx | null>(null)

export function I18nProvider({ children }: { children: ReactNode }) {
  const [locale, setLocaleState] = useState<Locale>(readLocale)
  const setLocale = useCallback((l: Locale) => {
    setLocaleState(l)
    localStorage.setItem(LANG_KEY, l)
    document.documentElement.lang = l
  }, [])
  const t = useCallback((key: string, vars?: Record<string, string | number>) => translate(locale, key, vars), [locale])
  const value = useMemo(() => ({ locale, setLocale, t }), [locale, setLocale, t])
  return <I18nContext.Provider value={value}>{children}</I18nContext.Provider>
}

export function useI18n() {
  const ctx = useContext(I18nContext)
  if (!ctx) throw new Error('I18nProvider missing')
  return ctx
}

export function LangSwitch({ className = '' }: { className?: string }) {
  const { locale, setLocale } = useI18n()
  return (
    <div className={`flex gap-1 text-xs ${className}`}>
      <button type="button" onClick={() => setLocale('ru')} className={locale === 'ru' ? 'text-[var(--accent)]' : 'text-[var(--muted)]'}>
        RU
      </button>
      <span className="text-[var(--muted)]">/</span>
      <button type="button" onClick={() => setLocale('en')} className={locale === 'en' ? 'text-[var(--accent)]' : 'text-[var(--muted)]'}>
        EN
      </button>
    </div>
  )
}
