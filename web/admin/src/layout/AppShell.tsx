import type { ReactNode } from 'react'
import { Sidebar } from './Sidebar'
import { TopBar } from './TopBar'
import type { PageKey } from '../types'

interface AppShellProps {
  active: PageKey
  onChange: (k: PageKey) => void
  children: ReactNode
}

export function AppShell({ active, onChange, children }: AppShellProps) {
  return (
    <div className="flex min-h-screen bg-gradient-to-br from-slate-50 via-blue-50/30 to-indigo-50/30">
      <Sidebar active={active} onChange={onChange} />
      <div className="flex min-w-0 flex-1 flex-col">
        <TopBar active={active} />
        <main className="flex-1 overflow-y-auto px-8 py-6">{children}</main>
      </div>
    </div>
  )
}
