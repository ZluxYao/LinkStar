export type PageKey =
  | 'dashboard'
  | 'stun'
  | 'ddns'
  | 'reverse-proxy'
  | 'cert'
  | 'settings'
  | 'logs'
  | 'notify'

export interface LogEntry {
  time: string
  level: string
  caller: string
  message: string
}

export interface StunService {
  id: number
  name: string
  internalPort: number
  protocol: string
  upnpMappedPort: number
  useUpnp: boolean
  /** 仅影响链接展示成 http:// 还是 https://，不改变转发行为 */
  https: boolean
  /** 该服务对外域名，如 fw.example.com；留空回落公网 IP */
  domain: string
  /** 由 LinkStar 在洞口终结 TLS（仅 TCP 有效） */
  tlsTerminate: boolean
  /** 绑定的证书 ID，0 表示按域名自动匹配 */
  certId: number
  /** 内网服务本身就说 TLS（自签也算），转发时用 tls.Dial */
  backendHttps: boolean
  enabled: boolean
  description: string
  webhookconfig?: WebhookConfig
}

export interface WebhookConfig {
  enabled: boolean
  onlyWhenChanged: boolean
  url: string
  method: string
  headers: string
  body: string
  disableSuccessCheck: boolean
  successContains: string
  proxy: string
}

export interface WebhookTemplate {
  id: string
  name: string
  description: string
  builtin: boolean
  config: WebhookConfig
  createdAt?: string
  updatedAt?: string
}

export interface StunDevice {
  id: number
  DeviceID?: number
  deviceId?: number
  name: string
  ip: string
  services: StunService[]
}

export interface NatRouter {
  natLevel: number
  lanIP: string
  ipType: 'private' | 'cgn'
}

export interface NatDetectResult {
  natType: string
  mapping: string
  filtering: string
  mappingErr?: string
  filteringErr?: string
}

export interface NatTypeInfo {
  udp: NatDetectResult | null
  tcp: NatDetectResult | null
  error?: string
}

export interface StunConfig {
  localIP: string
  publicIP: string
  bestStun: string
  natRouterList: NatRouter[]
  devices: StunDevice[]
}

export interface StunStatusLog {
  createdAt: string
  phaseStr: string
  message: string
}

export interface StunStatusEvent {
  key: string
  deviceName?: string
  serviceName?: string
  phaseStr: string
  restartCount?: number
  externalPort?: number
  lastError?: string
  logs?: StunStatusLog[]
}

export type DnsProviderType =
  | 'cloudflare'
  | 'alidns'
  | 'tencentcloud'
  | 'baiducloud'
  | 'huaweicloud'
  | 'namecheap'
  | 'namesilo'
export type DnsRecordType = 'A' | 'AAAA'
export type DdnsRecordStatus = 'pending' | 'success' | 'failed' | 'skipped'
export type IpSourceType = 'stun' | 'web' | 'dns' | 'interface'

export interface DdnsProvider {
  id: number
  name: string
  type: DnsProviderType
  hasCredential: boolean
}

export interface DdnsRecord {
  enabled: boolean
  id: number
  providerId: number
  name: string
  domain: string
  subDomain: string
  recordType: DnsRecordType
  ipSource: IpSourceType
  ipSourceArg: string
  ttl: number
  proxied: boolean
  lastIP: string
  lastStatus: DdnsRecordStatus
  lastMessage: string
  lastCheckAt: string
  lastSyncAt: string
  createdAt: string
  updatedAt: string
}

export interface DdnsConfig {
  intervalSec: number
  providers: DdnsProvider[]
  records: DdnsRecord[]
}

// ===================== 证书 =====================

/**
 * upload     手动粘贴/上传 PEM
 * path       读本机文件路径（外部 certbot / acme.sh 续期后自动热重载）
 * acme-dns01 自动签发，写 TXT 验证，支持通配符
 * acme-http01 自动签发，需公网 80 端口可入站
 */
export type CertSource = 'upload' | 'path' | 'acme-dns01' | 'acme-http01' | 'self-signed'

export interface AcmeOptions {
  directory: string
  email: string
  providerId: number
  httpPort: number
  eabKeyId: string
  /** HMAC 属于密钥，后端只回是否已配置 */
  hasEabHmac: boolean
  renewDays: number
}

export interface Certificate {
  id: number
  name: string
  source: CertSource
  domains: string[]
  enabled: boolean
  isDefault: boolean
  certFile: string
  keyFile: string
  acme: AcmeOptions
  notBefore?: string
  notAfter?: string
  issuer?: string
  lastError?: string
  lastIssueAt?: string
  /** 是否已真正装载进内存对外提供服务 */
  loaded: boolean
  daysLeft: number
  expired: boolean
  createdAt?: string
  updatedAt?: string
}

export interface AcmeProvider {
  id: number
  name: string
  type: DnsProviderType
  /** false 时该服务商还没实现 TXT 写入，不能用于 DNS-01 */
  supportsDns01: boolean
}

// ===================== 反向代理 =====================

/**
 * 默认入口，对应 nginx 的 listen 80 / listen 443 ssl。
 *
 * 站点不单独指定端口时就挂在这两个上面。端口填 0 表示不开这个入口。
 * 这里是用户的意愿；实际跑没跑起来看 listeners。
 */
export interface ProxyEntry {
  enabled: boolean
  /** HTTP 入口端口，0 = 不开 */
  httpPort: number
  /** HTTPS 入口端口，0 = 不开 */
  httpsPort: number
  /** HTTPS 入口的默认证书，0 表示按 SNI 自动匹配 */
  certId: number
  accessLog: boolean
  /** 主域名，如 zlux.top；加站点时只填前缀就能补全 */
  baseDomain: string
}

/** 此刻实际开着的一个监听端口，对照 netstat 看到的那一行 */
export interface ProxyListener {
  port: number
  /** 这个端口是 https */
  tls: boolean
  /** 0 表示按 SNI 自动匹配 */
  certId: number
  /** 挂在这个端口上的站点数 */
  siteCount: number
  running: boolean
  lastError: string
}

/** 一个站点，对应 nginx 的一个 server 块 */
export interface ProxySite {
  id: number
  /** 域名，分流依据，至少一个。对应 nginx 的 server_name a.com www.a.com */
  hosts: string[]
  pathPrefix: string
  /** 转发前剥掉前缀，后端需自己支持 base path */
  stripPrefix: boolean
  backend: string
  /** 后端本身就说 TLS（自签也算） */
  backendHttps: boolean
  /** 单独占一个端口；0 = 挂在默认入口上 */
  listenPort: number
  /** 对外走 HTTPS */
  https: boolean
  /** 0 表示按 SNI 自动匹配 */
  certId: number
  enabled: boolean
  description: string
  updatedAt?: string
}

export interface ProxyConfig {
  entry: ProxyEntry
  listeners: ProxyListener[]
  sites: ProxySite[]
}

export interface ProxyProbeResult {
  reachable: boolean
  /** 实际探到的协议：http / https */
  scheme: string
  /** 和用户勾的不一致 */
  mismatch: boolean
  latencyMs: number
  message: string
}
