import type { StunConfig } from '../types'

interface ApiResponse<T> {
  code: number
  msg?: string
  data?: T
}

async function request<T>(
  path: string,
  init?: RequestInit,
): Promise<T> {
  const resp = await fetch(path, {
    headers: { 'Content-Type': 'application/json' },
    ...init,
  })
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
  const json = (await resp.json()) as ApiResponse<T>
  if (json.code !== 0) throw new Error(json.msg || '请求失败')
  return json.data as T
}

export const getStunConfig = () => request<StunConfig>('/api/stun/config')

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
  tls: boolean
  enabled: boolean
  description: string
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
