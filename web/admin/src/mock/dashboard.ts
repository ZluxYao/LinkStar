export interface StatCard {
  key: string
  label: string
  value: number
  total: number
  delta: number // 本周新增 / - 即将到期
  deltaLabel: string
  tone: 'blue' | 'emerald' | 'violet' | 'amber' | 'cyan'
  trend: number[]
  iconKey: 'device' | 'service' | 'ddns' | 'cert' | 'rule'
}

export const stats: StatCard[] = [
  {
    key: 'device',
    label: '在线设备',
    value: 12,
    total: 15,
    delta: 2,
    deltaLabel: '本周新增',
    tone: 'blue',
    iconKey: 'device',
    trend: [4, 6, 5, 8, 7, 9, 12],
  },
  {
    key: 'service',
    label: '运行服务',
    value: 18,
    total: 24,
    delta: 3,
    deltaLabel: '本周新增',
    tone: 'emerald',
    iconKey: 'service',
    trend: [8, 9, 10, 12, 14, 16, 18],
  },
  {
    key: 'ddns',
    label: 'DDNS 域名',
    value: 8,
    total: 12,
    delta: 1,
    deltaLabel: '本周新增',
    tone: 'violet',
    iconKey: 'ddns',
    trend: [3, 4, 4, 5, 6, 7, 8],
  },
  {
    key: 'cert',
    label: '有效证书',
    value: 6,
    total: 9,
    delta: -1,
    deltaLabel: '即将到期',
    tone: 'amber',
    iconKey: 'cert',
    trend: [9, 9, 8, 8, 7, 7, 6],
  },
  {
    key: 'rule',
    label: '反代规则',
    value: 15,
    total: 18,
    delta: 2,
    deltaLabel: '本周新增',
    tone: 'cyan',
    iconKey: 'rule',
    trend: [6, 8, 9, 10, 12, 13, 15],
  },
]

export interface TopologyNode {
  id: string
  label: string
  addr: string
  kind: 'local' | 'router' | 'public'
}

export const topology: TopologyNode[] = [
  { id: 'local', label: '本机 IP', addr: '192.168.100.151', kind: 'local' },
  { id: 'r1', label: '路由 NAT 1', addr: '192.168.100.1', kind: 'router' },
  { id: 'r2', label: '路由 NAT 2', addr: '192.168.31.1', kind: 'router' },
  { id: 'pub', label: '公网 IP', addr: '14.19.69.156', kind: 'public' },
]

export interface DeviceRow {
  name: string
  ip: string
  status: 'on' | 'off'
  badge: '本机' | '在线' | '离线'
}

export const devices: DeviceRow[] = [
  { name: '本机', ip: '127.0.0.1', status: 'on', badge: '本机' },
  { name: 'Nas', ip: '192.168.100.151', status: 'on', badge: '在线' },
  { name: 'ubuntu desk', ip: '192.168.100.164', status: 'on', badge: '在线' },
  { name: 'istore', ip: '192.168.100.1', status: 'on', badge: '在线' },
]

export interface ServiceRow {
  name: string
  protocol: string
  port: number
  status: 'running' | 'abnormal'
}

export const services: ServiceRow[] = [
  { name: 'sunpanl', protocol: 'TCP', port: 7000, status: 'running' },
  { name: 'istore', protocol: 'TCP', port: 80, status: 'running' },
  { name: 'fnso', protocol: 'TCP', port: 250, status: 'running' },
  { name: 'nas', protocol: 'TCP', port: 445, status: 'abnormal' },
  { name: '监控服务', protocol: 'TCP', port: 9090, status: 'running' },
]

export interface DdnsRow {
  domain: string
  recordType: 'A' | 'AAAA' | 'CNAME'
  target: string
  status: 'normal' | 'pending' | 'failed'
}

export const ddnsRows: DdnsRow[] = [
  { domain: 'linkstar.ddns.net', recordType: 'A', target: '14.19.69.156', status: 'normal' },
  { domain: 'home.linkstar.ddns.net', recordType: 'A', target: '14.19.69.156', status: 'normal' },
  { domain: 'office.linkstar.ddns.net', recordType: 'A', target: '14.19.69.156', status: 'normal' },
  { domain: 'nas.linkstar.ddns.net', recordType: 'A', target: '14.19.69.156', status: 'normal' },
  { domain: 'test.linkstar.ddns.net', recordType: 'A', target: '-', status: 'failed' },
]

export interface CertRow {
  domain: string
  daysLeft: number
  expireOn: string
  status: 'expiring' | 'normal'
}

export const certs: CertRow[] = [
  { domain: '*.linkstar.com', daysLeft: 15, expireOn: '2026-06-20 到期', status: 'expiring' },
  { domain: 'api.linkstar.com', daysLeft: 30, expireOn: '2026-07-06 到期', status: 'expiring' },
  { domain: 'admin.linkstar.com', daysLeft: 66, expireOn: '2026-08-05 到期', status: 'normal' },
  { domain: '*.linkstar.ddns.net', daysLeft: 120, expireOn: '2026-09-29 到期', status: 'normal' },
]

export interface AuditLog {
  ago: string
  message: string
  tag: 'DDNS' | '反代' | '证书' | '设备' | '服务'
}

export const auditLogs: AuditLog[] = [
  { ago: '1 分钟前', message: 'admin 更新了 DDNS 域名 linkstar.ddns.net', tag: 'DDNS' },
  { ago: '3 分钟前', message: 'admin 创建了反代规则 api.linkstar.com', tag: '反代' },
  { ago: '5 分钟前', message: 'admin 申请了新证书 *.linkstar.com', tag: '证书' },
  { ago: '10 分钟前', message: 'system 设备 Nas 上线', tag: '设备' },
  { ago: '15 分钟前', message: 'system 服务 sunpanl 启动成功', tag: '服务' },
]

export interface SystemMetric {
  label: string
  value: number
  tone: 'blue' | 'emerald' | 'amber' | 'violet' | 'cyan'
  unit?: string
  display: string
  trend: number[]
}

export const systemMetrics: SystemMetric[] = [
  { label: 'CPU 使用率', value: 23, tone: 'blue', display: '23%', trend: [10, 12, 18, 15, 22, 20, 23] },
  { label: '内存使用率', value: 45, tone: 'emerald', display: '45%', trend: [30, 32, 36, 40, 42, 44, 45] },
  { label: '磁盘使用率', value: 31, tone: 'amber', display: '31%', trend: [28, 28, 29, 30, 30, 31, 31] },
  { label: '网络出站流量', value: 80, tone: 'violet', display: '1.87 GB/s', trend: [40, 55, 60, 70, 76, 82, 80] },
  { label: '网络入站流量', value: 85, tone: 'cyan', display: '2.34 GB/s', trend: [50, 60, 65, 75, 80, 88, 85] },
]
