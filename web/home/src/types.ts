// 与后端 modules/home/model.go / hydrate.go 对应

export type LayoutMode = 'paged-horizontal' | 'paged-vertical' | 'paged-free' | 'scroll'
export type NetworkPrefer = 'wanV4' | 'wanV6' | 'lan'
export type WallpaperMode = 'default' | 'bing' | 'custom'
export type WallpaperResolution = '1080' | 'uhd'
export type AppType = 'stun' | 'static'

export interface Wallpaper {
  mode: WallpaperMode
  resolution: WallpaperResolution
  blur: number
  /** 自己上传的壁纸，形如 data/wallpaper/xxx.jpg；和 mode 分开存，切走再切回来不用重传 */
  custom: string
  /** 默认背景的样式，.default-wallpaper 那条规则大括号里的内容；空 = 用内置的 */
  css: string
}

export interface SearchEngine {
  id: string
  name: string
  shortName: string
  url: string
  color: string
  icon?: string
  order: number
}

export interface Category {
  id: string
  name: string
  order: number
  createdAt?: string
  updatedAt?: string
}

export interface AppAddresses {
  wanV4: string
  wanV6: string
  lan: string
}

export interface AppView {
  id: string
  name: string
  icon: string
  color: string
  categoryId: string
  pagedOrder: number
  scrollOrder: number
  type: AppType
  addresses: AppAddresses
  online?: boolean
  createdAt?: string
  updatedAt?: string
}

export interface HomeConfig {
  title: string
  showTime: boolean
  wallpaper: Wallpaper
  layoutMode: LayoutMode
  networkPrefer: NetworkPrefer
  defaultSearchEngineId: string
  searchEngines: SearchEngine[]
  searchHistory: string[]
  categories: Category[]
  apps: AppView[]
}
