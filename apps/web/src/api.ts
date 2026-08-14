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
  if (!res.ok) throw new Error(data.error || res.statusText)
  return data as T
}

export function wsPath(suffix: string) {
  const ws = getWorkspaceId()
  return `/api/v1/workspaces/${ws}${suffix}`
}
