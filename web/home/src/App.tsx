import { useEffect, useMemo, useState } from 'react'
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
  Languages,
  MessageCircle,
  MonitorCog,
  Music2,
  NotebookText,
  Plus,
  Search,
  Settings,
  Shield,
  Terminal,
  Wifi,
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
  { id: 'yahoo', name: 'Yahoo', shortName: 'Y', url: 'https://search.yahoo.com/search?p=', color: 'from-purple-500 to-violet-800' },
]

const searchHistory = ['LinkStar DDNS 配置', 'Cloudflare API Token', 'NAS 反向代理', 'STUN UDP 穿透', 'React Tailwind dashboard']

const bingWallpaperUrl = '/api/home/bing-wallpaper'

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
    <button type="button" onClick={onClick} className="group flex w-20 flex-col items-center gap-2 outline-none">
      <div className={`grid h-16 w-16 place-items-center rounded-2xl bg-slate-100 text-white shadow-sm transition duration-200 group-hover:-translate-y-1 group-hover:shadow-lg ${active ? 'ring-4 ring-blue-200' : 'ring-1 ring-slate-200'}`}>
        {engine.icon ? (
          <img src={engine.icon} alt="" className="h-11 w-11 object-contain" />
        ) : (
          <div className={`grid h-full w-full place-items-center rounded-2xl bg-gradient-to-br ${engine.color}`}>
            <span className="text-xl font-black tracking-tight">{engine.shortName}</span>
          </div>
        )}
      </div>
      <span className={`max-w-20 truncate text-sm ${active ? 'font-semibold text-blue-600' : 'text-slate-600'}`}>{engine.name}</span>
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
  const [showHistory, setShowHistory] = useState(false)
  const [showEngines, setShowEngines] = useState(false)
  const [wallpaperUrl, setWallpaperUrl] = useState<string | null>(null)
  const [wallpaperLoaded, setWallpaperLoaded] = useState(false)

  const engine = useMemo(() => searchEngines.find((item) => item.id === engineId) ?? searchEngines[0], [engineId])

  useEffect(() => {
    setWallpaperLoaded(false)
    setWallpaperUrl(`${bingWallpaperUrl}?t=${Date.now()}`)
  }, [])

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

    openWithWhiteLoading(`${engine.url}${encodeURIComponent(keyword)}`)
    setShowHistory(false)
  }

  return (
    <main className="relative min-h-screen overflow-hidden bg-white text-white">
      {wallpaperUrl && (
        <img
          src={wallpaperUrl}
          alt=""
          onLoad={() => setWallpaperLoaded(true)}
          onError={() => {
            setWallpaperLoaded(false)
            setWallpaperUrl(null)
          }}
          className={`absolute inset-0 h-full w-full scale-105 object-cover blur-[2px] transition-opacity duration-700 ${wallpaperLoaded ? 'opacity-100' : 'opacity-0'}`}
        />
      )}
      {wallpaperLoaded && <div className="absolute inset-0 bg-black/10" />}

      <section className="relative z-10 mx-auto flex min-h-screen max-w-6xl flex-col px-6 py-14">
        <Clock />

        <div className="relative mx-auto mt-10 w-full max-w-3xl rounded-[1.75rem] bg-white px-5 py-4 text-slate-700 shadow-2xl">
          <div className="relative flex items-center gap-4">
            <button
              type="button"
              onClick={() => {
                setShowEngines((value) => !value)
                setShowHistory(false)
              }}
              onBlur={() => window.setTimeout(() => setShowEngines(false), 140)}
              className="grid aspect-square h-[2.25em] shrink-0 place-items-center rounded-xl bg-transparent outline-none transition hover:bg-slate-100/70"
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
              className="w-full bg-transparent text-xl text-slate-600 outline-none placeholder:text-slate-400"
              placeholder={`${engine.name} 搜索`}
            />
            <button
              type="button"
              onClick={() => submitSearch()}
              className="grid aspect-square h-[2.25em] shrink-0 place-items-center rounded-xl bg-transparent text-slate-400 outline-none transition hover:bg-slate-100/70 hover:text-slate-600"
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
              <div className="mb-7 flex items-center gap-3 text-2xl font-medium text-slate-500">
                <div className="grid h-8 w-8 place-items-center rounded-lg bg-slate-100">
                  {engine.icon ? (
                    <img src={engine.icon} alt="" className="h-6 w-6 object-contain" />
                  ) : (
                    <div className={`grid h-full w-full place-items-center rounded-lg bg-gradient-to-br ${engine.color} text-white`}>
                      <span className="text-xs font-black">{engine.shortName}</span>
                    </div>
                  )}
                </div>
                {engine.name} 搜索
              </div>
              <div className="grid grid-cols-4 gap-x-8 gap-y-7 md:grid-cols-6 lg:grid-cols-8">
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
                <button type="button" className="group flex w-20 flex-col items-center gap-2 outline-none">
                  <div className="grid h-16 w-16 place-items-center rounded-2xl bg-slate-100 text-slate-500 transition duration-200 group-hover:-translate-y-1 group-hover:bg-slate-200">
                    <Plus className="h-8 w-8" />
                  </div>
                  <span className="text-sm text-slate-600">添加</span>
                </button>
              </div>
            </div>
          )}

          {showHistory && (
            <div className="absolute left-8 right-8 top-24 z-30 overflow-hidden rounded-2xl bg-white p-2 shadow-xl ring-1 ring-slate-200">
              <div className="px-3 py-2 text-xs font-semibold uppercase tracking-wider text-slate-400">历史搜索</div>
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

        <div className="mx-auto mt-10 w-full max-w-5xl">
          <div className="mb-5 flex items-center gap-2 text-sm font-semibold text-white/85 drop-shadow">
            <Wifi className="h-4 w-4" />
            我的服务
          </div>
          <div className="grid grid-cols-4 justify-items-center gap-x-6 gap-y-7 sm:grid-cols-6 lg:grid-cols-8">
            {demoApps.map((app) => (
              <AppIcon key={app.id} app={app} />
            ))}
          </div>
        </div>
      </section>

      <div className="fixed bottom-7 right-7 z-20 flex flex-col gap-3">
        <button className="grid h-11 w-11 place-items-center rounded-full bg-white/15 text-white shadow-lg ring-1 ring-white/20 backdrop-blur-md transition hover:bg-white/25" title="公网 / 内网切换">
          <Home className="h-5 w-5" />
        </button>
        <a href="/admin" className="grid h-11 w-11 place-items-center rounded-full bg-white/15 text-white shadow-lg ring-1 ring-white/20 backdrop-blur-md transition hover:bg-white/25" title="后台控制">
          <Settings className="h-5 w-5" />
        </a>
        <a href="/admin" className="grid h-11 w-11 place-items-center rounded-full bg-white/15 text-white shadow-lg ring-1 ring-white/20 backdrop-blur-md transition hover:bg-white/25" title="打开后台">
          <ExternalLink className="h-5 w-5" />
        </a>
      </div>
    </main>
  )
}

export default App
