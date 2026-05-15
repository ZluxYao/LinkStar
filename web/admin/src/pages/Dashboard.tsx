import {
  ArrowDownRight,
  ArrowUpRight,
  ChevronRight,
  CircleCheck,
  Computer,
  FileBadge,
  FileText,
  Globe,
  Layers,
  Plus,
  Repeat,
  RotateCw,
  ServerCog,
  ShieldCheck,
  Sparkles,
  Wand2,
  Waves,
  Wifi,
  Workflow,
  Zap,
  type LucideIcon,
} from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import { Sparkline } from '../components/Sparkline'
import {
  auditLogs,
  certs,
  ddnsRows,
  devices,
  services,
  stats,
  systemMetrics,
  topology,
  type StatCard,
} from '../mock/dashboard'

const toneCfg: Record<
  StatCard['tone'],
  { iconBg: string; iconColor: string; line: string; fill: string }
> = {
  blue: { iconBg: 'bg-blue-50', iconColor: 'text-blue-500', line: '#3b82f6', fill: '#3b82f6' },
  emerald: { iconBg: 'bg-emerald-50', iconColor: 'text-emerald-500', line: '#10b981', fill: '#10b981' },
  violet: { iconBg: 'bg-violet-50', iconColor: 'text-violet-500', line: '#8b5cf6', fill: '#8b5cf6' },
  amber: { iconBg: 'bg-amber-50', iconColor: 'text-amber-500', line: '#f59e0b', fill: '#f59e0b' },
  cyan: { iconBg: 'bg-cyan-50', iconColor: 'text-cyan-500', line: '#06b6d4', fill: '#06b6d4' },
}

const statIcon: Record<StatCard['iconKey'], LucideIcon> = {
  device: ServerCog,
  service: Workflow,
  ddns: Globe,
  cert: FileBadge,
  rule: Layers,
}

const topoNodeStyle = {
  local: { ring: 'ring-sky-200', bg: 'bg-sky-50', icon: 'text-sky-500', label: 'text-sky-600' },
  router: { ring: 'ring-amber-200', bg: 'bg-amber-50', icon: 'text-amber-500', label: 'text-amber-600' },
  public: { ring: 'ring-emerald-200', bg: 'bg-emerald-50', icon: 'text-emerald-500', label: 'text-emerald-600' },
}

