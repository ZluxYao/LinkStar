import {
  Bell,
  FileText,
  Globe,
  LayoutDashboard,
  Repeat,
  Settings,
  ShieldCheck,
  Users,
  Waves,
  type LucideIcon,
} from 'lucide-react'
import type { PageKey } from '../types'

export interface NavItem {
  key: PageKey
  label: string
  icon: LucideIcon
  badge?: 'NEW'
}

export interface NavGroup {
  title?: string
  items: NavItem[]
}

export const navGroups: NavGroup[] = [
  {
    items: [{ key: 'dashboard', label: '仪表盘', icon: LayoutDashboard }],
  },
  {
    title: '网络与连接',
    items: [
      { key: 'stun', label: 'STUN 内网穿透', icon: Waves, badge: 'NEW' },
      { key: 'ddns', label: 'DDNS 域名解析', icon: Globe },
      { key: 'reverse-proxy', label: '反向代理', icon: Repeat },
      { key: 'cert', label: '证书管理', icon: ShieldCheck },
    ],
  },
  {
    title: '系统与管理',
    items: [
      { key: 'user', label: '用户管理', icon: Users },
      { key: 'settings', label: '系统设置', icon: Settings },
      { key: 'audit', label: '日志审计', icon: FileText },
      { key: 'notify', label: '通知设置', icon: Bell },
    ],
  },
]

export const findNav = (key: PageKey): NavItem | undefined => {
  for (const g of navGroups) {
    const found = g.items.find((it) => it.key === key)
    if (found) return found
  }
  return undefined
}
