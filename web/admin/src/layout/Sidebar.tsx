import { useEffect, useState } from 'react'
import { X } from 'lucide-react'
import logo from '../assets/logo.png'
import { navGroups } from './nav'
import { getVersion } from '../lib/api'
import type { PageKey } from '../types'

interface SidebarProps {
  active: PageKey
  onChange: (key: PageKey) => void
  /** 窄屏抽屉是否拉出来了；lg 以上忽略 */
  open: boolean
  onClose: () => void
}

export function Sidebar({ active, onChange, open, onClose }: SidebarProps) {
  const [version, setVersion] = useState('0.6.4')

  useEffect(() => {
    getVersion()
      .then((data) => {
        if (data?.version) setVersion(data.version)
      })
      .catch(() => {
        // 忽略错误，使用默认版本号
      })
  }, [])

  return (
    <aside
      className={`fixed inset-y-0 left-0 z-40 flex h-screen w-60 shrink-0 flex-col border-r border-slate-200/70 bg-white/95 backdrop-blur transition-transform duration-200 lg:static lg:z-auto lg:translate-x-0 lg:bg-white/80 ${
        open ? 'translate-x-0 shadow-2xl' : '-translate-x-full'
      }`}
    >
      {/* Logo。点 logo 回导航主页，跟大部分后台一个习惯 */}
      <div className="flex h-16 items-center gap-2 px-5">
        <a
          href="/"
          title="回到导航主页"
          className="-mx-1.5 flex min-w-0 items-center gap-2 rounded-lg px-1.5 py-1 transition hover:bg-slate-100"
        >
          <img src={logo} alt="LinkStar" className="h-9 w-9" />
          <span className="truncate text-lg font-bold tracking-tight text-slate-800">linkstar</span>
        </a>
        <button
          type="button"
          onClick={onClose}
          title="关闭菜单"
          className="ml-auto grid h-8 w-8 place-items-center rounded-lg text-slate-400 transition hover:bg-slate-100 hover:text-slate-600 lg:hidden"
        >
          <X className="h-4 w-4" />
        </button>
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

      <div className="px-5 pb-4 text-[11px] text-slate-400">
        © 2026 linkstar
        <div>v{version}</div>
      </div>
    </aside>
  )
}
