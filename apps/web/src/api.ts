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
  const token = getToken()
  if (token) headers.set('Authorization', `Bearer ${token}`)
  const res = await fetch(path, { ...init, headers })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(friendlyError(String(data.error || ''), res.status))
  return data as T
}

function friendlyError(raw: string, status: number): string {
  const msg = raw.trim()
  const lower = msg.toLowerCase()
  if (lower.includes('users_email') || (lower.includes('duplicate key') && lower.includes('email'))) {
    return 'This email is already registered. Sign in instead.'
  }
  if (lower.includes('duplicate key') || lower.includes('sqlstate') || lower.includes('violates unique')) {
    return 'This value is already in use.'
  }
  if (lower.includes('sqlstate') || lower.includes('pq:') || lower.includes('violates')) {
    return 'Something went wrong. Please try again.'
  }
  if (msg) return msg
  if (status === 401) return 'Wrong email or password.'
  if (status === 403) return "You don't have access to this workspace."
  if (status === 404) return 'Not found.'
  return 'Something went wrong. Please try again.'
}

export function wsPath(suffix: string) {
  const ws = getWorkspaceId()
  return `/api/v1/workspaces/${ws}${suffix}`
}
