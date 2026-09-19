import type {
  AcmeProvider,
  Certificate,
  CertSource,
  DdnsConfig,
  DdnsProvider,
  DdnsRecord,
  LanHost,
  LanSubnet,
  LogEntry,
  NatTypeInfo,
  NetInterface,
  ProxyConfig,
  ProxyEntry,
  ProxyProbeResult,
  ProxySite,
  RedirectConfig,
  RedirectInspection,
  StunConfig,
  WebhookConfig,
  WebhookTemplate,
} from '../types'

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

// requestWithMsg 连后端那句话一起拿回来。
//
// 大部分接口只要 data，成功与否界面自己有话说；但有些接口做成了一半——
// 规则写进去了、记录没建上——那句「还差什么」只有后端知道，丢掉就等于报了个假的成功。
async function requestWithMsg<T>(
  path: string,
  init?: RequestInit,
): Promise<{ data: T; msg: string }> {
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
  return { data: json.data as T, msg: json.msg ?? '' }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  return (await requestWithMsg<T>(path, init)).data
}

// ===================== Auth =====================

// MIN_PASSWORD_LENGTH 和后端 auth.MinPasswordLength 保持一致。
// 这里没有用户名，别人只能一串一串地试密码，长度就是唯一的门槛。
export const MIN_PASSWORD_LENGTH = 8

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

// 改完密码旧 token 全部作废，后端会回一个新的，记得存下来顶替旧的
export const changePassword = (oldPassword: string, newPassword: string) =>
  request<{ token: string }>('/api/auth/password', {
    method: 'PUT',
    body: JSON.stringify({ oldPassword, newPassword }),
  })

export const getVersion = () => request<{ version: string }>('/api/version')

// ===================== 运行日志 =====================

export const getLogDays = () => request<string[]>('/api/system/log/days')

export const getLogs = (params: {
  day: string
  file?: 'info' | 'err'
  level?: string
  keyword?: string
  limit?: number
}) => {
  const q = new URLSearchParams({ day: params.day })
  if (params.file) q.set('file', params.file)
  if (params.level) q.set('level', params.level)
  if (params.keyword) q.set('keyword', params.keyword)
  if (params.limit) q.set('limit', String(params.limit))
  return request<{ list: LogEntry[]; truncated: boolean }>(`/api/system/log?${q}`)
}

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

/** 本机接在哪几个局域网上。只读网卡，不发包 */
export const listLanSubnets = () => request<LanSubnet[]>('/api/stun/lan/subnets')

/** 扫一段局域网。要往两百多个地址发连接，两三秒才回得来 */
export const scanLan = (cidr: string) =>
  request<LanHost[]>('/api/stun/lan/scan', {
    method: 'POST',
    body: JSON.stringify({ cidr }),
  })

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
  /** 仅影响链接展示成 http:// 还是 https://，不改变转发行为 */
  https: boolean
  /** 对外域名，留空回落公网 IP */
  domain: string
  /** 由 LinkStar 在洞口终结 TLS，仅 TCP 有效 */
  tlsTerminate: boolean
  /** 绑定证书 ID，0 表示按 SNI 自动匹配 */
  certId: number
  /** 内网服务本身就说 TLS（自签也算），转发时用 tls.Dial */
  backendHttps: boolean
  enabled: boolean
  description: string
  webhookconfig?: WebhookConfig
  redirect?: RedirectConfig
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

/**
 * 立即把服务当前的外网地址写到入口域名上；平时端口一变会自动同步。
 *
 * 返回后端那句完整的话：这一步可能只做成一半（规则写进去了，入口域名的解析记录没建上），
 * 界面自己拼不出「还差什么」，拼了也会和后端各说各的。
 */
export const syncStunRedirect = (deviceId: number, serviceId: number) =>
  requestWithMsg<{ target: string; keepPath: boolean }>('/api/stun/redirect/sync', {
    method: 'POST',
    body: JSON.stringify({ deviceId, serviceId }),
  })

/** 入口/落地这两条解析记录现在各是什么样；入口那条要现场去服务商查，慢一点 */
export const inspectStunRedirect = (deviceId: number, serviceId: number) =>
  request<RedirectInspection>('/api/stun/redirect/inspect', {
    method: 'POST',
    body: JSON.stringify({ deviceId, serviceId }),
  })

/** 删掉服务商那边 LinkStar 写的那条规则 */
export const removeStunRedirect = (deviceId: number, serviceId: number) =>
  request<unknown>('/api/stun/redirect', {
    method: 'DELETE',
    body: JSON.stringify({ deviceId, serviceId }),
  })

