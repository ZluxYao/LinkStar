import { useEffect, useMemo, useRef, useState } from 'react'
import {
  BriefcaseBusiness,
  CalendarDays,
  Cloud,
  Code2,
  ExternalLink,
  FileText,
  Globe2,
  HardDrive,
  Home,
  Image as ImageIcon,
  Languages,
  LayoutGrid,
  MessageCircle,
  MonitorCog,
  Music2,
  NotebookText,
  Pencil,
  Plus,
  Search,
  Settings,
  Shield,
  Tags,
  Terminal,
  Trash2,
  X,
} from 'lucide-react'
import './App.css'

type SearchEngine = {
  id: string
  name: string
  shortName: string
  url: string
  color: string
  icon?: string
}

type AppItem = {
  id: string
  name: string
  url: string
  color: string
  icon: React.ComponentType<{ className?: string }>
  status?: 'online' | 'offline'
}

const searchEngines: SearchEngine[] = [
  { id: 'bing', name: '必应', shortName: 'b', url: 'https://www.bing.com/search?q=', color: 'from-sky-400 to-cyan-600', icon: '/icons/bing.com.ico' },
  { id: 'google', name: '谷歌', shortName: 'G', url: 'https://www.google.com/search?q=', color: 'from-red-400 via-yellow-400 to-blue-500', icon: '/icons/google.com.svg' },
  { id: 'ddg', name: 'DDG', shortName: 'D', url: 'https://duckduckgo.com/?q=', color: 'from-orange-400 to-red-600', icon: '/icons/dackdackgo.com.svg' },
  { id: 'brave', name: 'Brave', shortName: 'B', url: 'https://search.brave.com/search?q=', color: 'from-orange-500 to-red-700', icon: '/icons/brave.com.svg' },
  { id: 'github-search', name: 'GitHub', shortName: 'GH', url: 'https://github.com/search?q=', color: 'from-zinc-700 to-black', icon: '/icons/github.com.svg' },
  { id: 'zhihu', name: '知乎', shortName: '知', url: 'https://www.zhihu.com/search?q=', color: 'from-blue-500 to-sky-600', icon: '/icons/zhihu.com.ico' },
  { id: 'bilibili', name: 'B站', shortName: 'B', url: 'https://search.bilibili.com/all?keyword=', color: 'from-sky-300 to-blue-500', icon: '/icons/bilibili.com.ico' },
]

const searchHistoryStorageKey = 'linkstar.searchHistory'
const searchHistoryMax = 6

function loadSearchHistory(): string[] {
  try {
    const value = localStorage.getItem(searchHistoryStorageKey)
    if (!value) return []
    const parsed = JSON.parse(value)
    return Array.isArray(parsed) ? parsed.filter((item): item is string => typeof item === 'string') : []
  } catch {
    return []
  }
}

type BingWallpaperResponse = {
  url: string
}

type WallpaperMode = 'default' | 'bing'
type WallpaperResolution = '1080' | 'uhd'
type LayoutMode = 'paged-horizontal' | 'paged-vertical' | 'paged-free' | 'scroll'

type HomeSettings = {
  wallpaperMode: WallpaperMode
  wallpaperResolution: WallpaperResolution
  wallpaperBlur: number
  layoutMode: LayoutMode
}

type Category = { id: string; name: string }

const bingWallpaperUrl = '/api/home/bing-wallpaper'
const wallpaperStorageKey = 'linkstar.wallpaperUrl'
const homeSettingsStorageKey = 'linkstar.homeSettings'

const defaultHomeSettings: HomeSettings = {
  wallpaperMode: 'bing',
  wallpaperResolution: '1080',
  wallpaperBlur: 0,
  layoutMode: 'paged-free',
}

const defaultCategories: Category[] = [
  { id: 'productivity', name: '生产力' },
  { id: 'tools', name: '工具' },
  { id: 'entertainment', name: '娱乐' },
]

const defaultAppCategoryMap: Record<string, string> = {
  nas: 'tools',
  ssh: 'tools',
  admin: 'tools',
  github: 'productivity',
  chat: 'productivity',
  translate: 'productivity',
  calendar: 'productivity',
  notes: 'productivity',
  files: 'tools',
  ddns: 'tools',
  proxy: 'tools',
  code: 'productivity',
  music: 'entertainment',
  work: 'productivity',
}

function loadHomeSettings() {
  try {
    const value = localStorage.getItem(homeSettingsStorageKey)
    return value ? { ...defaultHomeSettings, ...JSON.parse(value) } : defaultHomeSettings
  } catch {
    return defaultHomeSettings
  }
}

const demoApps: AppItem[] = [
  { id: 'nas', name: 'NAS 管理', url: '#', color: 'from-sky-400 to-blue-600', icon: HardDrive, status: 'online' },
  { id: 'ssh', name: 'SSH 终端', url: '#', color: 'from-slate-700 to-zinc-950', icon: Terminal, status: 'online' },
  { id: 'admin', name: '后台控制', url: '/admin', color: 'from-indigo-400 to-violet-700', icon: MonitorCog, status: 'online' },
  { id: 'github', name: 'GitHub', url: '#', color: 'from-neutral-700 to-black', icon: Code2, status: 'online' },
  { id: 'chat', name: 'AI 助手', url: '#', color: 'from-emerald-400 to-teal-700', icon: MessageCircle, status: 'online' },
  { id: 'translate', name: '翻译', url: '#', color: 'from-purple-400 to-fuchsia-700', icon: Languages, status: 'online' },
  { id: 'calendar', name: '日历', url: '#', color: 'from-rose-400 to-red-600', icon: CalendarDays, status: 'online' },
  { id: 'notes', name: '笔记', url: '#', color: 'from-amber-300 to-orange-600', icon: NotebookText, status: 'online' },
  { id: 'files', name: '文件管理', url: '#', color: 'from-yellow-300 to-amber-600', icon: FileText, status: 'online' },
  { id: 'ddns', name: 'DDNS', url: '#', color: 'from-cyan-300 to-blue-700', icon: Cloud, status: 'offline' },
  { id: 'proxy', name: '反向代理', url: '#', color: 'from-green-400 to-emerald-800', icon: Shield, status: 'offline' },
  { id: 'code', name: '代码服务', url: '#', color: 'from-pink-400 to-rose-700', icon: Code2, status: 'online' },
  { id: 'music', name: '音乐', url: '#', color: 'from-violet-400 to-purple-800', icon: Music2, status: 'online' },
  { id: 'work', name: '工作台', url: '#', color: 'from-blue-300 to-indigo-700', icon: BriefcaseBusiness, status: 'online' },
  { id: 'add', name: '添加图标', url: '#', color: 'from-white/30 to-white/10', icon: Plus, status: 'offline' },
]

