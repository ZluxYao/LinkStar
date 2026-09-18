import { useEffect, useState, type ReactNode } from 'react'
import { Sidebar } from './Sidebar'
import { TopBar } from './TopBar'
import type { PageKey } from '../types'

interface AppShellProps {
  active: PageKey
  onChange: (k: PageKey) => void
  children: ReactNode
}

export function AppShell({ active, onChange, children }: AppShellProps) {
  // 窄屏（手机）上侧边栏是抽屉，默认收起，点汉堡按钮拉出来；
  // lg 以上它就是一直在那儿的固定栏，这个开关不起作用。
  const [navOpen, setNavOpen] = useState(false)

  // 抽屉开着时按 Esc 关掉
  useEffect(() => {
    if (!navOpen) return
    const onKey = (e: KeyboardEvent) => {
      if (e.key === 'Escape') setNavOpen(false)
    }
    document.addEventListener('keydown', onKey)
    return () => document.removeEventListener('keydown', onKey)
  }, [navOpen])

  return (
    <div className="flex min-h-screen bg-gradient-to-br from-slate-50 via-blue-50/30 to-indigo-50/30">
      {/* 抽屉拉出来时盖住内容，点一下关掉。lg 以上永不出现 */}
      {navOpen && (
        <div
          className="fixed inset-0 z-30 bg-slate-900/40 backdrop-blur-sm lg:hidden"
          onClick={() => setNavOpen(false)}
        />
      )}

      <Sidebar
        active={active}
        onChange={(k) => {
          onChange(k)
          setNavOpen(false) // 手机上选完一页就把抽屉收回去
        }}
        open={navOpen}
        onClose={() => setNavOpen(false)}
      />

      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar active={active} onMenu={() => setNavOpen(true)} />
        <main className="flex-1 overflow-y-auto px-4 py-4 sm:px-6 sm:py-6 lg:px-8">{children}</main>
      </div>
    </div>
  )
}