export interface HomeApp {
  id: string
  type: string
}

export const getHomeConfig = () =>
  request<{ apps: HomeApp[] }>('/api/home/config')

// ===================== DDNS =====================

export const getDdnsConfig = () => request<DdnsConfig>('/api/ddns/config')

/** 本机网卡列表，给「本地网卡」这个 IP 来源当选项 */
export const getDdnsInterfaces = () => request<NetInterface[]>('/api/ddns/interfaces')

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

// ===================== 证书 =====================

export const getCertList = () =>
  request<{ list: Certificate[]; count: number }>('/api/cert/list').then((r) => r.list ?? [])

/** DNS-01 可选服务商，复用 DDNS 里已配好凭证的那些 */
export const getCertProviders = () =>
  request<{ list: AcmeProvider[]; count: number }>('/api/cert/providers').then((r) => r.list ?? [])

export interface CertPayload {
  name: string
  source: CertSource
  domains: string[]
  enabled: boolean
  isDefault: boolean
  certFile: string
  keyFile: string
  acme: {
    directory: string
    email: string
    providerId: number
    httpPort: number
    eabKeyId: string
    /** 留空表示沿用原值 */
    eabHmac: string
    renewDays: number
  }
}

export const addCert = (body: CertPayload) =>
  request<Certificate>('/api/cert/create', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateCert = (body: CertPayload & { id: number }) =>
  request<Certificate>('/api/cert/update', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const deleteCert = (id: number) =>
  request<unknown>('/api/cert/remove', {
    method: 'DELETE',
    body: JSON.stringify({ id }),
  })

/** 粘贴 PEM 文本上传 */
export const uploadCertPEM = (id: number, certPem: string, keyPem: string) =>
  request<Certificate>('/api/cert/upload', {
    method: 'POST',
    body: JSON.stringify({ id, certPem, keyPem }),
  })

/** 立即签发/续期。ACME 全流程要几分钟，接口只负责启动，结果靠刷新列表看 */
export const issueCert = (id: number) =>
  request<unknown>('/api/cert/issue', {
    method: 'POST',
    body: JSON.stringify({ id }),
  })

// ===================== 反向代理 =====================

/** 入口设置 + 监听状态 + 站点表 */
export const getProxyConfig = () => request<ProxyConfig>('/api/proxy/config')

export interface EntryPayload {
  enabled: boolean
  /** HTTP 入口端口，0 = 不开 */
  httpPort: number
  /** HTTPS 入口端口，0 = 不开 */
  httpsPort: number
  /** 0 表示按 SNI 自动匹配 */
  certId: number
  /** 主域名，如 example.com；留空则站点必须写全域名 */
  baseDomain: string
}

export const saveProxyEntry = (body: EntryPayload) =>
  request<ProxyEntry>('/api/proxy/entry', {
    method: 'POST',
    body: JSON.stringify(body),
  })

/** 单独一个接口：它即点即生效，不该顺带把监听重开一遍 */
export const setProxyAccessLog = (enabled: boolean) =>
  request<unknown>('/api/proxy/accesslog', {
    method: 'POST',
    body: JSON.stringify({ enabled }),
  })

export interface ProxySitePayload {
  hosts: string[]
  pathPrefix: string
  stripPrefix: boolean
  backend: string
  backendHttps: boolean
  /** 单独占一个端口；0 = 挂在默认入口上 */
  listenPort: number
  /** 对外走 HTTPS */
  https: boolean
  /** 0 表示按 SNI 自动匹配 */
  certId: number
  enabled: boolean
  description: string
}

export const addProxySite = (body: ProxySitePayload) =>
  request<ProxySite>('/api/proxy/site', {
    method: 'POST',
    body: JSON.stringify(body),
  })

export const updateProxySite = (body: ProxySitePayload & { id: number }) =>
  request<ProxySite>('/api/proxy/site', {
    method: 'PUT',
    body: JSON.stringify(body),
  })

export const deleteProxySite = (id: number) =>
  request<unknown>('/api/proxy/site', {
    method: 'DELETE',
    body: JSON.stringify({ id }),
  })

/** 立即拨一次后端。不要求先保存，填到一半就能点 */
export const testProxySite = (body: ProxySitePayload) =>
  request<ProxyProbeResult>('/api/proxy/site/test', {
    method: 'POST',
    body: JSON.stringify(body),
  })
