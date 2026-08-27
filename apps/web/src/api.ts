import { currentLocale, localizeKnownError, translate } from './i18n'

const TOKEN_KEY = 'probot_token'
const WS_KEY = 'probot_workspace'

export function getToken() {
  return localStorage.getItem(TOKEN_KEY) || ''
}

export function setToken(token: string) {
  localStorage.setItem(TOKEN_KEY, token)
}

export function getWorkspaceId() {
  return localStorage.getItem(WS_KEY) || ''
}

export function setWorkspaceId(id: string) {
  localStorage.setItem(WS_KEY, id)
}

export async function api<T>(path: string, init: RequestInit = {}): Promise<T> {
  const headers = new Headers(init.headers || {})
  headers.set('Content-Type', 'application/json')
  headers.set('Accept-Language', currentLocale() === 'ru' ? 'ru' : 'en')
  const token = getToken()
  if (token) headers.set('Authorization', `Bearer ${token}`)
  const res = await fetch(path, { ...init, headers })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) {
    if (res.status === 401 && token) {
      setToken('')
      setWorkspaceId('')
      if (location.pathname !== '/login') location.href = '/login'
    }
    throw new Error(friendlyError(String(data.error || ''), res.status))
  }
  return data as T
}

function friendlyError(raw: string, status: number): string {
  const locale = currentLocale()
  const msg = raw.trim()
  const lower = msg.toLowerCase()
  if (lower.includes('users_email') || (lower.includes('duplicate key') && lower.includes('email'))) {
    return translate(locale, 'err.emailTaken')
  }
  if (lower.includes('duplicate key') || lower.includes('sqlstate') || lower.includes('violates unique')) {
    return translate(locale, 'err.valueTaken')
  }
  if (lower.includes('sqlstate') || lower.includes('pq:') || lower.includes('violates')) {
    return translate(locale, 'err.generic')
  }
  if (msg) return localizeKnownError(msg, locale)
  if (status === 401) return translate(locale, 'err.wrongPass')
  if (status === 403) return translate(locale, 'err.forbidden')
  if (status === 404) return translate(locale, 'err.notFound')
  return translate(locale, 'err.generic')
}

export function wsPath(suffix: string) {
  const ws = getWorkspaceId()
  return `/api/v1/workspaces/${ws}${suffix}`
}
