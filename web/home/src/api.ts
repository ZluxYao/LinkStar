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

// 与 admin 保持一致的鉴权凭证：token 存 localStorage（同源共享），桌面 secret 存 sessionStorage
const TOKEN_KEY = 'linkstar_token'
const DESKTOP_SECRET_KEY = 'linkstar_desktop_secret'

// 桌面版首屏若带 desktop_secret，存入 sessionStorage 并抹掉地址栏
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

interface ApiResponse<T> {
  code: number
  data: T
  msg: string
}

// 后台的登录 / 首次设置密码页面
const LOGIN_URL = '/linkstar/'

// 后端约定：HTTP 一律 200，登录态写在 body 的 code 里
const CODE_UNAUTHORIZED = 401
const CODE_NEED_INIT = 428

/** 去登录。token 过期了就顺手清掉，不然回来还是拿着张废票 */
export function gotoLogin() {
  localStorage.removeItem(TOKEN_KEY)
  window.location.href = LOGIN_URL
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers)
  if (init?.body && !(init.body instanceof FormData) && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json')
  }
  const token = localStorage.getItem(TOKEN_KEY)
  if (token) headers.set('Authorization', `Bearer ${token}`)
  const secret = sessionStorage.getItem(DESKTOP_SECRET_KEY)
  if (secret) headers.set('X-LinkStar-Desktop', secret)

  const resp = await fetch(`${BASE}${path}`, { ...init, headers })
  if (!resp.ok) throw new Error(`HTTP ${resp.status}`)
  const json = (await resp.json()) as ApiResponse<T>
  // 主页是公开的，读不用登录，写要。写到一半发现没登录（或 token 过期了）
  // 就直接去登录页，不要让用户对着一个「操作失败」猜发生了什么。
  if (json.code === CODE_UNAUTHORIZED || json.code === CODE_NEED_INIT) {
    gotoLogin()
    throw new Error(json.msg || '未登录')
  }
  if (json.code !== 0) throw new Error(json.msg || `code=${json.code}`)
  return json.data
}

const get = <T,>(path: string) => request<T>(path)
const send = <T,>(method: string, path: string, body?: unknown) =>
  request<T>(path, { method, body: body === undefined ? undefined : JSON.stringify(body) })

// ============ 登录态 ============
// authed 由后端按这次请求带的 token / 桌面 secret 判定，
// 比前端只看 localStorage 里有没有 token 准——过期的 token 也是有。
export interface AuthStatus {
  initialized: boolean
  authed: boolean
}
export const getAuthStatus = () => get<AuthStatus>('/auth/status')

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

// 上传自定义壁纸，返回相对路径如 data/wallpaper/xxx.jpg
export async function uploadWallpaper(file: File): Promise<string> {
  const form = new FormData()
  form.append('file', file)
  const data = await request<{ path: string }>('/home/wallpaper/upload', { method: 'POST', body: form })
  return data.path
}

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
