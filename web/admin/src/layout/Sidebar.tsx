import { Sparkles, Star } from 'lucide-react'
import { navGroups } from './nav'
import type { PageKey } from '../types'

interface SidebarProps {
  active: PageKey
  onChange: (key: PageKey) => void
}

export function Sidebar({ active, onChange }: SidebarProps) {
  return (
    <aside className="flex h-screen w-60 shrink-0 flex-col border-r border-slate-200/70 bg-white/80 backdrop-blur">
      {/* Logo */}
      <div className="flex h-16 items-center gap-2 px-5">
        <div className="grid h-9 w-9 place-items-center rounded-xl bg-gradient-to-br from-sky-400 to-blue-600 text-white shadow-md shadow-blue-500/30">
          <Star className="h-5 w-5" fill="currentColor" />
        </div>
        <span className="text-lg font-bold tracking-tight text-slate-800">linkstar</span>
      </div>

      <nav className="flex-1 overflow-y-auto px-3 pb-4">
        {navGroups.map((group, gi) => (
          <div key={gi} className="mb-3">
            {group.title && (
              <div className="px-3 pb-1 pt-3 text-[11px] font-semibold uppercase tracking-wider text-slate-400">
                {group.title}
              </div>
            )}
            {group.items.map((item) => {
              const Icon = item.icon
              const isActive = item.key === active
              return (
                <button
                  key={item.key}
                  type="button"
                  onClick={() => onChange(item.key)}
                  className={`group mb-0.5 flex w-full items-center gap-2.5 rounded-xl px-3 py-2 text-sm transition ${
                    isActive
                      ? 'bg-gradient-to-r from-blue-500 to-indigo-500 text-white shadow-md shadow-blue-500/25'
                      : 'text-slate-600 hover:bg-slate-100'
                  }`}
                >
                  <Icon
                    className={`h-4 w-4 ${isActive ? 'text-white' : 'text-slate-400 group-hover:text-slate-600'}`}
                  />
                  <span className="flex-1 text-left font-medium">{item.label}</span>
                  {item.badge && (
                    <span
                      className={`rounded-md px-1.5 py-0.5 text-[10px] font-bold ${
                        isActive
                          ? 'bg-white/25 text-white'
                          : 'bg-blue-50 text-blue-600'
                      }`}
                    >
                      {item.badge}
                    </span>
                  )}
                </button>
              )
            })}
          </div>
        ))}
      </nav>

      {/* 推荐配置 CTA */}
      <div className="m-3 rounded-2xl bg-gradient-to-br from-blue-50 via-indigo-50 to-violet-50 p-4 ring-1 ring-blue-100/80">
        <div className="flex items-center gap-1.5 text-xs font-semibold text-blue-600">
          <Sparkles className="h-3.5 w-3.5" />
          推荐配置
        </div>
        <div className="mt-1 text-[11px] leading-relaxed text-slate-500">
          一键完成 DDNS + 证书 + 反代
        </div>
        <button
          type="button"
          className="mt-3 w-full rounded-lg bg-gradient-to-r from-blue-500 to-indigo-500 px-3 py-2 text-xs font-semibold text-white shadow-md shadow-blue-500/25 transition hover:shadow-lg"
        >
          开始配置向导
        </button>
      </div>

      <div className="px-5 pb-4 text-[11px] text-slate-400">
        © 2026 linkstar
        <div>v1.0.0</div>
      </div>
    </aside>
  )
}
