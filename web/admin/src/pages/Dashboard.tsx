import { useCallback, useEffect, useMemo, useState } from 'react'
import {
  AlertTriangle,
  ClipboardCopy,
  ExternalLink,
  FileBadge,
  Globe,
  Plus,
  Repeat,
  RotateCw,
  ServerCog,
  ShieldCheck,
  Workflow,
} from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import * as api from '../lib/api'
import { portText, siteAddr, siteURL } from './ReverseProxy'
import type {
  Certificate,
  DdnsConfig,
  NatTypeInfo,
  PageKey,
  ProxyConfig,
  StunConfig,
  StunDevice,
  StunService,
  StunStatusEvent,
} from '../types'

// Dashboard 不接 props，App 监听 hashchange，改 hash 就跳页
function go(page: PageKey) {
  window.location.hash = `#/${page}`
}

const phaseLabel: Record<string, string> = {
  PROBING: '探测中',
  RUNNING: '穿透成功',
  RESTARTING: '重启中',
  FAILED: '探测失败',
  STOPPED: '已停止',
}

function getDeviceId(d: StunDevice): number {
  return d.DeviceID ?? d.deviceId ?? d.id
}

/**
 * 和后端 PublicScheme、STUN 页的 openAddress 保持同一套判断：
 * 洞口终结 TLS 时外面那一段就是 https；不终结却勾了「转发给内网时也用 HTTPS」
 * 时洞口在替内网加密、外面是明文，写成 https:// 只会给出一个必报
 * ERR_SSL_PROTOCOL_ERROR 的链接。
 */
function serviceScheme(svc: StunService) {
  return svc.tlsTerminate || (!svc.backendHttps && svc.https) ? 'https' : 'http'
}

function ddnsFullName(subDomain: string, domain: string) {
  const sub = (subDomain || '').trim()
  if (!sub || sub === '@') return domain
  return `${sub}.${domain}`
}

/** 需要用户去处理的一条，按严重程度排前面 */
interface Alert {
  key: string
  level: 'error' | 'warn'
  text: string
  page: PageKey
}

