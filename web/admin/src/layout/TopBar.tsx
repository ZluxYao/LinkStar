import { Bell, CircleCheck, HelpCircle, Search } from 'lucide-react'
import type { PageKey } from '../types'
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
  return (
    <header className="flex h-16 items-center gap-4 border-b border-slate-200/70 bg-white/70 px-8 backdrop-blur">
      <div className="flex-1">
        <div className="text-base font-bold text-slate-800">{nav?.label ?? ''}</div>
        {meta?.subtitle && (
          <div className="mt-0.5 text-xs text-slate-500">{meta.subtitle}</div>
        )}
      </div>

      <div className="flex items-center gap-2.5 rounded-full bg-emerald-50 px-3 py-1.5 text-xs font-semibold text-emerald-600 ring-1 ring-emerald-100">
        <CircleCheck className="h-4 w-4" />
        <span className="text-slate-500">系统状态</span>
        <span>正常</span>
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

      <button
        type="button"
        className="flex items-center gap-2 rounded-full bg-white px-1 py-1 ring-1 ring-slate-200 transition hover:bg-slate-50"
      >
        <span className="grid h-7 w-7 place-items-center rounded-full bg-gradient-to-br from-slate-700 to-slate-900 text-xs font-bold text-white">
          A
        </span>
        <span className="pr-3 text-sm font-semibold text-slate-700">admin</span>
      </button>
    </header>
  )
}