function AppIcon({ app }: { app: AppItem }) {
  const Icon = app.icon

  const handleClick = () => {
    if (app.url === '#') return
    window.open(app.url, app.url.startsWith('/') ? '_self' : '_blank')
  }

  return (
    <button type="button" onClick={handleClick} className="group flex w-24 flex-col items-center gap-2 text-white outline-none">
      <div className={`relative grid h-16 w-16 place-items-center rounded-2xl bg-gradient-to-br ${app.color} shadow-lg ring-1 ring-white/30 transition duration-200 group-hover:-translate-y-1 group-hover:scale-105 group-hover:shadow-2xl`}>
        <Icon className="h-8 w-8 drop-shadow" />
        {app.status === 'online' && <span className="absolute -right-1 -top-1 h-3 w-3 rounded-full border-2 border-white bg-emerald-400" />}
      </div>
      <span className="max-w-24 truncate text-sm font-medium drop-shadow-[0_1px_2px_rgba(0,0,0,0.9)]">{app.name}</span>
    </button>
  )
}

function SearchEngineIcon({ engine, active, onClick }: { engine: SearchEngine; active: boolean; onClick: () => void }) {
  return (
    <button type="button" onClick={onClick} className="group flex w-16 flex-col items-center gap-1.5 outline-none">
      <div className={`grid h-14 w-14 place-items-center rounded-xl bg-slate-100 text-white shadow-sm transition duration-200 group-hover:-translate-y-1 group-hover:shadow-lg ${active ? 'ring-4 ring-blue-200' : 'ring-1 ring-slate-200'}`}>
        {engine.icon ? (
          <img src={engine.icon} alt="" className="h-8 w-8 object-contain" />
        ) : (
          <div className={`grid h-full w-full place-items-center rounded-xl bg-gradient-to-br ${engine.color}`}>
            <span className="text-lg font-black tracking-tight">{engine.shortName}</span>
          </div>
        )}
      </div>
      <span className={`max-w-16 truncate text-xs ${active ? 'font-semibold text-blue-600' : 'text-slate-600'}`}>{engine.name}</span>
    </button>
  )
}

function Clock() {
  const [now, setNow] = useState(() => new Date())

  useEffect(() => {
    const timer = window.setInterval(() => setNow(new Date()), 1000)
    return () => window.clearInterval(timer)
  }, [])

  const time = now.toLocaleTimeString('zh-CN', { hour12: false })
  const date = now.toLocaleDateString('zh-CN', { month: 'long', day: 'numeric', weekday: 'long' })

  return (
    <div className="flex items-end justify-center gap-3 text-white drop-shadow-[0_2px_8px_rgba(0,0,0,0.65)]">
      <h1 className="text-5xl font-black tracking-tight">LinkStar</h1>
      <div className="mb-1 text-left">
        <div className="text-2xl font-bold leading-none">{time}</div>
        <div className="mt-1 text-sm font-medium opacity-90">{date}</div>
      </div>
    </div>
  )
}

