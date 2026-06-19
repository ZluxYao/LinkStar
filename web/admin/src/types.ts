export type PageKey =
  | 'dashboard'
  | 'stun'
  | 'ddns'
  | 'reverse-proxy'
  | 'cert'
  | 'user'
  | 'settings'
  | 'audit'
  | 'notify'

export interface StunService {
  id: number
  name: string
  internalPort: number
  protocol: string
  upnpMappedPort: number
  useUpnp: boolean
  https: boolean
  enabled: boolean
  description: string
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

export type DnsProviderType = 'cloudflare'
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