export function Dashboard() {
  return (
    <div className="space-y-4">
      {/* 顶部 stat cards */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-5">
        {stats.map((s) => {
          const tone = toneCfg[s.tone]
          const Icon = statIcon[s.iconKey]
          const delta = s.delta >= 0
          return (
            <Card key={s.key} className="!p-4">
              <div className="flex items-start gap-3">
                <div className={`grid h-10 w-10 shrink-0 place-items-center rounded-xl ${tone.iconBg}`}>
                  <Icon className={`h-5 w-5 ${tone.iconColor}`} />
                </div>
                <div className="min-w-0 flex-1">
                  <div className="text-xs text-slate-500">{s.label}</div>
                  <div className="mt-0.5 text-2xl font-bold leading-tight text-slate-800">{s.value}</div>
                </div>
              </div>
              <div className="mt-2 flex items-center justify-between text-xs">
                <span className="text-slate-500">总数: {s.total}</span>
                <span
                  className={`flex items-center gap-1 font-semibold ${
                    delta ? 'text-emerald-500' : 'text-amber-500'
                  }`}
                >
                  {delta ? <ArrowUpRight className="h-3.5 w-3.5" /> : <ArrowDownRight className="h-3.5 w-3.5" />}
                  {Math.abs(s.delta)} {s.deltaLabel}
                </span>
              </div>
              <Sparkline values={s.trend} stroke={tone.line} fill={tone.fill} className="mt-2" />
            </Card>
          )
        })}
      </div>

      {/* 拓扑 + 快捷操作 */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-12">
        <Card className="lg:col-span-6">
          <CardHeader title="网络拓扑总览" />
          <div className="flex flex-wrap items-center gap-2 rounded-2xl bg-slate-50/80 p-4">
            {topology.map((node, idx) => {
              const style = topoNodeStyle[node.kind]
              const NodeIcon = node.kind === 'local' ? Computer : node.kind === 'public' ? Globe : Wifi
              return (
                <div key={node.id} className="flex items-center gap-2">
                  <div className={`flex min-w-[140px] flex-col items-center gap-1 rounded-xl ${style.bg} px-4 py-3 ring-1 ${style.ring}`}>
                    <div className="flex items-center gap-1.5">
                      <NodeIcon className={`h-4 w-4 ${style.icon}`} />
                      <span className={`text-xs font-semibold ${style.label}`}>{node.label}</span>
                    </div>
                    <span className="font-mono text-xs text-slate-600">{node.addr}</span>
                  </div>
                  {idx < topology.length - 1 && <ChevronRight className="h-4 w-4 text-slate-300" />}
                </div>
              )
            })}
          </div>
          <div className="mt-4 flex flex-wrap items-center gap-4 text-xs">
            <span className="flex items-center gap-1.5 text-slate-600">
              <CircleCheck className="h-3.5 w-3.5 text-emerald-500" /> 内网连接{' '}
              <span className="font-semibold text-slate-700">正常</span>
            </span>
            <span className="flex items-center gap-1.5 text-slate-600">
              <CircleCheck className="h-3.5 w-3.5 text-emerald-500" /> NAT 穿透{' '}
              <span className="font-semibold text-slate-700">成功</span>
            </span>
            <span className="flex items-center gap-1.5 text-slate-600">
              <CircleCheck className="h-3.5 w-3.5 text-emerald-500" /> 公网连通{' '}
              <span className="font-semibold text-slate-700">正常</span>
            </span>
            <span className="ml-auto flex items-center gap-1 text-slate-400">检测时间：1 分钟前</span>
            <button className="flex items-center gap-1 rounded-md text-blue-500 hover:text-blue-600">
              <RotateCw className="h-3.5 w-3.5" /> 重新检测
            </button>
          </div>
        </Card>

        <Card className="lg:col-span-6">
          <CardHeader title="快捷操作" />
          <div className="grid grid-cols-2 gap-2 sm:grid-cols-5">
            {[
              { icon: ServerCog, label: '添加设备', tone: 'bg-blue-50 text-blue-600' },
              { icon: Workflow, label: '添加服务', tone: 'bg-emerald-50 text-emerald-600' },
              { icon: Globe, label: '添加 DDNS 域名', tone: 'bg-violet-50 text-violet-600' },
              { icon: Repeat, label: '添加反向代理', tone: 'bg-cyan-50 text-cyan-600' },
              { icon: ShieldCheck, label: '申请新证书', tone: 'bg-rose-50 text-rose-500' },
            ].map((q) => {
              const Icon = q.icon
              return (
                <button
                  key={q.label}
                  type="button"
                  className="flex flex-col items-center gap-1.5 rounded-2xl border border-slate-200/70 bg-white p-3 text-xs font-semibold text-slate-700 transition hover:-translate-y-0.5 hover:shadow-md"
                >
                  <span className={`grid h-9 w-9 place-items-center rounded-xl ${q.tone}`}>
                    <Icon className="h-4 w-4" />
                  </span>
                  <span>{q.label}</span>
                </button>
              )
            })}
          </div>

          <div className="mt-4 rounded-2xl bg-gradient-to-r from-slate-50 to-blue-50 p-4 ring-1 ring-slate-200/60">
            <div className="flex items-center justify-between gap-2">
              <div className="text-xs font-semibold text-slate-600">
                新手引导：推荐按以下顺序快速完成配置
              </div>
              <button className="flex items-center gap-1.5 rounded-lg bg-blue-500 px-3 py-1.5 text-xs font-semibold text-white shadow-md shadow-blue-500/25">
                <Wand2 className="h-3.5 w-3.5" /> 一键生成推荐配置
              </button>
            </div>
            <ol className="mt-3 grid grid-cols-1 gap-2 sm:grid-cols-4">
              {[
                { n: 1, title: '创建 DDNS 域名', desc: '绑定您的域名' },
                { n: 2, title: '申请 SSL 证书', desc: '为域名申请证书' },
                { n: 3, title: '创建反向代理', desc: '配置访问规则' },
                { n: 4, title: '绑定您的服务', desc: '对外提供服务' },
              ].map((step) => (
                <li
                  key={step.n}
                  className="flex items-center gap-2 rounded-xl bg-white p-2.5 ring-1 ring-slate-200/70"
                >
                  <span className="grid h-7 w-7 shrink-0 place-items-center rounded-full bg-blue-500 text-xs font-bold text-white">
                    {step.n}
                  </span>
                  <span className="min-w-0">
                    <div className="truncate text-xs font-semibold text-slate-700">{step.title}</div>
                    <div className="truncate text-[11px] text-slate-400">{step.desc}</div>
                  </span>
                </li>
              ))}
            </ol>
          </div>
        </Card>
      </div>

      {/* 状态卡片行 */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2 xl:grid-cols-5">
        <Card>
          <CardHeader
            title="设备状态"
            action={<a className="flex items-center gap-0.5 text-xs text-slate-400 hover:text-slate-600">全部设备 <ChevronRight className="h-3 w-3" /></a>}
          />
          <ul className="space-y-2">
            {devices.map((d) => (
              <li key={d.name} className="flex items-center justify-between gap-2">
                <div className="flex min-w-0 items-center gap-2">
                  <span className={`h-2 w-2 rounded-full ${d.status === 'on' ? 'bg-emerald-500' : 'bg-slate-300'}`} />
                  <div className="min-w-0">
                    <div className="truncate text-sm font-semibold text-slate-700">{d.name}</div>
                    <div className="truncate text-xs text-slate-400">{d.ip}</div>
                  </div>
                </div>
                <span
                  className={`rounded-md px-1.5 py-0.5 text-[10px] font-semibold ${
                    d.badge === '本机'
                      ? 'bg-blue-50 text-blue-600'
                      : d.badge === '在线'
                        ? 'bg-emerald-50 text-emerald-600'
                        : 'bg-slate-100 text-slate-500'
                  }`}
                >
                  {d.badge}
                </span>
              </li>
            ))}
          </ul>
          <button className="mt-3 flex w-full items-center justify-center gap-1 rounded-xl border border-dashed border-slate-300 py-2 text-xs text-slate-500 transition hover:border-blue-300 hover:text-blue-500">
            <Plus className="h-3.5 w-3.5" /> 添加设备
          </button>
        </Card>

        <Card>
          <CardHeader
            title="服务状态"
            action={<a className="flex items-center gap-0.5 text-xs text-slate-400 hover:text-slate-600">全部服务 <ChevronRight className="h-3 w-3" /></a>}
          />
          <ul className="space-y-2">
            {services.map((s) => (
              <li key={s.name} className="flex items-center justify-between gap-2">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="grid h-7 w-7 shrink-0 place-items-center rounded-lg bg-slate-100 text-slate-500">
                    <Workflow className="h-3.5 w-3.5" />
                  </span>
                  <div className="min-w-0">
                    <div className="truncate text-sm font-semibold text-slate-700">{s.name}</div>
                    <div className="truncate text-xs text-slate-400">
                      {s.protocol} {s.port}
                    </div>
                  </div>
                </div>
                <span
                  className={`rounded-md px-1.5 py-0.5 text-[10px] font-semibold ${
                    s.status === 'running' ? 'bg-emerald-50 text-emerald-600' : 'bg-rose-50 text-rose-500'
                  }`}
                >
                  {s.status === 'running' ? '运行中' : '异常'}
                </span>
              </li>
            ))}
          </ul>
          <button className="mt-3 flex w-full items-center justify-center gap-1 rounded-xl border border-dashed border-slate-300 py-2 text-xs text-slate-500 transition hover:border-blue-300 hover:text-blue-500">
            <Plus className="h-3.5 w-3.5" /> 添加服务
          </button>
        </Card>

        <Card>
          <CardHeader
            title="DDNS 域名状态"
            action={<a className="flex items-center gap-0.5 text-xs text-slate-400 hover:text-slate-600">全部域名 <ChevronRight className="h-3 w-3" /></a>}
          />
          <ul className="space-y-2">
            {ddnsRows.map((r) => (
              <li key={r.domain} className="flex items-center justify-between gap-2">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="grid h-7 w-7 shrink-0 place-items-center rounded-lg bg-violet-50 text-violet-500">
                    <Globe className="h-3.5 w-3.5" />
                  </span>
                  <div className="min-w-0">
                    <div className="truncate text-sm font-semibold text-slate-700">{r.domain}</div>
                    <div className="truncate text-xs text-slate-400">
                      {r.recordType} 记录 {r.target}
                    </div>
                  </div>
                </div>
                <span
                  className={`rounded-md px-1.5 py-0.5 text-[10px] font-semibold ${
                    r.status === 'normal'
                      ? 'bg-emerald-50 text-emerald-600'
                      : r.status === 'pending'
                        ? 'bg-amber-50 text-amber-600'
                        : 'bg-rose-50 text-rose-500'
                  }`}
                >
                  {r.status === 'normal' ? '正常' : r.status === 'pending' ? '更新中' : '更新失败'}
                </span>
              </li>
            ))}
          </ul>
          <button className="mt-3 flex w-full items-center justify-center gap-1 rounded-xl border border-dashed border-slate-300 py-2 text-xs text-slate-500 transition hover:border-blue-300 hover:text-blue-500">
            <Plus className="h-3.5 w-3.5" /> 添加域名
          </button>
        </Card>

        <Card>
          <CardHeader
            title="证书到期提醒"
            action={<a className="flex items-center gap-0.5 text-xs text-slate-400 hover:text-slate-600">全部证书 <ChevronRight className="h-3 w-3" /></a>}
          />
          <ul className="space-y-2">
            {certs.map((c) => (
              <li key={c.domain} className="flex items-center justify-between gap-2">
                <div className="flex min-w-0 items-center gap-2">
                  <span className="grid h-7 w-7 shrink-0 place-items-center rounded-lg bg-amber-50 text-amber-500">
                    <ShieldCheck className="h-3.5 w-3.5" />
                  </span>
                  <div className="min-w-0">
                    <div className="truncate text-sm font-semibold text-slate-700">{c.domain}</div>
                    <div className="truncate text-xs text-slate-400">
                      证书有效期剩余 {c.daysLeft} 天
                    </div>
                    <div className="truncate text-[10px] text-slate-400">{c.expireOn}</div>
                  </div>
                </div>
                <span
                  className={`shrink-0 rounded-md px-1.5 py-0.5 text-[10px] font-semibold ${
                    c.status === 'expiring' ? 'bg-amber-50 text-amber-600' : 'bg-emerald-50 text-emerald-600'
                  }`}
                >
                  {c.status === 'expiring' ? '即将到期' : '正常'}
                </span>
              </li>
            ))}
          </ul>
          <button className="mt-3 flex w-full items-center justify-center gap-1 rounded-xl border border-dashed border-slate-300 py-2 text-xs text-slate-500 transition hover:border-blue-300 hover:text-blue-500">
            <Plus className="h-3.5 w-3.5" /> 申请新证书
          </button>
        </Card>

        <Card>
          <CardHeader
            title="系统日志"
            action={<a className="flex items-center gap-0.5 text-xs text-slate-400 hover:text-slate-600">查看更多 <ChevronRight className="h-3 w-3" /></a>}
          />
          <ul className="space-y-2">
            {auditLogs.map((l, i) => (
              <li key={i} className="flex items-start gap-2">
                <span className="mt-1.5 h-1.5 w-1.5 shrink-0 rounded-full bg-blue-400" />
                <div className="min-w-0 flex-1">
                  <div className="text-xs text-slate-400">{l.ago}</div>
                  <div className="truncate text-sm text-slate-700">{l.message}</div>
                </div>
                <span className="shrink-0 rounded-md bg-slate-100 px-1.5 py-0.5 text-[10px] font-semibold text-slate-500">
                  {l.tag}
                </span>
              </li>
            ))}
          </ul>
          <button className="mt-3 flex w-full items-center justify-center gap-1 rounded-xl border border-dashed border-slate-300 py-2 text-xs text-slate-500 transition hover:border-blue-300 hover:text-blue-500">
            <FileText className="h-3.5 w-3.5" /> 查看全部日志
          </button>
        </Card>
      </div>

      {/* STUN + 系统资源 */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-12">
        <Card className="lg:col-span-5">
          <CardHeader
            title="网络穿透能力（STUN 与连接检测）"
            action={
              <a className="flex items-center gap-0.5 text-xs text-blue-500 hover:text-blue-600">
                进入网络穿透能力中心 <ChevronRight className="h-3 w-3" />
              </a>
            }
          />
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
            <div className="rounded-xl bg-slate-50 p-3">
              <div className="text-xs font-semibold text-slate-500">STUN 节点</div>
              <div className="mt-2 text-xs text-slate-500">最优节点</div>
              <div className="mt-0.5 text-sm font-semibold text-slate-700">stun.hot-chilli.net:3478</div>
              <div className="mt-2 flex items-center justify-between text-xs">
                <span className="text-slate-500">延迟</span>
                <span className="rounded-md bg-emerald-50 px-1.5 py-0.5 text-[10px] font-semibold text-emerald-600">
                  可用
                </span>
              </div>
              <div className="mt-0.5 text-sm font-semibold text-slate-700">38ms</div>
            </div>
            <div className="rounded-xl bg-slate-50 p-3">
              <div className="text-xs font-semibold text-slate-500">穿透检测结果</div>
              <div className="mt-2 space-y-1.5 text-xs">
                <div className="flex items-center justify-between">
                  <span className="flex items-center gap-1 text-slate-600">
                    <Waves className="h-3 w-3 text-blue-500" /> UDP 打洞
                  </span>
                  <span className="rounded-md bg-emerald-50 px-1.5 py-0.5 text-[10px] font-semibold text-emerald-600">
                    成功
                  </span>
                </div>
                <div className="flex items-center justify-between">
                  <span className="flex items-center gap-1 text-slate-600">
                    <Sparkles className="h-3 w-3 text-blue-500" /> NAT 类型
                  </span>
                  <span className="text-xs font-semibold text-slate-700">锥形 NAT</span>
                </div>
              </div>
            </div>
            <div className="rounded-xl bg-slate-50 p-3">
              <div className="text-xs font-semibold text-slate-500">连接质量</div>
              <div className="mt-2 space-y-1.5 text-xs">
                {[
                  { label: '成功率', value: '98%', pct: 98, tone: 'bg-emerald-500' },
                  { label: '平均延迟', value: '36ms', pct: 36, tone: 'bg-amber-500' },
                  { label: '丢包率', value: '0.2%', pct: 2, tone: 'bg-emerald-500' },
                ].map((m) => (
                  <div key={m.label}>
                    <div className="flex justify-between text-slate-500">
                      <span>{m.label}</span>
                      <span className="font-semibold text-slate-700">{m.value}</span>
                    </div>
                    <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-slate-200">
                      <div className={`h-full ${m.tone}`} style={{ width: `${m.pct}%` }} />
                    </div>
                  </div>
                ))}
              </div>
            </div>
          </div>
        </Card>

        <Card className="lg:col-span-7">
          <CardHeader
            title="系统资源"
            action={
              <a className="flex items-center gap-0.5 text-xs text-slate-400 hover:text-slate-600">
                查看更多系统信息 <ChevronRight className="h-3 w-3" />
              </a>
            }
          />
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-5">
            {systemMetrics.map((m) => {
              const tone = toneCfg[m.tone]
              return (
                <div key={m.label} className="rounded-xl bg-slate-50 p-3">
                  <div className="text-xs font-semibold text-slate-500">{m.label}</div>
                  <div className="mt-2 flex items-baseline justify-between">
                    <span className="text-lg font-bold text-slate-700">{m.display}</span>
                    <span className="text-xs text-slate-400">{m.value}%</span>
                  </div>
                  <div className="mt-1 h-1.5 overflow-hidden rounded-full bg-slate-200">
                    <div
                      className="h-full rounded-full"
                      style={{ width: `${m.value}%`, background: tone.line }}
                    />
                  </div>
                  <Sparkline values={m.trend} stroke={tone.line} className="mt-2" />
                </div>
              )
            })}
          </div>
        </Card>
      </div>

      <div className="flex justify-end pt-1 text-xs text-slate-400">
        <span className="flex items-center gap-1">
          最后更新：1 分钟前 <Zap className="h-3 w-3 text-amber-500" />
        </span>
      </div>
    </div>
  )
}