function App() {
  const [engineId, setEngineId] = useState('bing')
  const [query, setQuery] = useState('')
  const [appPage, setAppPage] = useState(0)
  const [searchHistory, setSearchHistory] = useState<string[]>(() => loadSearchHistory())
  const [showHistory, setShowHistory] = useState(false)
  const [showEngines, setShowEngines] = useState(false)
  const [showSettings, setShowSettings] = useState(false)
  const [settingsTab, setSettingsTab] = useState<'appearance' | 'layout' | 'categories' | 'search'>('appearance')
  const [homeSettings, setHomeSettings] = useState<HomeSettings>(() => loadHomeSettings())
  const [categories, setCategories] = useState<Category[]>(defaultCategories)
  const [appCategoryMap, setAppCategoryMap] = useState<Record<string, string>>(defaultAppCategoryMap)
  const [editingCategoryId, setEditingCategoryId] = useState<string | null>(null)
  const [editingCategoryName, setEditingCategoryName] = useState('')
  const [dragOverCategoryId, setDragOverCategoryId] = useState<string | null>(null)
  const [showDefaultWallpaper, setShowDefaultWallpaper] = useState(() => loadHomeSettings().wallpaperMode === 'default')
  const [backgroundReady, setBackgroundReady] = useState(() => loadHomeSettings().wallpaperMode === 'default')
  const [wallpaperUrl, setWallpaperUrl] = useState<string | null>(null)
  const [wallpaperLoaded, setWallpaperLoaded] = useState(false)
  const searchInputRef = useRef<HTMLInputElement>(null)
  const wallpaperFallbackTimer = useRef<number | null>(null)
  const appsPagerRef = useRef<HTMLDivElement>(null)
  const animatingRef = useRef(false)
  const slideTimeoutRef = useRef<number | null>(null)
  const touchStartXRef = useRef<number | null>(null)
  const touchStartYRef = useRef<number | null>(null)
  const dragStartXRef = useRef<number | null>(null)
  const dragStartYRef = useRef<number | null>(null)

  type AppSlide = { from: number; axis: 'x' | 'y'; dir: 1 | -1 }
  const [appSlide, setAppSlide] = useState<AppSlide | null>(null)

  const appsPerPage = 8
  const layoutMode = homeSettings.layoutMode
  const isScrollMode = layoutMode === 'scroll'
  const allowHorizontal = layoutMode === 'paged-horizontal' || layoutMode === 'paged-free'
  const allowVertical = layoutMode === 'paged-vertical' || layoutMode === 'paged-free'

  const appPages = useMemo(() => {
    const pages: AppItem[][] = []
    for (let i = 0; i < demoApps.length; i += appsPerPage) {
      pages.push(demoApps.slice(i, i + appsPerPage))
    }
    return pages.length > 0 ? pages : [[]]
  }, [])

  const groupedApps = useMemo(() => {
    const groups = categories.map((cat) => ({
      category: cat,
      apps: demoApps.filter((app) => appCategoryMap[app.id] === cat.id),
    }))
    const validIds = new Set(categories.map((c) => c.id))
    const uncategorized = demoApps.filter(
      (app) => !appCategoryMap[app.id] || !validIds.has(appCategoryMap[app.id]),
    )
    return { groups, uncategorized }
  }, [categories, appCategoryMap])

  const addCategory = () => {
    const id = `cat-${Date.now().toString(36)}`
    setCategories((prev) => [...prev, { id, name: '新分类' }])
    setEditingCategoryId(id)
    setEditingCategoryName('新分类')
  }

  const renameCategory = (id: string, name: string) => {
    const trimmed = name.trim() || '未命名'
    setCategories((prev) => prev.map((c) => (c.id === id ? { ...c, name: trimmed } : c)))
  }

  const deleteCategory = (id: string) => {
    setCategories((prev) => prev.filter((c) => c.id !== id))
    setAppCategoryMap((prev) => {
      const next: Record<string, string> = {}
      for (const [appId, catId] of Object.entries(prev)) {
        if (catId !== id) next[appId] = catId
      }
      return next
    })
  }

  const assignAppToCategory = (appId: string, categoryId: string) => {
    setAppCategoryMap((prev) => {
      const next = { ...prev }
      if (categoryId === '') delete next[appId]
      else next[appId] = categoryId
      return next
    })
  }

  const movePage = (delta: number, axis: 'x' | 'y') => {
    if (animatingRef.current) return
    setAppPage((curr) => {
      const next = curr + delta
      if (next === curr || next < 0 || next >= appPages.length) return curr
      animatingRef.current = true
      setAppSlide({ from: curr, axis, dir: delta > 0 ? 1 : -1 })
      if (slideTimeoutRef.current !== null) window.clearTimeout(slideTimeoutRef.current)
      slideTimeoutRef.current = window.setTimeout(() => {
        setAppSlide(null)
        animatingRef.current = false
      }, 360)
      return next
    })
  }

  const goToAppPage = (target: number, axis: 'x' | 'y') => {
    if (animatingRef.current) return
    setAppPage((curr) => {
      if (target === curr || target < 0 || target >= appPages.length) return curr
      animatingRef.current = true
      setAppSlide({ from: curr, axis, dir: target > curr ? 1 : -1 })
      if (slideTimeoutRef.current !== null) window.clearTimeout(slideTimeoutRef.current)
      slideTimeoutRef.current = window.setTimeout(() => {
        setAppSlide(null)
        animatingRef.current = false
      }, 360)
      return target
    })
  }

  useEffect(() => {
    if (appPage > appPages.length - 1) setAppPage(appPages.length - 1)
  }, [appPages.length, appPage])

  useEffect(() => {
    if (showSettings || isScrollMode) return

    const resolveAxis = (absX: number, absY: number): 'x' | 'y' | null => {
      if (allowHorizontal && allowVertical) return absX > absY ? 'x' : 'y'
      if (allowHorizontal) return absX > absY ? 'x' : null
      if (allowVertical) return absY > absX ? 'y' : null
      return null
    }

    const onWheel = (event: WheelEvent) => {
      const absX = Math.abs(event.deltaX)
      const absY = Math.abs(event.deltaY)
      if (Math.max(absX, absY) < 8) return
      const axis = resolveAxis(absX, absY)
      if (!axis) return
      event.preventDefault()
      const delta = axis === 'x' ? event.deltaX : event.deltaY
      movePage(delta > 0 ? 1 : -1, axis)
    }
    const onTouchStart = (event: TouchEvent) => {
      touchStartXRef.current = event.touches[0].clientX
      touchStartYRef.current = event.touches[0].clientY
    }
    const onTouchEnd = (event: TouchEvent) => {
      if (touchStartXRef.current === null || touchStartYRef.current === null) return
      const dx = touchStartXRef.current - event.changedTouches[0].clientX
      const dy = touchStartYRef.current - event.changedTouches[0].clientY
      touchStartXRef.current = null
      touchStartYRef.current = null
      const absX = Math.abs(dx)
      const absY = Math.abs(dy)
      if (Math.max(absX, absY) < 40) return
      const axis = resolveAxis(absX, absY)
      if (!axis) return
      const delta = axis === 'x' ? dx : dy
      movePage(delta > 0 ? 1 : -1, axis)
    }
    const onMouseDown = (event: MouseEvent) => {
      if (event.button !== 0) return
      const target = event.target as HTMLElement | null
      if (target?.closest('input, textarea, select, button, a')) return
      dragStartXRef.current = event.clientX
      dragStartYRef.current = event.clientY
    }
    const onMouseUp = (event: MouseEvent) => {
      if (dragStartXRef.current === null || dragStartYRef.current === null) return
      const dx = dragStartXRef.current - event.clientX
      const dy = dragStartYRef.current - event.clientY
      dragStartXRef.current = null
      dragStartYRef.current = null
      const absX = Math.abs(dx)
      const absY = Math.abs(dy)
      if (Math.max(absX, absY) < 40) return
      const axis = resolveAxis(absX, absY)
      if (!axis) return
      const delta = axis === 'x' ? dx : dy
      movePage(delta > 0 ? 1 : -1, axis)
    }

    window.addEventListener('wheel', onWheel, { passive: false })
    window.addEventListener('touchstart', onTouchStart, { passive: true })
    window.addEventListener('touchend', onTouchEnd, { passive: true })
    window.addEventListener('mousedown', onMouseDown)
    window.addEventListener('mouseup', onMouseUp)
    return () => {
      window.removeEventListener('wheel', onWheel)
      window.removeEventListener('touchstart', onTouchStart)
      window.removeEventListener('touchend', onTouchEnd)
      window.removeEventListener('mousedown', onMouseDown)
      window.removeEventListener('mouseup', onMouseUp)
    }
  }, [appPages.length, showSettings, isScrollMode, allowHorizontal, allowVertical])

  const getSlideInClass = (axis: 'x' | 'y', dir: 1 | -1) => {
    if (axis === 'x') return dir === 1 ? 'app-slide-in-from-right' : 'app-slide-in-from-left'
    return dir === 1 ? 'app-slide-in-from-bottom' : 'app-slide-in-from-top'
  }
  const getSlideOutClass = (axis: 'x' | 'y', dir: 1 | -1) => {
    if (axis === 'x') return dir === 1 ? 'app-slide-out-to-left' : 'app-slide-out-to-right'
    return dir === 1 ? 'app-slide-out-to-top' : 'app-slide-out-to-bottom'
  }

  const clearWallpaperFallbackTimer = () => {
    if (wallpaperFallbackTimer.current !== null) {
      window.clearTimeout(wallpaperFallbackTimer.current)
      wallpaperFallbackTimer.current = null
    }
  }

  const engine = useMemo(() => searchEngines.find((item) => item.id === engineId) ?? searchEngines[0], [engineId])

  useEffect(() => {
    localStorage.setItem(homeSettingsStorageKey, JSON.stringify(homeSettings))
  }, [homeSettings])

  useEffect(() => {
    let canceled = false
    clearWallpaperFallbackTimer()

    if (homeSettings.wallpaperMode === 'default') {
      setShowDefaultWallpaper(true)
      setBackgroundReady(true)
      setWallpaperLoaded(false)
      setWallpaperUrl(null)
      localStorage.removeItem(wallpaperStorageKey)
      return
    }

    setShowDefaultWallpaper(false)
    setBackgroundReady(false)
    setWallpaperLoaded(false)
    wallpaperFallbackTimer.current = window.setTimeout(() => {
      if (canceled) return
      canceled = true
      setWallpaperLoaded(false)
      setWallpaperUrl(null)
      setShowDefaultWallpaper(true)
      setBackgroundReady(true)
      clearWallpaperFallbackTimer()
    }, 3000)

    fetch(`${bingWallpaperUrl}?resolution=${homeSettings.wallpaperResolution}&t=${Date.now()}`)
      .then((response) => {
        if (!response.ok) {
          throw new Error('Failed to load wallpaper URL')
        }
        return response.json() as Promise<BingWallpaperResponse>
      })
      .then((data) => {
        if (!data.url || canceled) return

        const image = new Image()
        image.onload = () => {
          if (canceled) return
          clearWallpaperFallbackTimer()
          localStorage.setItem(wallpaperStorageKey, data.url)
          setShowDefaultWallpaper(false)
          setWallpaperUrl(data.url)
          setWallpaperLoaded(true)
          setBackgroundReady(true)
        }
        image.onerror = () => {
          if (!canceled) {
            setShowDefaultWallpaper(true)
            setBackgroundReady(true)
          }
          clearWallpaperFallbackTimer()
        }
        image.src = data.url
      })
      .catch(() => {
        if (!canceled) {
          setShowDefaultWallpaper(true)
          setBackgroundReady(true)
        }
        clearWallpaperFallbackTimer()
      })

    return () => {
      canceled = true
      clearWallpaperFallbackTimer()
    }
  }, [homeSettings.wallpaperMode, homeSettings.wallpaperResolution])

  const openWithWhiteLoading = (url: string) => {
    const page = window.open('', '_blank')
    if (!page) {
      window.open(url, '_blank')
      return
    }

    page.document.write(`<!doctype html><html><head><title>Loading...</title><style>html,body{margin:0;width:100%;height:100%;background:#fff;color:#64748b;font-family:system-ui,-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;}body{display:grid;place-items:center;}</style></head><body></body></html>`)
    page.document.close()
    window.setTimeout(() => {
      page.location.href = url
    }, 50)
  }

  const submitSearch = (value = query) => {
    const keyword = value.trim()
    if (!keyword) return

    const isUrl = /^(https?:\/\/)?([\w-]+\.)+[\w-]{2,}/.test(keyword)
    if (isUrl) {
      openWithWhiteLoading(keyword.startsWith('http') ? keyword : `https://${keyword}`)
      return
    }

    setSearchHistory((prev) => {
      const next = [keyword, ...prev.filter((item) => item !== keyword)].slice(0, searchHistoryMax)
      try {
        localStorage.setItem(searchHistoryStorageKey, JSON.stringify(next))
      } catch {
        // ignore
      }
      return next
    })
    openWithWhiteLoading(`${engine.url}${encodeURIComponent(keyword)}`)
    setShowHistory(false)
  }

  const clearSearchHistory = () => {
    setSearchHistory([])
    try {
      localStorage.removeItem(searchHistoryStorageKey)
    } catch {
      // ignore
    }
  }

  return (
    <main className={`${showDefaultWallpaper ? 'default-wallpaper' : 'bg-transparent'} relative min-h-screen ${isScrollMode ? '' : 'overflow-hidden'} text-white`}>
      {wallpaperUrl && (
        <img
          src={wallpaperUrl}
          alt=""
          onLoad={() => {
            clearWallpaperFallbackTimer()
            setWallpaperLoaded(true)
          }}
          onError={() => {
            clearWallpaperFallbackTimer()
            setWallpaperLoaded(false)
            localStorage.removeItem(wallpaperStorageKey)
            setWallpaperUrl(null)
          }}
          className={`fixed inset-0 h-full w-full object-cover transition-opacity duration-700 ${wallpaperLoaded ? 'opacity-100' : 'opacity-0'}`}
          style={{
            filter: `blur(${homeSettings.wallpaperBlur}px)`,
            transform: homeSettings.wallpaperBlur > 0 ? 'scale(1.04)' : 'scale(1)',
          }}
        />
      )}
      {wallpaperLoaded && <div className="fixed inset-0 bg-black/10" />}

      <section className={`relative mx-auto flex min-h-screen max-w-6xl flex-col px-6 py-14 transition-opacity duration-500 ${backgroundReady ? 'opacity-100' : 'opacity-0'}`}>
        <Clock />

        <div
          className="relative mx-auto mt-10 h-16 w-full max-w-3xl cursor-text rounded-[1.75rem] bg-white px-5 text-slate-700 shadow-2xl"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) {
              event.preventDefault()
              searchInputRef.current?.focus()
            }
          }}
        >
          <div
            className="relative flex h-full items-center gap-4"
            onMouseDown={(event) => {
              if (event.target === event.currentTarget) {
                event.preventDefault()
                searchInputRef.current?.focus()
              }
            }}
          >
            <button
              type="button"
              onClick={() => {
                setShowEngines((value) => !value)
                setShowHistory(false)
              }}
              onBlur={() => window.setTimeout(() => setShowEngines(false), 140)}
              className="grid h-9 w-9 shrink-0 place-items-center rounded-xl bg-transparent outline-none transition hover:bg-slate-100/70"
              title="切换搜索引擎"
            >
              {engine.icon ? (
                <img src={engine.icon} alt="" className="h-[72%] w-[72%] object-contain" />
              ) : (
                <div className={`grid h-[72%] w-[72%] place-items-center rounded-lg bg-gradient-to-br ${engine.color} text-white`}>
                  <span className="text-[0.7em] font-black">{engine.shortName}</span>
                </div>
              )}
            </button>
            <input
              id="home-search-input"
              name="q"
              ref={searchInputRef}
              autoComplete="off"
              autoCorrect="off"
              autoCapitalize="off"
              spellCheck={false}
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              onFocus={() => {
                setShowHistory(true)
                setShowEngines(false)
              }}
              onBlur={() => window.setTimeout(() => setShowHistory(false), 140)}
              onKeyDown={(event) => {
                if (event.key === 'Enter') submitSearch()
              }}
              className="h-full w-full bg-transparent text-[1.25rem] leading-none text-slate-600 outline-none placeholder:text-slate-400"
              placeholder={`${engine.name} 搜索`}
            />
            <button
              type="button"
              onClick={() => submitSearch()}
              className="grid h-12 w-12 shrink-0 place-items-center rounded-xl bg-transparent text-slate-400 outline-none transition hover:bg-slate-100/70 hover:text-slate-600"
              title="搜索"
            >
              <Search className="h-[58%] w-[58%]" />
            </button>
          </div>

          {showEngines && (
            <div
              className="absolute left-0 right-0 top-[4.75rem] z-40 rounded-[2rem] bg-white px-8 py-7 shadow-2xl ring-1 ring-slate-200"
              onMouseDown={(event) => event.preventDefault()}
            >
              <div className="grid grid-cols-4 gap-x-5 gap-y-5 md:grid-cols-6 lg:grid-cols-8">
                {searchEngines.map((item) => (
                  <SearchEngineIcon
                    key={item.id}
                    engine={item}
                    active={item.id === engineId}
                    onClick={() => {
                      setEngineId(item.id)
                      setShowEngines(false)
                    }}
                  />
                ))}
                <button type="button" className="group flex w-16 flex-col items-center gap-1.5 outline-none">
                  <div className="grid h-14 w-14 place-items-center rounded-xl bg-slate-100 text-slate-500 transition duration-200 group-hover:-translate-y-1 group-hover:bg-slate-200">
                    <Plus className="h-6 w-6" />
                  </div>
                  <span className="text-xs text-slate-600">添加</span>
                </button>

              </div>
            </div>
          )}

          {showHistory && searchHistory.length > 0 && (
            <div className="absolute left-8 right-8 top-24 z-30 overflow-hidden rounded-2xl bg-white p-2 shadow-xl ring-1 ring-slate-200">
              <div className="flex items-center justify-between px-3 py-2 text-xs font-semibold uppercase tracking-wider text-slate-400">
                <span>历史搜索</span>
                <button
                  type="button"
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={clearSearchHistory}
                  className="rounded-md px-2 py-1 text-[11px] font-medium normal-case tracking-normal text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
                >
                  清空
                </button>
              </div>
              {searchHistory.map((item) => (
                <button
                  key={item}
                  type="button"
                  onMouseDown={(event) => event.preventDefault()}
                  onClick={() => {
                    setQuery(item)
                    submitSearch(item)
                  }}
                  className="flex w-full items-center gap-3 rounded-xl px-3 py-2 text-left text-sm text-slate-600 transition hover:bg-slate-100"
                >
                  <Globe2 className="h-4 w-4 text-slate-400" />
                  {item}
                </button>
              ))}
            </div>
          )}
        </div>

        {isScrollMode && (
          <div className="mx-auto mt-10 w-full max-w-5xl px-6 pb-24">
            <div className="space-y-12 pt-6">
              {groupedApps.groups.map(({ category, apps }) =>
                apps.length === 0 ? null : (
                  <section key={category.id}>
                    <h3 className="mb-4 px-2 text-sm font-semibold uppercase tracking-wider text-white/85 drop-shadow-[0_1px_2px_rgba(0,0,0,0.65)]">
                      {category.name}
                    </h3>
                    <div className="grid grid-cols-8 content-start justify-items-center gap-x-3 gap-y-8 px-2">
                      {apps.map((app) => (
                        <AppIcon key={app.id} app={app} />
                      ))}
                    </div>
                  </section>
                ),
              )}
              {groupedApps.uncategorized.length > 0 && (
                <section>
                  <h3 className="mb-4 px-2 text-sm font-semibold uppercase tracking-wider text-white/85 drop-shadow-[0_1px_2px_rgba(0,0,0,0.65)]">
                    未分类
                  </h3>
                  <div className="grid grid-cols-8 content-start justify-items-center gap-x-3 gap-y-8 px-2">
                    {groupedApps.uncategorized.map((app) => (
                      <AppIcon key={app.id} app={app} />
                    ))}
                  </div>
                </section>
              )}
            </div>
          </div>
        )}

      </section>

      {!isScrollMode && (
        <div
          className={`fixed inset-x-0 top-[17rem] bottom-24 z-20 transition-opacity duration-500 ${backgroundReady ? 'opacity-100' : 'opacity-0'}`}
        >
          <div
            ref={appsPagerRef}
            className="relative mx-auto h-full w-full max-w-5xl overflow-hidden px-6 select-none"
          >
            {appSlide && (
              <div
                key={`out-${appSlide.from}-${appSlide.axis}-${appSlide.dir}`}
                className={`absolute inset-x-6 inset-y-0 ${getSlideOutClass(appSlide.axis, appSlide.dir)}`}
              >
                <div className="grid grid-cols-8 content-start justify-items-center gap-x-3 gap-y-8 px-2 pt-6 pb-4">
                  {appPages[appSlide.from]?.map((app) => (
                    <AppIcon key={app.id} app={app} />
                  ))}
                </div>
              </div>
            )}
            <div
              key={`in-${appPage}-${appSlide?.axis ?? 'static'}-${appSlide?.dir ?? 0}`}
              className={`absolute inset-x-6 inset-y-0 ${appSlide ? getSlideInClass(appSlide.axis, appSlide.dir) : ''}`}
            >
              <div className="grid grid-cols-8 content-start justify-items-center gap-x-3 gap-y-8 px-2 pt-6 pb-4">
                {appPages[appPage]?.map((app) => (
                  <AppIcon key={app.id} app={app} />
                ))}
              </div>
            </div>
          </div>
        </div>
      )}

      {!isScrollMode && (
        layoutMode === 'paged-horizontal' ? (
          <div className="fixed bottom-7 left-1/2 z-30 flex -translate-x-1/2 flex-row items-center gap-2.5">
            {appPages.map((_, index) => {
              const active = index === appPage
              return (
                <button
                  key={index}
                  type="button"
                  onClick={() => goToAppPage(index, 'x')}
                  className={`h-2 rounded-full bg-white/55 shadow-md transition-all duration-300 hover:bg-white ${active ? 'w-6 bg-white' : 'w-2'}`}
                  title={`第 ${index + 1} 页`}
                />
              )
            })}
          </div>
        ) : (
          <div className="fixed left-3 top-1/2 z-30 flex -translate-y-1/2 flex-col items-center gap-2.5">
            {appPages.map((_, index) => {
              const active = index === appPage
              return (
                <button
                  key={index}
                  type="button"
                  onClick={() => goToAppPage(index, allowVertical ? 'y' : 'x')}
                  className={`w-2 rounded-full bg-white/55 shadow-md transition-all duration-300 hover:bg-white ${active ? 'h-6 bg-white' : 'h-2'}`}
                  title={`第 ${index + 1} 页`}
                />
              )
            })}
          </div>
        )
      )}

      {showSettings && (
        <div
          className="fixed inset-0 z-40 grid place-items-center bg-black/40 px-4 py-6 backdrop-blur-sm"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) setShowSettings(false)
          }}
        >
          <div className="flex h-[32rem] w-full max-w-3xl overflow-hidden rounded-3xl bg-white text-slate-700 shadow-2xl ring-1 ring-slate-200">
            <aside className="flex w-48 shrink-0 flex-col gap-1 border-r border-slate-100 bg-slate-50/80 p-4">
              <div className="mb-3 px-2 text-base font-bold text-slate-800">Home 设置</div>
              {[
                { id: 'appearance' as const, name: '外观', icon: ImageIcon },
                { id: 'layout' as const, name: '布局', icon: LayoutGrid },
                { id: 'categories' as const, name: '分类', icon: Tags },
                { id: 'search' as const, name: '搜索', icon: Search },
              ].map((tab) => {
                const Icon = tab.icon
                const active = settingsTab === tab.id
                return (
                  <button
                    key={tab.id}
                    type="button"
                    onClick={() => setSettingsTab(tab.id)}
                    className={`flex items-center gap-2.5 rounded-xl px-3 py-2 text-sm font-medium transition ${active ? 'bg-blue-500 text-white shadow-md shadow-blue-500/20' : 'text-slate-600 hover:bg-slate-200/60'}`}
                  >
                    <Icon className="h-4 w-4" />
                    {tab.name}
                  </button>
                )
              })}
            </aside>

            <div className="relative flex-1 overflow-y-auto p-6">
              <button
                type="button"
                onClick={() => setShowSettings(false)}
                className="absolute right-4 top-4 grid h-8 w-8 place-items-center rounded-full text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
                title="关闭"
              >
                <X className="h-4 w-4" />
              </button>

              {settingsTab === 'appearance' && (
                <div className="space-y-6">
                  <div>
                    <div className="text-base font-bold text-slate-800">外观</div>
                    <div className="mt-1 text-sm text-slate-500">调整主页背景</div>
                  </div>

                  <div>
                    <div className="mb-2 text-sm font-semibold text-slate-600">背景</div>
                    <div className="grid grid-cols-2 gap-2">
                      <button
                        type="button"
                        onClick={() => setHomeSettings((value) => ({ ...value, wallpaperMode: 'bing' }))}
                        className={`rounded-2xl px-4 py-3 text-sm font-semibold transition ${homeSettings.wallpaperMode === 'bing' ? 'bg-blue-500 text-white shadow-lg shadow-blue-500/25' : 'bg-slate-100 text-slate-600 hover:bg-slate-200'}`}
                      >
                        Bing 壁纸
                      </button>
                      <button
                        type="button"
                        onClick={() => setHomeSettings((value) => ({ ...value, wallpaperMode: 'default' }))}
                        className={`rounded-2xl px-4 py-3 text-sm font-semibold transition ${homeSettings.wallpaperMode === 'default' ? 'bg-blue-500 text-white shadow-lg shadow-blue-500/25' : 'bg-slate-100 text-slate-600 hover:bg-slate-200'}`}
                      >
                        默认背景
                      </button>
                    </div>
                  </div>

                  <div>
                    <div className="mb-2 text-sm font-semibold text-slate-600">清晰度</div>
                    <div className="grid grid-cols-2 gap-2">
                      <button
                        type="button"
                        onClick={() => setHomeSettings((value) => ({ ...value, wallpaperResolution: '1080' }))}
                        disabled={homeSettings.wallpaperMode === 'default'}
                        className={`rounded-2xl px-4 py-3 text-sm font-semibold transition disabled:cursor-not-allowed disabled:opacity-45 ${homeSettings.wallpaperResolution === '1080' ? 'bg-slate-800 text-white shadow-lg shadow-slate-800/20' : 'bg-slate-100 text-slate-600 hover:bg-slate-200'}`}
                      >
                        1080P
                      </button>
                      <button
                        type="button"
                        onClick={() => setHomeSettings((value) => ({ ...value, wallpaperResolution: 'uhd' }))}
                        disabled={homeSettings.wallpaperMode === 'default'}
                        className={`rounded-2xl px-4 py-3 text-sm font-semibold transition disabled:cursor-not-allowed disabled:opacity-45 ${homeSettings.wallpaperResolution === 'uhd' ? 'bg-slate-800 text-white shadow-lg shadow-slate-800/20' : 'bg-slate-100 text-slate-600 hover:bg-slate-200'}`}
                      >
                        4K / UHD
                      </button>
                    </div>
                  </div>

                  <label className="block">
                    <div className="mb-2 flex items-center justify-between text-sm font-semibold text-slate-600">
                      <span>模糊度</span>
                      <span className="text-slate-400">{homeSettings.wallpaperBlur}px</span>
                    </div>
                    <input
                      type="range"
                      min="0"
                      max="12"
                      step="1"
                      value={homeSettings.wallpaperBlur}
                      disabled={homeSettings.wallpaperMode === 'default'}
                      onChange={(event) => setHomeSettings((value) => ({ ...value, wallpaperBlur: Number(event.target.value) }))}
                      className="w-full accent-blue-500 disabled:opacity-45"
                    />
                  </label>
                </div>
              )}

              {settingsTab === 'layout' && (
                <div className="space-y-6">
                  <div>
                    <div className="text-base font-bold text-slate-800">布局</div>
                    <div className="mt-1 text-sm text-slate-500">控制应用页面的滑动方向</div>
                  </div>

                  <div className="grid grid-cols-2 gap-2">
                    {[
                      { id: 'paged-horizontal' as const, name: '左右翻页', desc: '仅允许左右滑动切换' },
                      { id: 'paged-vertical' as const, name: '上下翻页', desc: '仅允许上下滑动切换' },
                      { id: 'paged-free' as const, name: '自由翻页', desc: '上下/左右都可切换' },
                      { id: 'scroll' as const, name: '整页滚动', desc: '所有应用按分类纵向滚动' },
                    ].map((mode) => {
                      const active = homeSettings.layoutMode === mode.id
                      return (
                        <button
                          key={mode.id}
                          type="button"
                          onClick={() => setHomeSettings((value) => ({ ...value, layoutMode: mode.id }))}
                          className={`rounded-2xl px-4 py-3 text-left transition ${active ? 'bg-blue-500 text-white shadow-lg shadow-blue-500/25' : 'bg-slate-100 text-slate-600 hover:bg-slate-200'}`}
                        >
                          <div className="text-sm font-semibold">{mode.name}</div>
                          <div className={`mt-0.5 text-xs ${active ? 'text-white/85' : 'text-slate-400'}`}>{mode.desc}</div>
                        </button>
                      )
                    })}
                  </div>
                </div>
              )}

              {settingsTab === 'categories' && (
                <div className="space-y-5">
                  <div>
                    <div className="text-base font-bold text-slate-800">分类</div>
                    <div className="mt-1 text-sm text-slate-500">拖拽应用图标到对应分类，仅在「整页滚动」布局下分组显示</div>
                  </div>

                  <button
                    type="button"
                    onClick={addCategory}
                    className="flex items-center gap-2 rounded-xl bg-slate-100 px-3 py-2 text-sm font-semibold text-slate-600 transition hover:bg-slate-200"
                  >
                    <Plus className="h-4 w-4" />
                    新建分类
                  </button>

                  <div className="space-y-3">
                    {categories.map((cat) => {
                      const apps = demoApps.filter((app) => appCategoryMap[app.id] === cat.id)
                      const isOver = dragOverCategoryId === cat.id
                      const isEditing = editingCategoryId === cat.id
                      return (
                        <div
                          key={cat.id}
                          onDragOver={(event) => {
                            event.preventDefault()
                            setDragOverCategoryId(cat.id)
                          }}
                          onDragLeave={() => setDragOverCategoryId((prev) => (prev === cat.id ? null : prev))}
                          onDrop={(event) => {
                            event.preventDefault()
                            const appId = event.dataTransfer.getData('text/app-id')
                            setDragOverCategoryId(null)
                            if (appId) assignAppToCategory(appId, cat.id)
                          }}
                          className={`rounded-2xl border-2 border-dashed p-3 transition ${isOver ? 'border-blue-400 bg-blue-50' : 'border-slate-200 bg-slate-50/60'}`}
                        >
                          <div className="mb-2 flex items-center justify-between gap-2">
                            {isEditing ? (
                              <input
                                autoFocus
                                value={editingCategoryName}
                                onChange={(event) => setEditingCategoryName(event.target.value)}
                                onBlur={() => {
                                  renameCategory(cat.id, editingCategoryName)
                                  setEditingCategoryId(null)
                                }}
                                onKeyDown={(event) => {
                                  if (event.key === 'Enter') {
                                    renameCategory(cat.id, editingCategoryName)
                                    setEditingCategoryId(null)
                                  } else if (event.key === 'Escape') {
                                    setEditingCategoryId(null)
                                  }
                                }}
                                className="flex-1 rounded-lg border border-slate-200 bg-white px-2 py-1 text-sm text-slate-700 outline-none focus:border-blue-400"
                              />
                            ) : (
                              <div className="flex flex-1 items-center gap-2">
                                <span className="text-sm font-semibold text-slate-700">{cat.name}</span>
                                <span className="text-xs text-slate-400">{apps.length}</span>
                              </div>
                            )}
                            <button
                              type="button"
                              onClick={() => {
                                setEditingCategoryId(cat.id)
                                setEditingCategoryName(cat.name)
                              }}
                              className="grid h-7 w-7 place-items-center rounded-lg text-slate-400 transition hover:bg-slate-200 hover:text-slate-600"
                              title="重命名"
                            >
                              <Pencil className="h-3.5 w-3.5" />
                            </button>
                            <button
                              type="button"
                              onClick={() => deleteCategory(cat.id)}
                              className="grid h-7 w-7 place-items-center rounded-lg text-slate-400 transition hover:bg-rose-100 hover:text-rose-600"
                              title="删除分类"
                            >
                              <Trash2 className="h-3.5 w-3.5" />
                            </button>
                          </div>
                          <div className="flex flex-wrap gap-2">
                            {apps.length === 0 ? (
                              <span className="px-1 py-2 text-xs text-slate-400">拖拽应用到此处</span>
                            ) : (
                              apps.map((app) => {
                                const Icon = app.icon
                                return (
                                  <div
                                    key={app.id}
                                    draggable
                                    onDragStart={(event) => {
                                      event.dataTransfer.setData('text/app-id', app.id)
                                      event.dataTransfer.effectAllowed = 'move'
                                    }}
                                    className={`flex cursor-grab items-center gap-1.5 rounded-xl bg-white px-2.5 py-1.5 text-xs font-medium text-slate-600 shadow-sm ring-1 ring-slate-200 transition active:cursor-grabbing hover:shadow-md`}
                                  >
                                    <span className={`grid h-5 w-5 place-items-center rounded-md bg-gradient-to-br ${app.color} text-white`}>
                                      <Icon className="h-3 w-3" />
                                    </span>
                                    {app.name}
                                  </div>
                                )
                              })
                            )}
                          </div>
                        </div>
                      )
                    })}

                    <div
                      onDragOver={(event) => {
                        event.preventDefault()
                        setDragOverCategoryId('__uncategorized__')
                      }}
                      onDragLeave={() => setDragOverCategoryId((prev) => (prev === '__uncategorized__' ? null : prev))}
                      onDrop={(event) => {
                        event.preventDefault()
                        const appId = event.dataTransfer.getData('text/app-id')
                        setDragOverCategoryId(null)
                        if (appId) assignAppToCategory(appId, '')
                      }}
                      className={`rounded-2xl border-2 border-dashed p-3 transition ${dragOverCategoryId === '__uncategorized__' ? 'border-blue-400 bg-blue-50' : 'border-slate-200 bg-slate-50/60'}`}
                    >
                      <div className="mb-2 flex items-center gap-2">
                        <span className="text-sm font-semibold text-slate-700">未分类</span>
                        <span className="text-xs text-slate-400">{groupedApps.uncategorized.length}</span>
                      </div>
                      <div className="flex flex-wrap gap-2">
                        {groupedApps.uncategorized.length === 0 ? (
                          <span className="px-1 py-2 text-xs text-slate-400">拖拽应用到此处可移除分类</span>
                        ) : (
                          groupedApps.uncategorized.map((app) => {
                            const Icon = app.icon
                            return (
                              <div
                                key={app.id}
                                draggable
                                onDragStart={(event) => {
                                  event.dataTransfer.setData('text/app-id', app.id)
                                  event.dataTransfer.effectAllowed = 'move'
                                }}
                                className="flex cursor-grab items-center gap-1.5 rounded-xl bg-white px-2.5 py-1.5 text-xs font-medium text-slate-600 shadow-sm ring-1 ring-slate-200 transition active:cursor-grabbing hover:shadow-md"
                              >
                                <span className={`grid h-5 w-5 place-items-center rounded-md bg-gradient-to-br ${app.color} text-white`}>
                                  <Icon className="h-3 w-3" />
                                </span>
                                {app.name}
                              </div>
                            )
                          })
                        )}
                      </div>
                    </div>
                  </div>
                </div>
              )}

              {settingsTab === 'search' && (
                <div className="space-y-6">
                  <div>
                    <div className="text-base font-bold text-slate-800">搜索</div>
                    <div className="mt-1 text-sm text-slate-500">管理搜索历史与默认引擎</div>
                  </div>

                  <div>
                    <div className="mb-2 text-sm font-semibold text-slate-600">默认搜索引擎</div>
                    <div className="grid grid-cols-3 gap-2 sm:grid-cols-4">
                      {searchEngines.map((item) => {
                        const active = item.id === engineId
                        return (
                          <button
                            key={item.id}
                            type="button"
                            onClick={() => setEngineId(item.id)}
                            className={`flex items-center gap-2 rounded-xl px-3 py-2 text-sm font-medium transition ${active ? 'bg-blue-500 text-white shadow-md shadow-blue-500/20' : 'bg-slate-100 text-slate-600 hover:bg-slate-200'}`}
                          >
                            {item.icon ? (
                              <img src={item.icon} alt="" className="h-4 w-4 object-contain" />
                            ) : (
                              <span className="text-xs font-black">{item.shortName}</span>
                            )}
                            <span className="truncate">{item.name}</span>
                          </button>
                        )
                      })}
                    </div>
                  </div>

                  <div>
                    <div className="mb-2 flex items-center justify-between text-sm font-semibold text-slate-600">
                      <span>搜索历史</span>
                      <span className="text-xs font-normal text-slate-400">已保存 {searchHistory.length} 条</span>
                    </div>
                    <button
                      type="button"
                      onClick={clearSearchHistory}
                      disabled={searchHistory.length === 0}
                      className="rounded-2xl bg-slate-100 px-4 py-2.5 text-sm font-semibold text-slate-600 transition hover:bg-rose-100 hover:text-rose-600 disabled:cursor-not-allowed disabled:opacity-45 disabled:hover:bg-slate-100 disabled:hover:text-slate-600"
                    >
                      清空搜索历史
                    </button>
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      <div className="fixed bottom-7 right-7 z-30 flex flex-col gap-3">
        <button type="button" className="grid h-11 w-11 place-items-center rounded-full bg-white/15 text-white shadow-lg ring-1 ring-white/20 backdrop-blur-md transition hover:bg-white/25" title="公网 / 内网切换">
          <Home className="h-5 w-5" />
        </button>
        <button type="button" onClick={() => setShowSettings((value) => !value)} className="grid h-11 w-11 place-items-center rounded-full bg-white/15 text-white shadow-lg ring-1 ring-white/20 backdrop-blur-md transition hover:bg-white/25" title="Home 设置">
          <Settings className="h-5 w-5" />
        </button>
        <a href="/admin" className="grid h-11 w-11 place-items-center rounded-full bg-white/15 text-white shadow-lg ring-1 ring-white/20 backdrop-blur-md transition hover:bg-white/25" title="打开后台">
          <ExternalLink className="h-5 w-5" />
        </a>
      </div>
    </main>
  )
}

export default App
