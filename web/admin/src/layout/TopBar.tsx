import { useEffect, useRef, useState } from 'react'
import { Home, LogOut, Menu } from 'lucide-react'
import type { PageKey } from '../types'
import { clearToken, isDesktop } from '../lib/api'
import { findNav } from './nav'

interface TopBarProps {
  active: PageKey
  /** 窄屏上拉出侧边栏抽屉 */
  onMenu: () => void
}

const pageMeta: Partial<Record<PageKey, { subtitle: string }>> = {
  dashboard: { subtitle: '一站式管理网络、域名、证书与服务，让一切安全、稳定、简单' },
  stun: { subtitle: '基于 STUN 的内网穿透与连接质量检测' },
  ddns: { subtitle: '动态域名解析与多服务商同步' },
}

export function TopBar({ active, onMenu }: TopBarProps) {
  const nav = findNav(active)
  const meta = pageMeta[active]

  // 桌面版靠 sessionStorage 里的 desktop secret 鉴权，清 token 也退不出去，就别给这个入口
  const canLogout = !isDesktop()
  const [menuOpen, setMenuOpen] = useState(false)
  const menuRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    if (!menuOpen) return
    const onDown = (e: MouseEvent) => {
      if (!menuRef.current?.contains(e.target as Node)) setMenuOpen(false)
    }
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setMenuOpen(false)
    }
    document.addEventListener('mousedown', onDown)
    document.addEventListener('keydown', onKey)
    return () => {
      document.removeEventListener('mousedown', onDown)
      document.removeEventListener('keydown', onKey)
    }
  }, [menuOpen])

  // 整页重载，顺带把 SSE 连接和各页缓存的配置一起丢掉
  const logout = () => {
    clearToken()
    window.location.reload()
  }

  return (
    <header className="flex h-16 items-center gap-2 border-b border-slate-200/70 bg-white/70 px-4 backdrop-blur sm:gap-4 sm:px-6 lg:px-8">
      {/* 汉堡：只有窄屏才有，侧边栏这时候是抽屉 */}
      <button
        type="button"
        onClick={onMenu}
        title="菜单"
        className="-ml-1 grid h-9 w-9 shrink-0 place-items-center rounded-lg text-slate-500 transition hover:bg-slate-100 lg:hidden"
      >
        <Menu className="h-5 w-5" />
      </button>

      <div className="min-w-0 flex-1">
        <div className="truncate text-base font-bold text-slate-800">{nav?.label ?? ''}</div>
        {/* 副标题在手机上占不下，藏起来 */}
        {meta?.subtitle && (
          <div className="mt-0.5 hidden truncate text-xs text-slate-500 sm:block">
            {meta.subtitle}
          </div>
        )}
      </div>

      {/* 回导航主页。整页跳转，不走前端路由 —— 导航主页是另一个应用，
          跟后台不共用一份 JS。桌面版同理，webview 直接换地址就行 */}
      <a
        href="/"
        title="回到导航主页"
        className="flex shrink-0 items-center gap-1.5 rounded-lg px-2 py-1.5 text-sm font-medium text-slate-500 transition hover:bg-slate-100 hover:text-slate-700 sm:px-2.5"
      >
        <Home className="h-4 w-4" />
        <span className="hidden sm:inline">导航主页</span>
      </a>

      <div className="relative" ref={menuRef}>
        <button
          type="button"
          onClick={canLogout ? () => setMenuOpen((v) => !v) : undefined}
          aria-haspopup={canLogout || undefined}
          aria-expanded={canLogout ? menuOpen : undefined}
          className={`flex shrink-0 items-center gap-2 rounded-full bg-white p-1 ring-1 transition ${
            menuOpen ? 'ring-slate-300 bg-slate-50' : 'ring-slate-200'
          } ${canLogout ? 'hover:bg-slate-50' : 'cursor-default'}`}
        >
          <span className="grid h-7 w-7 place-items-center rounded-full bg-gradient-to-br from-slate-700 to-slate-900 text-xs font-bold text-white">
            A
          </span>
          {/* 手机上只留头像，名字藏掉 */}
          <span className="hidden pr-2 text-sm font-semibold text-slate-700 sm:inline">admin</span>
        </button>

        {menuOpen && (
          <div className="absolute right-0 top-full z-20 mt-2 w-36 overflow-hidden rounded-xl border border-slate-200 bg-white py-1 shadow-lg shadow-slate-900/5">
            <button
              type="button"
              onClick={logout}
              className="flex w-full items-center gap-2 px-3 py-2 text-sm text-slate-600 transition hover:bg-slate-50"
            >
              <LogOut className="h-4 w-4 text-slate-400" />
              退出登录
            </button>
          </div>
        )}
      </div>
    </header>
  )
}
