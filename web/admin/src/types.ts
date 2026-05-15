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
