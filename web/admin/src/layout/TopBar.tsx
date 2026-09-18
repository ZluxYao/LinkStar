import { useEffect, useRef, useState } from 'react'
import { Bell, HelpCircle, LogOut, Search } from 'lucide-react'
import type { PageKey } from '../types'
import { clearToken, isDesktop } from '../lib/api'
import { findNav } from './nav'

interface TopBarProps {
  active: PageKey
}

const pageMeta: Partial<Record<PageKey, { subtitle: string }>> = {
  dashboard: { subtitle: '一站式管理网络、域名、证书与服务，让一切安全、稳定、简单' },
  stun: { subtitle: '基于 STUN 的内网穿透与连接质量检测' },
  ddns: { subtitle: '动态域名解析与多服务商同步' },
}

export function TopBar({ active }: TopBarProps) {
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
    <header className="flex h-16 items-center gap-4 border-b border-slate-200/70 bg-white/70 px-8 backdrop-blur">
      <div className="flex-1">
        <div className="text-base font-bold text-slate-800">{nav?.label ?? ''}</div>
        {meta?.subtitle && (
          <div className="mt-0.5 text-xs text-slate-500">{meta.subtitle}</div>
        )}
      </div>

      <div className="relative w-72">
        <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" />
        <input
          placeholder="搜索设备、服务、域名..."
          className="h-9 w-full rounded-full border border-slate-200 bg-white pl-9 pr-3 text-sm text-slate-600 outline-none transition focus:border-blue-400 focus:ring-2 focus:ring-blue-100"
        />
      </div>

      <button
        type="button"
        className="relative grid h-9 w-9 place-items-center rounded-full text-slate-500 transition hover:bg-slate-100"
        title="通知"
      >
        <Bell className="h-4.5 w-4.5" />
        <span className="absolute right-1 top-1 grid h-4 min-w-4 place-items-center rounded-full bg-rose-500 px-1 text-[10px] font-bold text-white">
          3
        </span>
      </button>
      <button
        type="button"
        className="grid h-9 w-9 place-items-center rounded-full text-slate-500 transition hover:bg-slate-100"
        title="帮助"
      >
        <HelpCircle className="h-4.5 w-4.5" />
      </button>

      <div className="relative" ref={menuRef}>
        <button
          type="button"
          onClick={canLogout ? () => setMenuOpen((v) => !v) : undefined}
          aria-haspopup={canLogout || undefined}
          aria-expanded={canLogout ? menuOpen : undefined}
          className={`flex items-center gap-2 rounded-full bg-white px-1 py-1 ring-1 transition ${
            menuOpen ? 'ring-slate-300 bg-slate-50' : 'ring-slate-200'
          } ${canLogout ? 'hover:bg-slate-50' : 'cursor-default'}`}
        >
          <span className="grid h-7 w-7 place-items-center rounded-full bg-gradient-to-br from-slate-700 to-slate-900 text-xs font-bold text-white">
            A
          </span>
          <span className="pr-3 text-sm font-semibold text-slate-700">admin</span>
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
