import type { DdnsConfig, DdnsProvider, DdnsRecord, NatTypeInfo, StunConfig, WebhookConfig, WebhookTemplate } from '../types'

interface ApiResponse<T> {
  code: number
  msg?: string
  data?: T
}

const TOKEN_KEY = 'linkstar_token'
const DESKTOP_SECRET_KEY = 'linkstar_desktop_secret'

// captureDesktopSecret 桌面版首屏：从 URL 读 desktop_secret 存入 sessionStorage 并抹掉地址栏
function captureDesktopSecret() {
  const params = new URLSearchParams(window.location.search)
  const secret = params.get('desktop_secret')
  if (secret) {
    sessionStorage.setItem(DESKTOP_SECRET_KEY, secret)
    params.delete('desktop_secret')
    const q = params.toString()
    const url = window.location.pathname + (q ? `?${q}` : '') + window.location.hash
    window.history.replaceState(null, '', url)
  }
}
captureDesktopSecret()

export const getToken = () => localStorage.getItem(TOKEN_KEY)
export const setToken = (t: string) => localStorage.setItem(TOKEN_KEY, t)
export const clearToken = () => localStorage.removeItem(TOKEN_KEY)
export const isDesktop = () => !!sessionStorage.getItem(DESKTOP_SECRET_KEY)

async function request<T>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  const headers: Record<string, string> = {
    'Content-Type': 'application/json',
    ...(init?.headers as Record<string, string> | undefined),
  }
  const token = getToken()
  if (token) headers['Authorization'] = `Bearer ${token}`
  const secret = sessionStorage.getItem(DESKTOP_SECRET_KEY)
  if (secret) headers['X-LinkStar-Desktop'] = secret

  const resp = await fetch(path, {
    ...init,
    headers,
  })
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
  const json = (await resp.json()) as ApiResponse<T>
  if (json.code === 401) {
    clearToken()
    if (!location.hash.startsWith('#/login')) location.hash = '#/login'
    throw new Error(json.msg || '未登录')
  }
  if (json.code === 428) {
    if (!location.hash.startsWith('#/setup')) location.hash = '#/setup'
    throw new Error(json.msg || '系统尚未初始化')
  }
  if (json.code !== 0) throw new Error(json.msg || '请求失败')
  return json.data as T
}

// ===================== Auth =====================

export const getAuthStatus = () =>
  request<{ initialized: boolean }>('/api/auth/status')

export const setupPassword = (password: string) =>
  request<{ token: string }>('/api/auth/setup', {
    method: 'POST',
    body: JSON.stringify({ password }),
  })

export const login = (password: string) =>
  request<{ token: string }>('/api/auth/login', {
    method: 'POST',
    body: JSON.stringify({ password }),
  })

export const changePassword = (oldPassword: string, newPassword: string) =>
  request<unknown>('/api/auth/password', {
    method: 'PUT',
    body: JSON.stringify({ oldPassword, newPassword }),
  })

export const getVersion = () => request<{ version: string }>('/api/version')

export const getStunConfig = () => request<StunConfig>('/api/stun/config')

export const getNatType = () => request<NatTypeInfo>('/api/stun/nat-type')

export const detectNatType = () => request<NatTypeInfo>('/api/stun/nat-type/detect', { method: 'POST' })

export interface SSEHandle {
  close: () => void
}

// subscribeStunStatus 用 fetch 流式读取 SSE，替代浏览器原生 EventSource——
// 因为受保护端点需要 Authorization/桌面 secret 头，而 EventSource 无法自定义请求头。
export function subscribeStunStatus(onEvent: (data: string) => void): SSEHandle {
  const controller = new AbortController()
  let closed = false
  let retry = 1000

  const run = async () => {
    while (!closed) {
      try {
        const headers: Record<string, string> = { Accept: 'text/event-stream' }
        const token = getToken()
        if (token) headers['Authorization'] = `Bearer ${token}`
        const secret = sessionStorage.getItem(DESKTOP_SECRET_KEY)
        if (secret) headers['X-LinkStar-Desktop'] = secret

        const resp = await fetch('/api/stun/status/events', {
          headers,
          signal: controller.signal,
        })
        if (resp.status === 401) {
          clearToken()
          if (!location.hash.startsWith('#/login')) location.hash = '#/login'
          return
        }
        if (!resp.ok || !resp.body) throw new Error(`HTTP ${resp.status}`)
        retry = 1000

        const reader = resp.body.getReader()
        const decoder = new TextDecoder()
        let buffer = ''
        while (!closed) {
          const { value, done } = await reader.read()
          if (done) break
          buffer += decoder.decode(value, { stream: true })
          let idx: number
          while ((idx = buffer.indexOf('\n\n')) >= 0) {
            const raw = buffer.slice(0, idx)
            buffer = buffer.slice(idx + 2)
            const data = raw
              .split('\n')
              .filter((l) => l.startsWith('data:'))
              .map((l) => l.slice(5).replace(/^ /, ''))
              .join('\n')
            if (data) onEvent(data)
          }
        }
      } catch {
        if (closed) return
      }
      if (closed) return
      await new Promise((r) => setTimeout(r, retry))
      retry = Math.min(retry * 2, 15000)
    }
  }
  run()

  return {
    close: () => {
      closed = true
      controller.abort()
    },
  }
}