export function Dashboard() {
  const [config, setConfig] = useState<StunConfig | null>(null)
  const [natType, setNatType] = useState<NatTypeInfo | null>(null)
  const [ddns, setDdns] = useState<DdnsConfig | null>(null)
  const [certs, setCerts] = useState<Certificate[]>([])
  const [proxy, setProxy] = useState<ProxyConfig | null>(null)
  const [version, setVersion] = useState('')
  const [statusMap, setStatusMap] = useState<Record<string, StunStatusEvent>>({})
  const [loadErr, setLoadErr] = useState('')
  const [refreshedAt, setRefreshedAt] = useState<Date | null>(null)
  const [copied, setCopied] = useState('')

  // 每块都单独 catch：证书页没配、反代没开都不该让整个仪表盘空白
  const refresh = useCallback(async () => {
    try {
      const cfg = await api.getStunConfig()
      setConfig(cfg)
      setLoadErr('')
    } catch (e) {
      setLoadErr(e instanceof Error ? e.message : String(e))
    }
    api.getNatType().then(setNatType).catch(() => {})
    api.getDdnsConfig().then(setDdns).catch(() => {})
    api.getCertList().then(setCerts).catch(() => {})
    api.getProxyConfig().then(setProxy).catch(() => {})
    api.getVersion().then((v) => setVersion(v.version)).catch(() => {})
    setRefreshedAt(new Date())
  }, [])

  useEffect(() => {
    refresh()
    const t = window.setInterval(refresh, 30000)
    return () => window.clearInterval(t)
  }, [refresh])

  // 外部端口是漂的，只有这条 SSE 是实时的，别的都靠 30s 轮询
  useEffect(() => {
    const es = api.subscribeStunStatus((data) => {
      try {
        const evt = JSON.parse(data) as StunStatusEvent
        setStatusMap((p) => ({ ...p, [evt.key]: evt }))
      } catch {
        // 单条事件坏了不影响后面的
      }
    })
    return () => es.close()
  }, [])

  const copy = (text: string) => {
    navigator.clipboard
      .writeText(text)
      .then(() => {
        setCopied(text)
        window.setTimeout(() => setCopied(''), 1500)
      })
      .catch(() => {})
  }

  // 把设备摊平成一条条对外服务，每条都带上它此刻真实的外部地址
  const rows = useMemo(() => {
    const list = []
    for (const d of config?.devices ?? []) {
      for (const svc of d.services ?? []) {
        const key = `${getDeviceId(d)}-${svc.id}`
        const status = statusMap[key]
        const phase = status?.phaseStr || 'STOPPED'
        const externalPort = status?.externalPort ?? 0
        // 配了对外域名就用域名——终结 TLS 时证书是按域名签的，用 IP 访问必然报证书错
        const externalHost = svc.domain || config?.publicIP || ''
        list.push({
          key,
          svc,
          deviceName: d.name,
          internal: `${d.ip}:${svc.internalPort}`,
          phase,
          running: phase === 'RUNNING',
          lastError: status?.lastError || '',
          external: externalPort > 0 && externalHost ? `${externalHost}:${externalPort}` : '',
        })
      }
    }
    return list
  }, [config, statusMap])

  const enabledRows = rows.filter((r) => r.svc.enabled)
  const runningCount = enabledRows.filter((r) => r.running).length

  const alerts = useMemo<Alert[]>(() => {
    const out: Alert[] = []

    for (const r of rows) {
      if (!r.svc.enabled) continue
      if (!r.running) {
        out.push({
          key: `hole-${r.key}`,
          level: 'error',
          text: `「${r.svc.name || '未命名服务'}」没穿透成功（${phaseLabel[r.phase] || r.phase}）${r.lastError ? '：' + r.lastError : ''}`,
          page: 'stun',
        })
      }
      // 终结 TLS 时证书是按域名签的，域名留空会回落公网 IP，
      // 而证书永远盖不住 IP，点开只会看到 ERR_CERT_COMMON_NAME_INVALID
      if (r.svc.tlsTerminate && !r.svc.domain) {
        out.push({
          key: `nodomain-${r.key}`,
          level: 'warn',
          text: `「${r.svc.name || '未命名服务'}」开了 TLS 终结却没填对外域名，生成的链接会用公网 IP，浏览器会报证书错误`,
          page: 'stun',
        })
      }
    }

    for (const c of certs) {
      if (!c.enabled) continue
      if (c.lastError) {
        out.push({ key: `cert-err-${c.id}`, level: 'error', text: `证书「${c.name}」上次签发失败：${c.lastError}`, page: 'cert' })
      } else if (c.expired) {
        out.push({ key: `cert-exp-${c.id}`, level: 'error', text: `证书「${c.name}」已过期`, page: 'cert' })
      } else if (c.daysLeft > 0 && c.daysLeft <= 15) {
        out.push({ key: `cert-soon-${c.id}`, level: 'warn', text: `证书「${c.name}」还剩 ${c.daysLeft} 天到期`, page: 'cert' })
      }
    }

    for (const r of ddns?.records ?? []) {
      if (r.enabled && r.lastStatus === 'failed') {
        out.push({
          key: `ddns-${r.id}`,
          level: 'error',
          text: `域名 ${ddnsFullName(r.subDomain, r.domain)} 同步失败${r.lastMessage ? '：' + r.lastMessage : ''}`,
          page: 'ddns',
        })
      }
    }

    for (const l of proxy?.listeners ?? []) {
      if (!l.running) {
        out.push({
          key: `proxy-${l.port}`,
          level: 'error',
          text: `反向代理 ${l.port} 端口没监听起来${l.lastError ? '：' + l.lastError : ''}`,
          page: 'reverse-proxy',
        })
      }
    }

    return out.sort((a, b) => (a.level === b.level ? 0 : a.level === 'error' ? -1 : 1))
  }, [rows, certs, ddns, proxy])

  const proxySites = (proxy?.sites ?? []).filter((s) => s.enabled)
  const entry = proxy?.entry

  if (loadErr && !config) {
    return (
      <Card>
        <div className="py-12 text-center text-rose-500">
          <AlertTriangle className="mx-auto h-8 w-8" />
          <div className="mt-2 text-sm font-semibold">数据加载失败</div>
          <div className="mt-1 text-xs text-slate-500">{loadErr}</div>
          <button onClick={refresh} className="mt-4 rounded-xl bg-blue-500 px-4 py-2 text-xs font-semibold text-white">
            重试
          </button>
        </div>
      </Card>
    )
  }

  return (
    <div className="space-y-4">
      {/* 概览 + 快捷操作 */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-12">
        <Card className="lg:col-span-7">
          <CardHeader
            title="当前状态"
            action={
              <button
                onClick={refresh}
                className="flex items-center gap-1 text-xs text-slate-400 transition hover:text-blue-500"
              >
                <RotateCw className="h-3.5 w-3.5" /> 刷新
              </button>
            }
          />
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
            <div className="rounded-xl bg-slate-50 p-3">
              <div className="text-xs text-slate-500">公网 IP</div>
              <div className="mt-1 break-all font-mono text-sm font-bold text-slate-800">
                {config?.publicIP || '--'}
              </div>
            </div>
            <div className="rounded-xl bg-slate-50 p-3">
              <div className="text-xs text-slate-500">NAT 层数</div>
              <div className="mt-1 text-sm font-bold text-slate-800">
                {config ? `${(config.natRouterList || []).length} 层` : '--'}
              </div>
              <div className="mt-0.5 truncate text-[11px] text-slate-400" title={natType?.udp?.natType || ''}>
                {natType?.udp?.natType || '未检测'}
              </div>
            </div>
            <div className="rounded-xl bg-slate-50 p-3">
              <div className="text-xs text-slate-500">穿透中</div>
              <div
                className={`mt-1 text-sm font-bold ${
                  enabledRows.length > 0 && runningCount === enabledRows.length ? 'text-emerald-600' : 'text-slate-800'
                }`}
              >
                {runningCount} / {enabledRows.length}
              </div>
              <div className="mt-0.5 text-[11px] text-slate-400">已启用的服务</div>
            </div>
            <div className="rounded-xl bg-slate-50 p-3">
              <div className="text-xs text-slate-500">最优 STUN</div>
              <div className="mt-1 break-all font-mono text-[11px] font-semibold text-slate-700">
                {config?.bestStun || '--'}
              </div>
            </div>
          </div>
        </Card>

        <Card className="lg:col-span-5">
          <CardHeader title="快捷操作" />
          <div className="grid grid-cols-3 gap-2 sm:grid-cols-5">
            {[
              { icon: ServerCog, label: '设备', tone: 'bg-blue-50 text-blue-600', page: 'stun' as PageKey },
              { icon: Workflow, label: '服务', tone: 'bg-emerald-50 text-emerald-600', page: 'stun' as PageKey },
              { icon: Globe, label: 'DDNS', tone: 'bg-violet-50 text-violet-600', page: 'ddns' as PageKey },
              { icon: Repeat, label: '反向代理', tone: 'bg-cyan-50 text-cyan-600', page: 'reverse-proxy' as PageKey },
              { icon: ShieldCheck, label: '证书', tone: 'bg-rose-50 text-rose-500', page: 'cert' as PageKey },
            ].map((q) => {
              const Icon = q.icon
              return (
                <button
                  key={q.label}
                  type="button"
                  onClick={() => go(q.page)}
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
        </Card>
      </div>

      {/* 需要处理——没问题时整块不出现，不占地方 */}
      {alerts.length > 0 && (
        <Card>
          <CardHeader title={`需要处理（${alerts.length}）`} />
          <ul className="space-y-2">
            {alerts.map((a) => (
              <li key={a.key}>
                <button
                  type="button"
                  onClick={() => go(a.page)}
                  className={`flex w-full items-start gap-2 rounded-xl border px-3 py-2 text-left text-xs transition hover:shadow-sm ${
                    a.level === 'error'
                      ? 'border-rose-200 bg-rose-50 text-rose-700 hover:bg-rose-100'
                      : 'border-amber-200 bg-amber-50 text-amber-700 hover:bg-amber-100'
                  }`}
                >
                  <AlertTriangle className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                  <span className="flex-1">{a.text}</span>
                </button>
              </li>
            ))}
          </ul>
        </Card>
      )}

      {/* 对外服务——这页的主体 */}
      <Card>
        <CardHeader
          title="对外服务"
          action={
            <button
              onClick={() => go('stun')}
              className="flex items-center gap-1 text-xs text-blue-500 transition hover:text-blue-600"
            >
              <Plus className="h-3.5 w-3.5" /> 管理服务
            </button>
          }
        />
        {rows.length === 0 ? (
          <div className="py-10 text-center text-sm text-slate-400">
            还没有服务。到「网络穿透」页添加设备和服务，这里会显示它们此刻的外部地址
          </div>
        ) : (
          <ul className="divide-y divide-slate-100">
            {rows.map((r) => {
              const scheme = serviceScheme(r.svc)
              // UDP 洞浏览器根本连不上，给「打开」只会点出一个必然失败的标签页，地址还是要能复制的
              const openable = r.external !== '' && (r.svc.protocol || 'TCP').toUpperCase() !== 'UDP'
              const url = openable ? `${scheme}://${r.external}` : ''
              return (
                <li key={r.key} className="flex flex-wrap items-center gap-x-3 gap-y-1.5 py-2.5">
                  <span
                    className={`h-2 w-2 shrink-0 rounded-full ${
                      !r.svc.enabled ? 'bg-slate-300' : r.running ? 'bg-emerald-500' : 'bg-rose-400'
                    }`}
                  />
                  <span className="min-w-[7rem] truncate text-sm font-semibold text-slate-800">
                    {r.svc.name || '未命名服务'}
                  </span>
                  <span className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] font-bold text-slate-500">
                    {(r.svc.protocol || 'TCP').toUpperCase()}
                  </span>
                  {r.svc.tlsTerminate && (
                    <span
                      className="rounded bg-violet-50 px-1.5 py-0.5 text-[10px] font-bold text-violet-600"
                      title="LinkStar 在洞口终结 TLS"
                    >
                      TLS 终结
                    </span>
                  )}

                  <span className="font-mono text-xs text-slate-400" title={`${r.deviceName} 上的内网地址`}>
                    {r.internal}
                  </span>
                  <span className="text-slate-300">→</span>

                  {!r.svc.enabled ? (
                    <span className="text-xs text-slate-400">已停用</span>
                  ) : r.external ? (
                    <>
                      <span className="break-all font-mono text-xs font-semibold text-blue-600">{r.external}</span>
                      <span className="ml-auto flex shrink-0 items-center gap-1.5">
                        <button
                          onClick={() => copy(r.external)}
                          className="flex items-center gap-1 rounded-lg border border-slate-200 px-2 py-1 text-[11px] font-semibold text-slate-600 transition hover:bg-slate-50"
                        >
                          <ClipboardCopy className="h-3 w-3" /> {copied === r.external ? '已复制' : '复制'}
                        </button>
                        {openable && (
                          <a
                            href={url}
                            target="_blank"
                            rel="noopener noreferrer"
                            className="flex items-center gap-1 rounded-lg border border-blue-200 bg-blue-50 px-2 py-1 text-[11px] font-semibold text-blue-600 transition hover:bg-blue-100"
                          >
                            <ExternalLink className="h-3 w-3" /> 打开
                          </a>
                        )}
                      </span>
                    </>
                  ) : (
                    <span className="text-xs text-slate-400">
                      {phaseLabel[r.phase] || r.phase}，还没拿到外部端口
                    </span>
                  )}
                </li>
              )
            })}
          </ul>
        )}
      </Card>

      {/* 反向代理站点——没配就不出现 */}
      {proxySites.length > 0 && (
        <Card>
          <CardHeader
            title="反向代理站点"
            action={
              <button
                onClick={() => go('reverse-proxy')}
                className="text-xs text-blue-500 transition hover:text-blue-600"
              >
                管理站点
              </button>
            }
          />
          <ul className="divide-y divide-slate-100">
            {proxySites.map((s) => {
              // 端口和 scheme 的推导跟反向代理页共用一份，不在这里再算第二遍
              const addr = entry ? siteAddr(entry, s) : null
              const url = entry ? siteURL(entry, s) : ''
              const host = s.hosts[0] || ''
              return (
                <li key={s.id} className="flex flex-wrap items-center gap-x-3 gap-y-1.5 py-2.5">
                  <span className="h-2 w-2 shrink-0 rounded-full bg-cyan-500" />
                  <span className="break-all font-mono text-xs font-semibold text-slate-800">
                    {addr?.https ? 'https' : 'http'}://
                    {/* 占了端口又不填域名 = 当端口转发使，这个端口上来的请求全收 */}
                    <span className={host ? '' : 'text-slate-400'}>{host || '任意域名'}</span>
                    {addr && portText(addr.https, addr.port)}
                    {s.pathPrefix}
                  </span>
                  {s.hosts.length > 1 && (
                    <span className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] text-slate-500">
                      另有 {s.hosts.length - 1} 个域名
                    </span>
                  )}
                  <span className="text-slate-300">→</span>
                  <span className="break-all font-mono text-xs text-slate-500">
                    {s.backendHttps ? 'https' : 'http'}://{s.backend}
                  </span>
                  {url && (
                    <a
                      href={url}
                      target="_blank"
                      rel="noopener noreferrer"
                      className="ml-auto flex shrink-0 items-center gap-1 rounded-lg border border-blue-200 bg-blue-50 px-2 py-1 text-[11px] font-semibold text-blue-600 transition hover:bg-blue-100"
                    >
                      <ExternalLink className="h-3 w-3" /> 打开
                    </a>
                  )}
                </li>
              )
            })}
          </ul>
        </Card>
      )}

      {/* DDNS + 证书 */}
      <div className="grid grid-cols-1 gap-4 md:grid-cols-2">
        <Card>
          <CardHeader
            title="DDNS 域名"
            action={
              <button onClick={() => go('ddns')} className="text-xs text-slate-400 transition hover:text-slate-600">
                全部域名
              </button>
            }
          />
          {(ddns?.records ?? []).length === 0 ? (
            <div className="py-6 text-center text-xs text-slate-400">还没有域名</div>
          ) : (
            <ul className="space-y-2">
              {(ddns?.records ?? []).map((r) => (
                <li key={r.id} className="flex items-center justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="grid h-7 w-7 shrink-0 place-items-center rounded-lg bg-violet-50 text-violet-500">
                      <Globe className="h-3.5 w-3.5" />
                    </span>
                    <div className="min-w-0">
                      <div className="truncate text-sm font-semibold text-slate-700">
                        {ddnsFullName(r.subDomain, r.domain)}
                      </div>
                      <div className="truncate font-mono text-xs text-slate-400">
                        {r.recordType} {r.lastIP || '尚未同步'}
                      </div>
                    </div>
                  </div>
                  <span
                    className={`shrink-0 rounded-md px-1.5 py-0.5 text-[10px] font-semibold ${
                      !r.enabled
                        ? 'bg-slate-100 text-slate-500'
                        : r.lastStatus === 'success'
                          ? 'bg-emerald-50 text-emerald-600'
                          : r.lastStatus === 'failed'
                            ? 'bg-rose-50 text-rose-500'
                            : 'bg-amber-50 text-amber-600'
                    }`}
                    title={r.lastMessage || ''}
                  >
                    {!r.enabled
                      ? '已停用'
                      : r.lastStatus === 'success'
                        ? '正常'
                        : r.lastStatus === 'failed'
                          ? '同步失败'
                          : r.lastStatus === 'skipped'
                            ? 'IP 未变'
                            : '待同步'}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card>
          <CardHeader
            title="证书"
            action={
              <button onClick={() => go('cert')} className="text-xs text-slate-400 transition hover:text-slate-600">
                全部证书
              </button>
            }
          />
          {certs.length === 0 ? (
            <div className="py-6 text-center text-xs text-slate-400">还没有证书</div>
          ) : (
            <ul className="space-y-2">
              {certs.map((c) => (
                <li key={c.id} className="flex items-center justify-between gap-2">
                  <div className="flex min-w-0 items-center gap-2">
                    <span className="grid h-7 w-7 shrink-0 place-items-center rounded-lg bg-amber-50 text-amber-500">
                      <FileBadge className="h-3.5 w-3.5" />
                    </span>
                    <div className="min-w-0">
                      <div className="truncate text-sm font-semibold text-slate-700">{c.name}</div>
                      <div className="truncate font-mono text-xs text-slate-400" title={c.domains.join('、')}>
                        {c.domains.length > 0 ? c.domains.join('、') : '无域名（自签）'}
                      </div>
                    </div>
                  </div>
                  <span
                    className={`shrink-0 rounded-md px-1.5 py-0.5 text-[10px] font-semibold ${
                      !c.enabled
                        ? 'bg-slate-100 text-slate-500'
                        : c.expired || c.lastError
                          ? 'bg-rose-50 text-rose-500'
                          : c.daysLeft <= 15
                            ? 'bg-amber-50 text-amber-600'
                            : 'bg-emerald-50 text-emerald-600'
                    }`}
                    title={c.lastError || ''}
                  >
                    {!c.enabled
                      ? '已停用'
                      : c.lastError
                        ? '签发失败'
                        : c.expired
                          ? '已过期'
                          : c.daysLeft > 0
                            ? `剩 ${c.daysLeft} 天`
                            : '未签发'}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      <div className="flex justify-end gap-3 pt-1 text-xs text-slate-400">
        {version && <span>LinkStar {version}</span>}
        {refreshedAt && <span>最后刷新 {refreshedAt.toLocaleTimeString()}</span>}
      </div>
    </div>
  )
}
