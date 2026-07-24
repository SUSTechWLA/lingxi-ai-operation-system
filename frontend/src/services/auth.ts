export interface AuthUser {
  id: string
  email: string
  nickname?: string
  avatarUrl?: string
  status: 'active' | 'disabled' | 'deleted'
  createdAt?: string
  updatedAt?: string
  lastLoginAt?: string
}

export interface AuthSession {
  user: AuthUser
  accessToken: string
  refreshToken: string
  expiresIn: number
}

/** Raw backend auth response — uses snake_case keys. */
interface RawAuthResponse {
  user: AuthUser
  access_token: string
  refresh_token: string
  expires_in: number
}

interface AuthEnvelope<T> {
  code: number
  message: string
  data: T
}

export class AuthHTTPError extends Error {
  readonly status: number

  constructor(message: string, status: number) {
    super(message)
    this.name = 'AuthHTTPError'
    this.status = status
  }
}

export function isDefinitiveAuthFailure(error: unknown): boolean {
  if (!error || typeof error !== 'object') return false
  const directStatus = 'status' in error && typeof error.status === 'number' ? error.status : undefined
  const response = 'response' in error && error.response && typeof error.response === 'object'
    ? error.response as { status?: unknown }
    : undefined
  const responseStatus = typeof response?.status === 'number' ? response.status : undefined
  return directStatus === 401 || directStatus === 403 || responseStatus === 401 || responseStatus === 403
}

function mapAuthResponse(raw: RawAuthResponse): AuthSession {
  return {
    user: raw.user,
    accessToken: raw.access_token,
    refreshToken: raw.refresh_token,
    expiresIn: raw.expires_in,
  }
}

const configuredCloudBase = import.meta.env.VITE_CLOUD_API_BASE || import.meta.env.VITE_API_BASE
const electronCloudBase = typeof window !== 'undefined' ? window.electronAPI?.runtimeConfig?.cloudApiBase : ''
const API_BASE = configuredCloudBase || electronCloudBase || '/api'
const STORAGE_KEY = 'tangying.auth.session'

let cachedSession: AuthSession | null = readStoredSession()

export function getStoredAuthSession(): AuthSession | null {
  if (cachedSession) return cachedSession
  cachedSession = readStoredSession()
  return cachedSession
}

export function getAuthAccessToken(): string | null {
  return getStoredAuthSession()?.accessToken || null
}

export function getAuthRefreshToken(): string | null {
  return getStoredAuthSession()?.refreshToken || null
}

export async function register(email: string, password: string, nickname: string): Promise<AuthSession> {
  const raw = await authRequest<RawAuthResponse>('/auth/register', {
    email,
    password,
    nickname: nickname.trim() || undefined,
  })
  const session = mapAuthResponse(raw)
  storeSession(session)
  return session
}

export async function login(email: string, password: string): Promise<AuthSession> {
  const raw = await authRequest<RawAuthResponse>('/auth/login', { email, password })
  const session = mapAuthResponse(raw)
  storeSession(session)
  return session
}

export async function refreshAuthSession(): Promise<AuthSession> {
  const refreshToken = getAuthRefreshToken()
  if (!refreshToken) throw new Error('登录已过期')
  const raw = await authRequest<RawAuthResponse>('/auth/refresh', { refresh_token: refreshToken })
  const session = mapAuthResponse(raw)
  storeSession(session)
  return session
}

export async function fetchCurrentUser(): Promise<AuthUser> {
  const user = await authedGet<AuthUser>('/auth/me')
  const current = getStoredAuthSession()
  if (current) storeSession({ ...current, user })
  return user
}

export async function logoutRemote(): Promise<void> {
  const session = getStoredAuthSession()
  if (!session) return
  try {
    await fetch(cloudUrl('/auth/logout'), {
      method: 'POST',
      headers: {
        'Content-Type': 'application/json',
        Authorization: `Bearer ${session.accessToken}`,
      },
      body: JSON.stringify({ refresh_token: session.refreshToken }),
    })
  } finally {
    logout()
  }
}

export function logout(): void {
  cachedSession = null
  if (typeof window !== 'undefined') {
    window.localStorage.removeItem(STORAGE_KEY)
    void window.electronAPI?.clearLocalRunnerSession?.()
  }
}

function storeSession(session: AuthSession): void {
  cachedSession = session
  if (typeof window !== 'undefined') {
    window.localStorage.setItem(STORAGE_KEY, JSON.stringify(session))
    syncLocalRunnerSession(session)
  }
}

function readStoredSession(): AuthSession | null {
  if (typeof window === 'undefined') return null
  try {
    const raw = window.localStorage.getItem(STORAGE_KEY)
    if (!raw) return null
    return JSON.parse(raw) as AuthSession
  } catch {
    window.localStorage.removeItem(STORAGE_KEY)
    return null
  }
}

async function authedGet<T>(path: string): Promise<T> {
  const session = getStoredAuthSession()
  if (!session) throw new Error('请先登录')
  let response = await fetch(cloudUrl(path), {
    headers: { Authorization: `Bearer ${session.accessToken}` },
  })
  if (response.status === 401 && session.refreshToken) {
    const refreshed = await refreshAuthSession()
    response = await fetch(cloudUrl(path), {
      headers: { Authorization: `Bearer ${refreshed.accessToken}` },
    })
  }
  if (!response.ok) throw new AuthHTTPError(await errorMessage(response, '请求失败'), response.status)
  const envelope = await response.json() as AuthEnvelope<T>
  return envelope.data
}

async function authRequest<T>(path: string, payload: Record<string, unknown>): Promise<T> {
  const response = await fetch(cloudUrl(path), {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      DeviceID: getDeviceID(),
    },
    body: JSON.stringify(payload),
  })
  if (!response.ok) throw new AuthHTTPError(await errorMessage(response, '认证失败'), response.status)
  const envelope = await response.json() as AuthEnvelope<T>
  return envelope.data
}

function cloudUrl(path: string): string {
  return `${API_BASE.replace(/\/$/, '')}${path}`
}

export function getDeviceID(): string {
  const key = 'tangying.device.id'
  if (typeof window === 'undefined') return 'web'
  const existing = window.localStorage.getItem(key)
  if (existing) return existing
  const next = `web-${crypto.randomUUID?.() || Date.now().toString(36)}`
  window.localStorage.setItem(key, next)
  return next
}

function syncLocalRunnerSession(session: AuthSession): void {
  if (typeof window === 'undefined') return
  const api = window.electronAPI
  if (!api?.configureLocalRunnerSession) return
  void api.configureLocalRunnerSession({
    userToken: session.accessToken,
    deviceID: getDeviceID(),
  })
}

async function errorMessage(response: Response, fallback: string): Promise<string> {
  try {
    const body = await response.json() as { message?: string, error?: string }
    return body.message || body.error || fallback
  } catch {
    return fallback
  }
}
