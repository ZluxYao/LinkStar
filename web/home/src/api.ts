import type {
  AppAddresses,
  AppView,
  Category,
  HomeConfig,
  LayoutMode,
  NetworkPrefer,
  SearchEngine,
  Wallpaper,
} from './types'

const BASE = '/api'

interface ApiResponse<T> {
  code: number
  data: T
  msg: string
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  if (init?.body && !(init.body instanceof FormData) && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  const resp = await fetch(`${BASE}${path}`, { ...init, headers })
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
  const json = (await resp.json()) as ApiResponse<T>
  if (json.code !== 0) throw new Error(json.msg || `code=${json.code}`)
  return json.data
}

const get = <T,>(path: string) => request<T>(path)
const send = <T,>(method: string, path: string, body?: unknown) =>
  request<T>(path, { method, body: body === undefined ? undefined : JSON.stringify(body) })

// ============ Home Config ============
// 防御: Go 序列化空切片有时会变 null,这里统一兜底成空数组
export async function getConfig(): Promise<HomeConfig> {
  const data = await get<HomeConfig>('/home/config')
  data.searchEngines = data.searchEngines ?? []
  data.searchHistory = data.searchHistory ?? []
  data.categories = data.categories ?? []
  data.apps = data.apps ?? []
  return data
}

// ============ 主页装饰 ============
export const updateWallpaper = (body: Wallpaper) => send<unknown>('PUT', '/home/wallpaper', body)
export const updateLayout = (layoutMode: LayoutMode) => send<unknown>('PUT', '/home/layout', { layoutMode })
export const updateNetwork = (networkPrefer: NetworkPrefer) => send<unknown>('PUT', '/home/network', { networkPrefer })

// ============ 搜索引擎 ============
export const addSearchEngine = (body: Omit<SearchEngine, 'order'>) =>
  send<unknown>('POST', '/home/search-engine/add', body)
export const updateSearchEngine = (body: Omit<SearchEngine, 'order'>) =>
  send<unknown>('PUT', '/home/search-engine/update', body)
export const deleteSearchEngine = (id: string) =>
  send<unknown>('DELETE', '/home/search-engine/delete', { id })
export const reorderSearchEngines = (ids: string[]) =>
  send<unknown>('PUT', '/home/search-engine/reorder', { ids })
export const setDefaultSearchEngine = (id: string) =>
  send<unknown>('PUT', '/home/search-engine/default', { id })

// ============ 搜索历史 ============
export const getSearchHistory = () => get<string[]>('/home/search-history')
export const addSearchHistory = (keyword: string) =>
  send<unknown>('POST', '/home/search-history/add', { keyword })
export const clearSearchHistory = () =>
  send<unknown>('DELETE', '/home/search-history/clear')

// ============ 分类 ============
export const addCategory = (name: string) => send<Category>('POST', '/home/category/add', { name })
export const updateCategory = (id: string, name: string) =>
  send<unknown>('PUT', '/home/category/update', { id, name })
export const deleteCategory = (id: string) =>
  send<unknown>('DELETE', '/home/category/delete', { id })
export const reorderCategories = (ids: string[]) =>
  send<unknown>('PUT', '/home/category/reorder', { ids })

// ============ App ============
export interface AddAppRequest {
  name: string
  icon?: string
  color?: string
  categoryId?: string
  addresses: AppAddresses
  // 翻页布局下的绝对槽位 (1-indexed)
  pagedOrder?: number
}
export interface UpdateAppRequest {
  id: string
  name: string
  icon?: string
  color?: string
  categoryId?: string
  addresses?: AppAddresses
}
export interface AppPositionItem {
  id: string
  pagedOrder: number
}

export const addApp = (body: AddAppRequest) => send<AppView>('POST', '/home/app/add', body)
export const updateApp = (body: UpdateAppRequest) => send<unknown>('PUT', '/home/app/update', body)
export const deleteApp = (id: string) => send<unknown>('DELETE', '/home/app/delete', { id })
export const reorderApps = (mode: 'paged' | 'scroll', ids: string[], categoryId?: string) =>
  send<unknown>('PUT', '/home/app/reorder', { mode, ids, categoryId })
export const setAppPositions = (positions: AppPositionItem[]) =>
  send<unknown>('PUT', '/home/app/position', { positions })
export const setAppCategory = (id: string, categoryId: string) =>
  send<unknown>('PUT', '/home/app/category', { id, categoryId })

// ============ 图标上传 ============
export async function uploadIcon(file: File): Promise<string> {
  const form = new FormData()
  form.append('file', file)
  const data = await request<{ path: string }>('/home/icon/upload', { method: 'POST', body: form })
  return data.path
}

// ============ 从 URL 抓取图标 ============
export async function fetchIconFromURL(url: string): Promise<string> {
  const data = await send<{ path: string }>('POST', '/home/icon/fetch', { url })
  return data.path
}

// ============ Bing 壁纸 (沿用) ============
export interface BingWallpaper { url: string }
export async function getBingWallpaper(resolution: 'uhd' | '1080'): Promise<BingWallpaper> {
  const resp = await fetch(`/api/home/bing-wallpaper?resolution=${resolution}&t=${Date.now()}`)
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
  return resp.json() as Promise<BingWallpaper>
}