export const addStunDevice = (body: { name: string; ip: string }) =>
  request<{ id: number }>('/api/stun/device/add', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateStunDevice = (body: { deviceId: number; name: string; ip: string }) =>
  request<unknown>('/api/stun/device/update', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const deleteStunDevice = (deviceId: number) =>
  request<unknown>('/api/stun/device/delete', {
    method: 'DELETE',
    body: JSON.stringify({ deviceId }),
  })

export interface StunServicePayload {
  deviceId: number
  name: string
  internalPort: number
  protocol: string
  upnpMappedPort: number
  useUpnp: boolean
  https: boolean
  enabled: boolean
  description: string
  webhookconfig?: WebhookConfig
}

export const addStunService = (body: StunServicePayload) =>
  request<{ id: number }>('/api/stun/service/add', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateStunService = (body: StunServicePayload & { serviceId: number }) =>
  request<unknown>('/api/stun/service/update', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const deleteStunService = (deviceId: number, serviceId: number) =>
  request<unknown>('/api/stun/service/delete', {
    method: 'DELETE',
    body: JSON.stringify({ deviceId, serviceId }),
  })

export const setStunShowOnHome = (deviceId: number, serviceId: number, show: boolean) =>
  request<unknown>('/api/stun/service/show-on-home', {
    method: 'PUT',
    body: JSON.stringify({ deviceId, serviceId, show }),
  })

export interface HomeApp {
  id: string
  type: string
}

export const getHomeConfig = () =>
  request<{ apps: HomeApp[] }>('/api/home/config')

// ===================== DDNS =====================

export const getDdnsConfig = () => request<DdnsConfig>('/api/ddns/config')

export const updateDdnsSettings = (body: { intervalSec: number }) =>
  request<unknown>('/api/ddns/settings', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const addDdnsProvider = (body: { name: string; type: string; credential: Record<string, string> }) =>
  request<DdnsProvider>('/api/ddns/provider/add', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateDdnsProvider = (body: { id: number; name: string; credential: Record<string, string> }) =>
  request<unknown>('/api/ddns/provider/update', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const deleteDdnsProvider = (id: number) =>
  request<unknown>('/api/ddns/provider/delete', {
    method: 'DELETE',
    body: JSON.stringify({ id }),
  })

export interface DdnsRecordPayload {
  enabled: boolean
  providerId: number
  name: string
  domain: string
  subDomain: string
  recordType: string
  ipSource: string
  ipSourceArg: string
  ttl: number
  proxied: boolean
}

export const addDdnsRecord = (body: DdnsRecordPayload) =>
  request<DdnsRecord>('/api/ddns/record/add', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateDdnsRecord = (body: DdnsRecordPayload & { id: number }) =>
  request<unknown>('/api/ddns/record/update', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const deleteDdnsRecord = (id: number) =>
  request<unknown>('/api/ddns/record/delete', {
    method: 'DELETE',
    body: JSON.stringify({ id }),
  })

export const syncDdnsRecord = (id: number) =>
  request<unknown>('/api/ddns/record/sync', {
    method: 'POST',
    body: JSON.stringify({ id }),
  })

// ===================== Webhook =====================

export const getWebhookTemplates = () => request<WebhookTemplate[]>('/api/webhook/templates')

export const addWebhookTemplate = (body: { name: string; description: string; config: WebhookConfig }) =>
  request<WebhookTemplate>('/api/webhook/template/add', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateWebhookTemplate = (body: { id: string; name: string; description: string; config: WebhookConfig }) =>
  request<WebhookTemplate>('/api/webhook/template/update', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const deleteWebhookTemplate = (id: string) =>
  request<unknown>('/api/webhook/template/delete', {
    method: 'DELETE',
    body: JSON.stringify({ id }),
  })
