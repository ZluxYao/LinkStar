import {
  ChevronDown,
  CircleCheck,
  Cloud,
  Globe,
  Pencil,
  Plus,
  RefreshCw,
  Search,
  Server,
  Trash2,
  TriangleAlert,
  type LucideIcon,
} from 'lucide-react'
import { useMemo, useState } from 'react'
import { Card, CardHeader } from '../components/Card'

type Provider = 'Cloudflare' | '阿里云' | 'DNSPod' | 'NameSilo'
type RecordType = 'A' | 'AAAA' | 'CNAME' | 'TXT'
type Status = 'normal' | 'pending' | 'failed'

interface DdnsRecord {
  id: string
  domain: string
  recordType: RecordType
  target: string
  ttl: number
  provider: Provider
  status: Status
  lastUpdate: string
  autoSync: boolean
}

const records: DdnsRecord[] = [
  {
    id: '1',
    domain: 'linkstar.ddns.net',
    recordType: 'A',
    target: '14.19.69.156',
    ttl: 120,
    provider: 'Cloudflare',
    status: 'normal',
    lastUpdate: '1 分钟前',
    autoSync: true,
  },
  {
    id: '2',
    domain: 'home.linkstar.ddns.net',
    recordType: 'A',
    target: '14.19.69.156',
    ttl: 120,
    provider: 'Cloudflare',
    status: 'normal',
    lastUpdate: '2 分钟前',
    autoSync: true,
  },
  {
    id: '3',
    domain: 'office.linkstar.ddns.net',
    recordType: 'A',
    target: '14.19.69.156',
    ttl: 300,
    provider: '阿里云',
    status: 'normal',
    lastUpdate: '5 分钟前',
    autoSync: true,
  },
  {
    id: '4',
    domain: 'nas.linkstar.ddns.net',
    recordType: 'A',
    target: '14.19.69.156',
    ttl: 120,
    provider: 'Cloudflare',
    status: 'normal',
    lastUpdate: '8 分钟前',
    autoSync: true,
  },
  {
    id: '5',
    domain: 'ipv6.linkstar.ddns.net',
    recordType: 'AAAA',
    target: '240e:43:c000::1:abcd',
    ttl: 120,
    provider: 'DNSPod',
    status: 'pending',
    lastUpdate: '正在同步',
    autoSync: true,
  },
  {
    id: '6',
    domain: 'test.linkstar.ddns.net',
    recordType: 'A',
    target: '-',
    ttl: 120,
    provider: 'Cloudflare',
    status: 'failed',
    lastUpdate: '失败 (DNS API 鉴权错误)',
    autoSync: false,
  },
]

const providers: { name: Provider; tone: string }[] = [
  { name: 'Cloudflare', tone: 'bg-orange-50 text-orange-600 ring-orange-200' },
  { name: '阿里云', tone: 'bg-blue-50 text-blue-600 ring-blue-200' },
  { name: 'DNSPod', tone: 'bg-cyan-50 text-cyan-600 ring-cyan-200' },
  { name: 'NameSilo', tone: 'bg-violet-50 text-violet-600 ring-violet-200' },
]

const statusInfo: Record<Status, { label: string; tone: string; icon: LucideIcon }> = {
  normal: { label: '正常', tone: 'bg-emerald-50 text-emerald-600', icon: CircleCheck },
  pending: { label: '更新中', tone: 'bg-amber-50 text-amber-600', icon: RefreshCw },
  failed: { label: '更新失败', tone: 'bg-rose-50 text-rose-500', icon: TriangleAlert },
}

export function Ddns() {
  const [query, setQuery] = useState('')
  const [filterStatus, setFilterStatus] = useState<'all' | Status>('all')

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return records.filter((r) => {
      if (filterStatus !== 'all' && r.status !== filterStatus) return false
      if (q && !r.domain.toLowerCase().includes(q) && !r.target.toLowerCase().includes(q)) return false
      return true
    })
  }, [query, filterStatus])

  const total = records.length
  const normal = records.filter((r) => r.status === 'normal').length
  const failed = records.filter((r) => r.status === 'failed').length

  return (
    <div className="space-y-4">
      {/* 顶部概览 */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Card>
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-blue-50 text-blue-500">
              <Globe className="h-5 w-5" />
            </div>
            <div>
              <div className="text-xs text-slate-500">域名总数</div>
              <div className="text-2xl font-bold text-slate-800">{total}</div>
            </div>
          </div>
        </Card>
        <Card>
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-emerald-50 text-emerald-500">
              <CircleCheck className="h-5 w-5" />
            </div>
            <div>
              <div className="text-xs text-slate-500">正常解析</div>
              <div className="text-2xl font-bold text-slate-800">{normal}</div>
            </div>
          </div>
        </Card>
        <Card>
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-rose-50 text-rose-500">
              <TriangleAlert className="h-5 w-5" />
            </div>
            <div>
              <div className="text-xs text-slate-500">异常域名</div>
              <div className="text-2xl font-bold text-slate-800">{failed}</div>
            </div>
          </div>
        </Card>
        <Card>
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-violet-50 text-violet-500">
              <Cloud className="h-5 w-5" />
            </div>
            <div>
              <div className="text-xs text-slate-500">已接入服务商</div>
              <div className="text-2xl font-bold text-slate-800">3</div>
            </div>
          </div>
        </Card>
      </div>

      {/* 服务商 */}
      <Card>
        <CardHeader
          title="DNS 服务商"
          action={
            <button className="flex items-center gap-1 rounded-md bg-blue-500 px-2.5 py-1 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 hover:bg-blue-600">
              <Plus className="h-3 w-3" /> 添加凭证
            </button>
          }
        />
        <div className="flex flex-wrap gap-2">
          {providers.map((p) => (
            <div
              key={p.name}
              className={`flex items-center gap-2 rounded-xl px-3 py-2 text-xs font-semibold ring-1 ${p.tone}`}
            >
              <Server className="h-3.5 w-3.5" />
              {p.name}
            </div>
          ))}
        </div>
      </Card>

      {/* 域名列表 */}
      <Card className="!p-0">
        <div className="flex flex-wrap items-center gap-3 border-b border-slate-100 px-5 py-4">
          <div className="text-sm font-bold text-slate-800">域名解析列表</div>
          <div className="relative ml-auto w-64">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-3.5 w-3.5 -translate-y-1/2 text-slate-400" />
            <input
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="搜索域名或 IP"
              className="h-8 w-full rounded-full border border-slate-200 bg-white pl-8 pr-3 text-xs outline-none focus:border-blue-400"
            />
          </div>
          <button
            type="button"
            className="flex items-center gap-1 rounded-md bg-white px-2 py-1 text-xs font-semibold text-slate-600 ring-1 ring-slate-200 hover:bg-slate-50"
            onClick={() =>
              setFilterStatus((p) => (p === 'all' ? 'normal' : p === 'normal' ? 'pending' : p === 'pending' ? 'failed' : 'all'))
            }
          >
            状态: {filterStatus === 'all' ? '全部' : statusInfo[filterStatus].label}
            <ChevronDown className="h-3 w-3" />
          </button>
          <button className="flex items-center gap-1 rounded-md bg-blue-500 px-3 py-1.5 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 hover:bg-blue-600">
            <Plus className="h-3 w-3" /> 添加域名
          </button>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-slate-50/60 text-xs text-slate-500">
                <th className="px-5 py-2.5 text-left font-medium">域名</th>
                <th className="px-3 py-2.5 text-left font-medium">类型</th>
                <th className="px-3 py-2.5 text-left font-medium">目标地址</th>
                <th className="px-3 py-2.5 text-left font-medium">TTL</th>
                <th className="px-3 py-2.5 text-left font-medium">服务商</th>
                <th className="px-3 py-2.5 text-left font-medium">状态</th>
                <th className="px-3 py-2.5 text-left font-medium">最近同步</th>
                <th className="px-3 py-2.5 text-left font-medium">自动同步</th>
                <th className="px-5 py-2.5 text-right font-medium">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {filtered.length === 0 && (
                <tr>
                  <td colSpan={9} className="px-5 py-8 text-center text-sm text-slate-400">
                    没有匹配的记录
                  </td>
                </tr>
              )}
              {filtered.map((r) => {
                const s = statusInfo[r.status]
                const Icon = s.icon
                return (
                  <tr key={r.id} className="text-slate-700 transition hover:bg-slate-50/60">
                    <td className="px-5 py-3 font-semibold">{r.domain}</td>
                    <td className="px-3 py-3">
                      <span className="rounded-md bg-blue-50 px-1.5 py-0.5 text-[11px] font-bold text-blue-600">
                        {r.recordType}
                      </span>
                    </td>
                    <td className="px-3 py-3 font-mono text-xs">{r.target}</td>
                    <td className="px-3 py-3 text-xs text-slate-500">{r.ttl}s</td>
                    <td className="px-3 py-3 text-xs">{r.provider}</td>
                    <td className="px-3 py-3">
                      <span
                        className={`inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] font-semibold ${s.tone}`}
                      >
                        <Icon className={`h-3 w-3 ${r.status === 'pending' ? 'animate-spin' : ''}`} />
                        {s.label}
                      </span>
                    </td>
                    <td className="px-3 py-3 text-xs text-slate-500">{r.lastUpdate}</td>
                    <td className="px-3 py-3">
                      <span
                        className={`inline-block h-2 w-2 rounded-full ${r.autoSync ? 'bg-emerald-500' : 'bg-slate-300'}`}
                      />
                      <span className="ml-1.5 text-xs text-slate-600">{r.autoSync ? '已开启' : '已关闭'}</span>
                    </td>
                    <td className="px-5 py-3 text-right">
                      <div className="inline-flex items-center gap-1">
                        <button className="grid h-6 w-6 place-items-center rounded-md text-slate-400 hover:bg-blue-50 hover:text-blue-600" title="立即同步">
                          <RefreshCw className="h-3 w-3" />
                        </button>
                        <button className="grid h-6 w-6 place-items-center rounded-md text-slate-400 hover:bg-emerald-50 hover:text-emerald-600" title="编辑">
                          <Pencil className="h-3 w-3" />
                        </button>
                        <button className="grid h-6 w-6 place-items-center rounded-md text-slate-400 hover:bg-rose-50 hover:text-rose-500" title="删除">
                          <Trash2 className="h-3 w-3" />
                        </button>
                      </div>
                    </td>
                  </tr>
                )
              })}
            </tbody>
          </table>
        </div>

        <div className="border-t border-slate-100 px-5 py-3 text-xs text-slate-400">
          共 {filtered.length} 条记录 (mock 数据，后续接入真实 DDNS 模块)
        </div>
      </Card>
    </div>
  )
}
