import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertCircle,
  BookmarkPlus,
  ChevronRight,
  ClipboardCopy,
  Computer,
  Copy,
  CornerUpRight,
  ExternalLink,
  FileText,
  Globe,
  Home as HomeIcon,
  Info,
  LayoutTemplate,
  LoaderCircle,
  Pencil,
  Plus,
  Power,
  Radar,
  RotateCw,
  Server,
  Send,
  ShieldCheck,
  Trash2,
  TriangleAlert,
  Wifi,
  X,
} from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import { modalBackdrop } from '../components/modal'
import * as api from '../lib/api'
import type {
  Certificate,
  DdnsProvider,
  LanHost,
  LanPort,
  LanSubnet,
  NatDetectResult,
  NatTypeInfo,
  StunConfig,
  StunDevice,
  RedirectConfig,
  RedirectInspection,
  StunService,
  StunStatusEvent,
  WebhookConfig,
  WebhookTemplate,
} from '../types'

const phaseLabel: Record<string, string> = {
  PROBING: '探测中',
  RUNNING: '穿透成功',
  RESTARTING: '重启中',
  FAILED: '探测失败',
  STOPPED: '已停止',
}

/** 后端给的是 time.Time，没同步过时是零值，别显示成 0001 年 */
function fmtTime(v?: string) {
  if (!v) return ''
  const d = new Date(v)
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return ''
  return d.toLocaleString()
}

interface Toast {
  id: number
  text: string
}

function useToast() {
  const [toasts, setToasts] = useState<Toast[]>([])
  const idRef = useRef(0)
  const show = useCallback((text: string) => {
    const id = ++idRef.current
    setToasts((p) => [...p, { id, text }])
    // 「已保存」两秒够了，但同步失败那种带着「接下来该干什么」的长句，
    // 两秒读不完就没了，等于没说。按字数给时间，最多十二秒
    const ms = Math.min(12000, Math.max(2000, text.length * 130))
    window.setTimeout(() => setToasts((p) => p.filter((t) => t.id !== id)), ms)
  }, [])
  return { toasts, show }
}

function getDeviceId(d: StunDevice): number {
  return d.DeviceID ?? d.deviceId ?? d.id
}

interface DeviceFormState {
  name: string
  ip: string
}
const emptyDevice: DeviceFormState = { name: '', ip: '' }

function DeviceModal({
  initial,
  prefill,
  onCancel,
  onSubmit,
}: {
  initial?: StunDevice
  /** 扫描结果点「添加」时带过来的 IP。只在新增时用，改的时候以 initial 为准 */
  prefill?: DeviceFormState
  onCancel: () => void
  onSubmit: (form: DeviceFormState) => Promise<void>
}) {
  const [form, setForm] = useState<DeviceFormState>(() =>
    initial ? { name: initial.name, ip: initial.ip } : (prefill ?? emptyDevice),
  )
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const submit = async () => {
    if (!form.name.trim() || !form.ip.trim()) {
      setErr('设备名称和 IP 不能为空')
      return
    }
    setErr('')
    setBusy(true)
    try {
      await onSubmit(form)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '操作失败')
      setBusy(false)
    }
  }

  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="max-h-full w-full max-w-md overflow-y-auto rounded-2xl bg-white p-6 text-slate-700 shadow-2xl ring-1 ring-slate-200">
        <div className="mb-5 flex items-center justify-between">
          <div className="text-base font-bold text-slate-800">{initial ? '编辑设备' : '添加设备'}</div>
          <button
            type="button"
            onClick={onCancel}
            className="grid h-7 w-7 place-items-center rounded-full text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="space-y-3">
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-slate-500">设备名称</div>
            <input
              autoFocus
              value={form.name}
              onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
              placeholder="如 群晖NAS / 树莓派"
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-slate-500">设备内网 IP</div>
            <input
              value={form.ip}
              onChange={(e) => setForm((p) => ({ ...p, ip: e.target.value }))}
              placeholder="如 192.168.1.100"
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          {err && <div className="text-xs text-rose-500">{err}</div>}
        </div>
        <div className="mt-5 flex justify-end gap-2">
          <button
            type="button"
            onClick={onCancel}
            className="rounded-xl bg-slate-100 px-4 py-2 text-sm font-semibold text-slate-600 transition hover:bg-slate-200"
          >
            取消
          </button>
          <button
            type="button"
            onClick={submit}
            disabled={busy}
            className="rounded-xl bg-blue-500 px-4 py-2 text-sm font-semibold text-white shadow-md shadow-blue-500/20 transition hover:bg-blue-600 disabled:opacity-50"
          >
            {busy ? '保存中...' : '保存'}
          </button>
        </div>
      </div>
    </div>
  )
}

/**
 * 局域网扫描。
 *
 * 「添加设备」原本要用户自己知道那台机器的内网 IP——得去路由器后台翻 DHCP 列表，
 * 或者跑去那台机器上敲命令。卡在这一步的人，后面打洞根本轮不到。
 * 这里直接把这段网里的机器摆出来选。
 *
 * 网段列表由外面先取好再传进来，弹窗一开下拉框就是满的。
 */
function LanScanModal({
  subnets,
  devices,
  onCancel,
  onPick,
  onPickPort,
}: {
  subnets: LanSubnet[]
  devices: StunDevice[]
  onCancel: () => void
  onPick: (host: LanHost) => void
  onPickPort: (device: StunDevice, port: LanPort) => void
}) {
  const [cidr, setCidr] = useState(subnets[0]?.cidr ?? '')
  const [hosts, setHosts] = useState<LanHost[] | null>(null)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  // 扫完之后用户可能直接在底下把设备加了，加完这一行要变成「已加入」，
  // 所以别信扫描结果里的 added，按当前设备列表现算
  const byIP = useMemo(() => {
    const m = new Map<string, StunDevice>()
    for (const d of devices) if (d.ip) m.set(d.ip, d)
    return m
  }, [devices])

  // 这个 IP 的这个端口上已经建过服务了。不标的话同一个端口很容易建出两份，
  // 两份各自去打一个洞，对外两个地址指向同一个服务
  const takenPorts = useMemo(() => {
    const s = new Set<string>()
    for (const d of devices) {
      for (const svc of d.services ?? []) s.add(`${d.ip}:${svc.internalPort}`)
    }
    return s
  }, [devices])

  const run = async () => {
    setBusy(true)
    setErr('')
    // 上一轮的结果先清掉。换个网段再扫的时候，旧的那几台还挂在那儿，
    // 看着就跟这一轮扫出来的一样
    setHosts(null)
    try {
      setHosts(await api.scanLan(cidr))
    } catch (e) {
      setHosts(null)
      setErr(e instanceof Error ? e.message : '扫描失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="max-h-full w-full max-w-lg overflow-y-auto rounded-2xl bg-white p-6 text-slate-700 shadow-2xl ring-1 ring-slate-200">
        <div className="mb-4 flex items-center justify-between">
          <div className="text-base font-bold text-slate-800">扫描局域网</div>
          <button
            type="button"
            onClick={onCancel}
            className="grid h-7 w-7 place-items-center rounded-full text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        {subnets.length === 0 ? (
          <div className="rounded-xl bg-amber-50 px-3 py-2.5 text-xs leading-5 text-amber-700">
            没找到本机接着的内网网段。手动填 IP 添加设备吧。
          </div>
        ) : (
          <>
            <div className="flex items-end gap-2">
              <label className="min-w-0 flex-1">
                <div className="mb-1 text-xs font-semibold text-slate-500">扫哪一段</div>
                <select
                  value={cidr}
                  onChange={(e) => setCidr(e.target.value)}
                  className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
                >
                  {subnets.map((s) => (
                    <option key={s.cidr} value={s.cidr}>
                      {s.cidr} · {s.iface}（本机 {s.localIP}）
                    </option>
                  ))}
                </select>
              </label>
              <button
                type="button"
                onClick={run}
                disabled={busy}
                className="flex shrink-0 items-center gap-1.5 rounded-xl bg-blue-500 px-4 py-2 text-sm font-semibold text-white shadow-md shadow-blue-500/20 transition hover:bg-blue-600 disabled:opacity-50"
              >
                {busy ? (
                  <LoaderCircle className="h-4 w-4 animate-spin" />
                ) : (
                  <Radar className="h-4 w-4" />
                )}
                {busy ? '扫描中' : '开始扫描'}
              </button>
            </div>

            {/*
              扫不到的是防火墙设成「什么都不回」的机器，Windows 默认就这样。
              不说这句的话，用户会以为那台设备根本不在网里，转头去查网线。
            */}
            <div className="mt-2 text-[11px] leading-5 text-slate-400">
              两三秒出结果。防火墙拦得严的机器（Windows 默认就是）扫不出来，这种只能自己填 IP。
            </div>

            {err && <div className="mt-3 text-xs text-rose-500">扫不了：{err}</div>}

            {hosts && (
              <div className="mt-4">
                <div className="mb-2 flex items-baseline gap-2">
                  <span className="text-xs font-semibold text-slate-500">找到 {hosts.length} 台</span>
                  {/* 端口能点这件事看不出来，得说一句 */}
                  <span className="text-[11px] text-slate-400">已加过的设备，点端口直接建服务</span>
                </div>
                {hosts.length === 0 ? (
                  <div className="rounded-xl bg-slate-50 px-3 py-4 text-center text-xs text-slate-400">
                    一台都没扫到。换个网段试试，或者手动填 IP
                  </div>
                ) : (
                  <ul className="space-y-1.5">
                    {hosts.map((h) => {
                      const dev = byIP.get(h.ip)
                      return (
                        <li
                          key={h.ip}
                          className="rounded-xl border border-slate-100 bg-slate-50/60 px-3 py-2"
                        >
                          <div className="flex items-center gap-2">
                            <Server className="h-4 w-4 shrink-0 text-slate-400" />
                            <span className="truncate font-mono text-sm font-semibold text-slate-700">
                              {h.ip}
                            </span>
                            {h.name && (
                              <span className="truncate text-xs text-slate-400">{h.name}</span>
                            )}
                            {h.self && (
                              <span className="shrink-0 rounded bg-slate-200 px-1.5 py-0.5 text-[10px] font-semibold text-slate-600">
                                本机
                              </span>
                            )}
                            <div className="flex-1" />
                            {dev ? (
                              <span className="shrink-0 truncate text-[11px] text-emerald-600">
                                已加过「{dev.name || dev.ip}」
                              </span>
                            ) : (
                              <button
                                type="button"
                                onClick={() => onPick(h)}
                                className="shrink-0 rounded-md bg-white px-2 py-0.5 text-[11px] font-semibold text-blue-600 ring-1 ring-blue-200 transition hover:bg-blue-50"
                              >
                                添加
                              </button>
                            )}
                          </div>
                          {h.ports.length > 0 && (
                            <div className="mt-1.5 flex flex-wrap gap-1">
                              {h.ports.map((p) => {
                                const taken = takenPorts.has(`${h.ip}:${p.port}`)
                                // 端口能点着建服务的前提是这台先成了设备——
                                // 服务得挂在某台设备底下，不然没地方放
                                if (!dev || taken) {
                                  return (
                                    <span
                                      key={p.port}
                                      title={taken ? '这个端口已经建过服务了' : '先把这台加成设备'}
                                      className={`rounded px-1.5 py-0.5 text-[10px] ring-1 ${
                                        taken
                                          ? 'bg-emerald-50 text-emerald-600 ring-emerald-200'
                                          : 'bg-white text-slate-500 ring-slate-200'
                                      }`}
                                    >
                                      {taken && '✓ '}
                                      {p.port}
                                      {p.name && <span className="opacity-70"> {p.name}</span>}
                                    </span>
                                  )
                                }
                                return (
                                  <button
                                    key={p.port}
                                    type="button"
                                    onClick={() => onPickPort(dev, p)}
                                    title={`建一个服务，对外开放 ${p.port} 端口`}
                                    className="rounded bg-white px-1.5 py-0.5 text-[10px] text-slate-600 ring-1 ring-slate-200 transition hover:bg-blue-50 hover:text-blue-600 hover:ring-blue-300"
                                  >
                                    {p.port}
                                    {p.name && <span className="opacity-70"> {p.name}</span>}
                                  </button>
                                )
                              })}
                            </div>
                          )}
                        </li>
                      )
                    })}
                  </ul>
                )}
              </div>
            )}
          </>
        )}

        <div className="mt-5 flex justify-end">
          <button
            type="button"
            onClick={onCancel}
            className="rounded-xl bg-slate-100 px-4 py-2 text-sm font-semibold text-slate-600 transition hover:bg-slate-200"
          >
            关闭
          </button>
        </div>
      </div>
    </div>
  )
}

interface ServiceFormState {
  name: string
  internalPort: string
  protocol: 'TCP' | 'UDP'
  upnpMappedPort: string
  useUpnp: boolean
  /** 只管链接展示成 http:// 还是 https://，不影响转发 */
  https: boolean
  domain: string
  tlsTerminate: boolean
  /** 证书 ID，'0' 表示按域名自动匹配 */
  certId: string
  /** 转发给内网时也用 tls.Dial，等价 nginx 的 proxy_pass https://；仅在 tlsTerminate 时成立 */
  backendHttps: boolean
  enabled: boolean
  showOnHome: boolean
  description: string
  webhookconfig: WebhookConfig
  redirect: RedirectConfig
}

type ServiceModalTab = 'basic' | 'webhook' | 'redirect'

const emptyRedirect: RedirectConfig = {
  enabled: false,
  providerId: 0,
  entryHost: '',
  zoneDomain: '',
}

const emptyWebhook: WebhookConfig = {
  enabled: false,
  onlyWhenChanged: true,
  url: '',
  method: 'POST',
  headers: '',
  body: '{"service":"#{service_name}","address":"#{address}","phase":"#{phase}"}',
  disableSuccessCheck: true,
  successContains: '',
  proxy: '',
}

const webhookMethods = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS']

const emptyService: ServiceFormState = {
  name: '',
  internalPort: '',
  protocol: 'TCP',
  upnpMappedPort: '0',
  useUpnp: true,
  https: false,
  domain: '',
  tlsTerminate: false,
  certId: '0',
  backendHttps: false,
  enabled: true,
  showOnHome: false,
  description: '',
  webhookconfig: emptyWebhook,
  redirect: emptyRedirect,
}

/** *.example.com 这种通配证书挑不出具体主机名——用哪个标签只能由用户决定 */
function isWildcardDomain(d: string) {
  return d.startsWith('*.')
}

/** 和后端 utils/domain.Match 同一套语义：通配只覆盖恰好多一层标签的名字 */
function matchCertDomain(pattern: string, name: string) {
  if (!name) return false
  if (!isWildcardDomain(pattern)) return pattern === name
  const suffix = pattern.slice(1) // ".example.com"
  if (!name.endsWith(suffix)) return false
  const label = name.slice(0, name.length - suffix.length)
  return label !== '' && !label.includes('.')
}

/** 选中的证书覆盖哪些域名。certId 为 0（按 SNI 自动匹配）时是所有启用证书的并集 */
function certDomainsOf(certs: Certificate[], certId: string): string[] {
  const id = Number(certId) || 0
  if (id !== 0) return certs.find((c) => c.id === id)?.domains ?? []
  return certs.filter((c) => c.enabled).flatMap((c) => c.domains)
}

/**
 * pickCertDomain 从证书上挑一个能直接当「对外域名」用的具体域名。
 *
 * 终结 TLS 时证书是按域名签的，而对外域名留空会回落公网 IP——IP 不发 SNI
 * （RFC 6066），证书也不可能覆盖 IP，首页链接和 /go 跳转就会带用户去一个
 * 必然报 ERR_CERT_COMMON_NAME_INVALID 的地址。证书上既然写着域名，
 * 就直接填上，不该让用户自己去证书页抄一遍。
 *
 * 挑不出来时返回空串（通配证书、或自签这种压根没有域名的证书）。
 */
function pickCertDomain(certs: Certificate[], certId: string): string {
  const concrete = (c?: Certificate) => c?.domains?.find((d) => !isWildcardDomain(d)) || ''

  const id = Number(certId) || 0
  if (id !== 0) return concrete(certs.find((c) => c.id === id))

  // 按 SNI 自动匹配：兜底证书是 SNI 落空时真正会用上的那张，优先看它；
  // 它没有具体域名（自签）时，只有唯一一张带域名的证书才好替用户做主
  const withDomain = certs.filter((c) => c.enabled && c.domains.some((d) => !isWildcardDomain(d)))
  const fallback = concrete(certs.find((c) => c.enabled && c.isDefault))
  if (fallback) return fallback
  return withDomain.length === 1 ? concrete(withDomain[0]) : ''
}

function normalizeWebhook(cfg?: Partial<WebhookConfig>): WebhookConfig {
  return {
    ...emptyWebhook,
    ...(cfg || {}),
    method: cfg?.method || emptyWebhook.method,
  }
}

/** 已存在的服务 → 表单。编辑和「复制」都从这儿取初值，省得两处各写一遍 */
function serviceToForm(svc: StunService, showOnHome: boolean): ServiceFormState {
  return {
    name: svc.name,
    internalPort: String(svc.internalPort || ''),
    protocol: (svc.protocol as 'TCP' | 'UDP') || 'TCP',
    upnpMappedPort: String(svc.upnpMappedPort || 0),
    useUpnp: !!svc.useUpnp,
    https: !!svc.https,
    domain: svc.domain || '',
    tlsTerminate: !!svc.tlsTerminate,
    certId: String(svc.certId || 0),
    backendHttps: !!svc.backendHttps,
    enabled: svc.enabled !== false,
    showOnHome,
    description: svc.description || '',
    webhookconfig: normalizeWebhook(svc.webhookconfig),
    redirect: { ...emptyRedirect, ...(svc.redirect ?? {}) },
  }
}

/**
 * 已存在的服务 → 更新报文，原样一份。
 *
 * 列表上那个启停开关也得把整个服务发回去：后端的更新接口是整体覆盖，
 * 少带哪个字段，那个字段就被清成空的了。
 */
function serviceToPayload(deviceId: number, svc: StunService): api.StunServicePayload {
  return {
    deviceId,
    name: svc.name,
    internalPort: svc.internalPort,
    protocol: svc.protocol || 'TCP',
    upnpMappedPort: svc.upnpMappedPort || 0,
    useUpnp: !!svc.useUpnp,
    https: !!svc.https,
    domain: svc.domain || '',
    tlsTerminate: !!svc.tlsTerminate,
    certId: svc.certId || 0,
    backendHttps: !!svc.backendHttps,
    enabled: svc.enabled !== false,
    description: svc.description || '',
    webhookconfig: normalizeWebhook(svc.webhookconfig),
    redirect: { ...emptyRedirect, ...(svc.redirect ?? {}) },
  }
}

function getWebhookStatus(status?: StunStatusEvent) {
  const logs = status?.logs ?? []
  for (let i = logs.length - 1; i >= 0; i--) {
    const msg = logs[i]?.message || ''
    if (msg.includes('Webhook 发送成功')) {
      return { state: 'success' as const, text: '最近发送成功', at: logs[i].createdAt }
    }
    if (msg.includes('Webhook 发送失败')) {
      return { state: 'failed' as const, text: '最近发送失败', at: logs[i].createdAt }
    }
  }
  return { state: 'pending' as const, text: '等待后台发送', at: '' }
}

function ServiceModal({
  deviceId,
  initial,
  prefill,
  initialShowOnHome,
  status,
  onCancel,
  onSubmit,
  onToast,
}: {
  deviceId: number
  initial?: StunService
  /** 新建时的初值：扫描里点端口、或者复制现有服务带过来的。改的时候以 initial 为准 */
  prefill?: Partial<ServiceFormState>
  initialShowOnHome: boolean
  status?: StunStatusEvent
  onCancel: () => void
  onSubmit: (form: ServiceFormState) => Promise<void>
  onToast: (text: string) => void
}) {
  const [form, setForm] = useState<ServiceFormState>(() =>
    initial ? serviceToForm(initial, initialShowOnHome) : { ...emptyService, ...prefill },
  )
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')
  const [activeTab, setActiveTab] = useState<ServiceModalTab>('basic')
  const [selectedTemplateId, setSelectedTemplateId] = useState('')
  const [templates, setTemplates] = useState<WebhookTemplate[]>([])
  const [templatesLoading, setTemplatesLoading] = useState(false)
  const [templateBusy, setTemplateBusy] = useState(false)
  const [templateName, setTemplateName] = useState('')
  const [templateDescription, setTemplateDescription] = useState('')
  const [certs, setCerts] = useState<Certificate[]>([])
  const [cfProviders, setCfProviders] = useState<DdnsProvider[]>([])
  const [redirectBusy, setRedirectBusy] = useState(false)
  const [inspect, setInspect] = useState<RedirectInspection | null>(null)
  const [inspecting, setInspecting] = useState(false)
  const [inspectErr, setInspectErr] = useState('')

  // 证书列表拿不到不该拦住整个服务表单，失败就当没有证书
  useEffect(() => {
    let alive = true
    api
      .getCertList()
      .then((list) => alive && setCerts(list))
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [])

  // 重定向规则目前只有 Cloudflare 能写，别的服务商列出来只会让人白填
  useEffect(() => {
    let alive = true
    api
      .getDdnsConfig()
      .then((cfg) => {
        if (!alive) return
        setCfProviders((cfg.providers ?? []).filter((p) => p.type === 'cloudflare'))
      })
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [])

  const updateRedirect = (patch: Partial<RedirectConfig>) =>
    setForm((p) => ({ ...p, redirect: { ...p.redirect, ...patch } }))

  /** 入口域名后两段，和后端 redirectZone 留空时的算法一致 */
  const redirectGuessedZone = useMemo(() => {
    const parts = form.redirect.entryHost.trim().toLowerCase().split('.').filter(Boolean)
    return parts.length >= 2 ? parts.slice(-2).join('.') : ''
  }, [form.redirect.entryHost])

  // 只是给人看这条规则长什么样；真正写过去的地址由后端按当时的端口算
  const redirectPreviewTarget = useMemo(() => {
    const port = status?.externalPort ?? 0
    if (!port) return status?.redirectTarget ?? ''
    // 与 stun.PublicScheme 同一套判断
    const scheme = form.tlsTerminate || (!form.backendHttps && form.https) ? 'https' : 'http'
    return `${scheme}://${form.domain.trim() || '公网IP'}:${port}`
  }, [status?.externalPort, status?.redirectTarget, form.tlsTerminate, form.backendHttps, form.https, form.domain])

  // 下面两个按钮打的是后端「已保存」的配置，不是眼前这张表单。
  // 刚勾上启用还没保存就点同步，后端读到的 enabled 还是 false，
  // 只会回一句「这个服务没有开启入口重定向」——界面上明明写着已启用，看着像见鬼。
  // 干脆表单一改就按不动，旁边直接说清楚该点哪个按钮。
  const redirectDirty = useMemo(() => {
    const saved = { ...emptyRedirect, ...(initial?.redirect ?? {}) }
    const norm = (v?: string) => (v ?? '').trim().toLowerCase()
    return (
      !!saved.enabled !== form.redirect.enabled ||
      (saved.providerId || 0) !== form.redirect.providerId ||
      norm(saved.entryHost) !== norm(form.redirect.entryHost) ||
      norm(saved.zoneDomain) !== norm(form.redirect.zoneDomain)
    )
  }, [initial, form.redirect])

  // 这两条记录归 Cloudflare 那边管，本地配置里看不出来，只能现场去查。
  // 查一次可能几百毫秒，所以不跟着服务列表一起刷，只在打开这个页签时拉一次。
  const savedRedirectOn = !!initial?.redirect?.enabled
  const loadInspect = useCallback(async () => {
    if (!initial || !savedRedirectOn) return
    setInspecting(true)
    setInspectErr('')
    try {
      setInspect(await api.inspectStunRedirect(deviceId, initial.id))
    } catch (e) {
      setInspect(null)
      setInspectErr(e instanceof Error ? e.message : '查不到')
    } finally {
      setInspecting(false)
    }
  }, [deviceId, initial, savedRedirectOn])

  // 切到「CF 重定向」那页才去查那两条记录：查一次要打 Cloudflare 的接口，
  // 打开弹窗就顺手跑一趟的话，只想改个端口的人也得白等
  const openTab = (key: ServiceModalTab) => {
    setActiveTab(key)
    if (key === 'redirect' && !inspect && !inspecting) void loadInspect()
  }

  // 同步/删除打的是已保存的配置，所以新建服务时按不动
  const runRedirect = async (action: 'sync' | 'remove') => {
    if (!initial) return
    if (action === 'remove' && !window.confirm('确认删掉 Cloudflare 上那条规则？')) return
    setRedirectBusy(true)
    try {
      if (action === 'sync') {
        // 直接用后端那句话：这一步可能只做成一半（规则写好了但解析记录没建上），
        // 在这儿按 target 自己拼一句「已同步」，就会把那半截失败盖掉
        const r = await api.syncStunRedirect(deviceId, initial.id)
        onToast(r.msg || `已同步：${r.data?.target ?? ''}`)
      } else {
        await api.removeStunRedirect(deviceId, initial.id)
        onToast('已删除 Cloudflare 上的规则')
      }
      // 同步那一步会顺手建入口记录，下面那块得跟着变，不然还摆着同步前的样子
      void loadInspect()
    } catch (e) {
      onToast((action === 'sync' ? '同步失败: ' : '删除失败: ') + (e instanceof Error ? e.message : ''))
    } finally {
      setRedirectBusy(false)
    }
  }

  // 老服务可能是「已经终结 TLS、对外域名却空着」的状态（那时候还没这条约束）。
  // 证书列表是异步来的，等它到位再补，且只补空的——用户自己填过的不动。
  useEffect(() => {
    if (certs.length === 0) return
    setForm((p) => {
      if (!p.tlsTerminate || p.protocol === 'UDP' || p.domain.trim()) return p
      const filled = pickCertDomain(certs, p.certId)
      return filled ? { ...p, domain: filled } : p
    })
  }, [certs])

  const tlsOn = form.tlsTerminate && form.protocol !== 'UDP'
  const certDomains = useMemo(() => certDomainsOf(certs, form.certId), [certs, form.certId])
  const domainValue = form.domain.trim().toLowerCase()
  // 绑定了一张带域名的证书却不填对外域名，生成出来的地址必然是证书盖不住的公网 IP。
  // 正常情况下 pickCertDomain 已经自动填好了，走到这里只剩通配证书——
  // *.example.com 用哪个标签只有用户知道，替他猜不如让他填。
  const domainRequired = tlsOn && Number(form.certId) !== 0 && certDomains.length > 0
  const domainCovered = certDomains.some((d) => matchCertDomain(d, domainValue))

  const refreshWebhookTemplates = useCallback(async () => {
    setTemplatesLoading(true)
    try {
      const list = await api.getWebhookTemplates()
      setTemplates(
        list.map((item) => ({
          ...item,
          config: normalizeWebhook(item.config),
        })),
      )
    } catch (e) {
      setErr(e instanceof Error ? e.message : '读取模板失败')
    } finally {
      setTemplatesLoading(false)
    }
  }, [])

  useEffect(() => {
    refreshWebhookTemplates()
  }, [refreshWebhookTemplates])

  const updateWebhook = (patch: Partial<WebhookConfig>) => {
    setForm((p) => ({
      ...p,
      webhookconfig: {
        ...p.webhookconfig,
        ...patch,
      },
    }))
  }

  const applyWebhookTemplate = (templateId: string) => {
    const template = templates.find((item) => item.id === templateId)
    if (!template) return
    setForm((p) => ({
      ...p,
      webhookconfig: normalizeWebhook(template.config),
    }))
    setSelectedTemplateId(templateId)
    setTemplateName(template.builtin ? '' : template.name)
    setTemplateDescription(template.builtin ? '' : template.description)
    setActiveTab('webhook')
  }

  const saveCurrentWebhookTemplate = async () => {
    const name = templateName.trim()
    if (!name) {
      setErr('请填写模板名称')
      return
    }
    setTemplateBusy(true)
    try {
      const created = await api.addWebhookTemplate({
        name,
        description: templateDescription.trim(),
        config: normalizeWebhook(form.webhookconfig),
      })
      setTemplates((p) => [...p, { ...created, config: normalizeWebhook(created.config) }])
      setSelectedTemplateId(created.id)
      setTemplateName('')
      setTemplateDescription('')
      setErr('')
    } catch (e) {
      setErr(e instanceof Error ? e.message : '保存模板失败')
    } finally {
      setTemplateBusy(false)
    }
  }

  const updateCurrentWebhookTemplate = async () => {
    const current = templates.find((item) => item.id === selectedTemplateId)
    if (!current || current.builtin) {
      setErr('请选择一个自定义模板')
      return
    }
    const name = templateName.trim()
    if (!name) {
      setErr('请填写模板名称')
      return
    }
    setTemplateBusy(true)
    try {
      const updated = await api.updateWebhookTemplate({
        id: current.id,
        name,
        description: templateDescription.trim(),
        config: normalizeWebhook(form.webhookconfig),
      })
      setTemplates((p) =>
        p.map((item) => (
          item.id === updated.id ? { ...updated, config: normalizeWebhook(updated.config) } : item
        )),
      )
      setErr('')
    } catch (e) {
      setErr(e instanceof Error ? e.message : '更新模板失败')
    } finally {
      setTemplateBusy(false)
    }
  }

  const deleteSavedWebhookTemplate = async (templateId: string) => {
    setTemplateBusy(true)
    try {
      await api.deleteWebhookTemplate(templateId)
      setTemplates((p) => p.filter((item) => item.id !== templateId))
      if (selectedTemplateId === templateId) setSelectedTemplateId('')
      setTemplateName('')
      setTemplateDescription('')
    } catch (e) {
      setErr(e instanceof Error ? e.message : '删除模板失败')
    } finally {
      setTemplateBusy(false)
    }
  }

  const submit = async () => {
    if (!form.name.trim()) {
      setErr('请填写服务名称')
      return
    }
    const port = Number(form.internalPort)
    if (!port || port < 1 || port > 65535) {
      setErr('请填写有效的内网端口(1-65535)')
      return
    }
    if (domainRequired && !domainValue) {
      setErr(
        `请填写对外域名：绑定的证书覆盖 ${certDomains.join('、')}，留空会回落到公网 IP，证书盖不住 IP，浏览器会报证书错误`,
      )
      return
    }
    setErr('')
    setBusy(true)
    try {
      // UDP 没有洞口 TLS 一说，先勾选过再改协议的情况在这里抹掉
      await onSubmit(
        form.protocol === 'UDP' ? { ...form, tlsTerminate: false, certId: '0' } : form,
      )
    } catch (e) {
      setErr(e instanceof Error ? e.message : '操作失败')
      setBusy(false)
    }
  }

  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="flex max-h-[92vh] w-full max-w-3xl flex-col rounded-2xl bg-white text-slate-700 shadow-2xl ring-1 ring-slate-200">
        <div className="flex items-center justify-between px-6 py-5">
          <div className="text-base font-bold text-slate-800">{initial ? '编辑服务' : '添加服务'}</div>
          <button
            type="button"
            onClick={onCancel}
            className="grid h-7 w-7 place-items-center rounded-full text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="flex gap-8 border-b border-slate-100 px-6">
          {[
            { key: 'basic' as const, label: '基础配置' },
            { key: 'webhook' as const, label: 'Webhook' },
            { key: 'redirect' as const, label: 'CF 重定向' },
          ].map((tab) => (
            <button
              key={tab.key}
              type="button"
              onClick={() => openTab(tab.key)}
              className={`border-b-2 px-0 pb-3 text-sm font-bold transition ${
                activeTab === tab.key
                  ? 'border-blue-500 text-blue-600'
                  : 'border-transparent text-slate-500 hover:text-slate-800'
              }`}
            >
              {tab.label}
            </button>
          ))}
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-6 py-5">
          {activeTab === 'basic' && (
            <div className="grid grid-cols-2 gap-3">
              <label className="block col-span-2 sm:col-span-1">
                <div className="mb-1 text-xs font-semibold text-slate-500">服务名称</div>
                <input
                  autoFocus
                  value={form.name}
                  onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
                  placeholder="如 SSH / Web管理"
                  className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
                />
              </label>
              <label className="block col-span-2 sm:col-span-1">
                <div className="mb-1 text-xs font-semibold text-slate-500">内网端口</div>
                <input
                  type="number"
                  value={form.internalPort}
                  onChange={(e) => setForm((p) => ({ ...p, internalPort: e.target.value }))}
                  placeholder="如 22"
                  min={1}
                  max={65535}
                  className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
                />
              </label>
              <label className="block col-span-2 sm:col-span-1">
                <div className="mb-1 text-xs font-semibold text-slate-500">协议类型</div>
                <select
                  value={form.protocol}
                  onChange={(e) => setForm((p) => ({ ...p, protocol: e.target.value as 'TCP' | 'UDP' }))}
                  className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
                >
                  <option value="TCP">TCP</option>
                  <option value="UDP">UDP</option>
                </select>
              </label>
              <label className="block col-span-2 sm:col-span-1">
                <div className="mb-1 text-xs font-semibold text-slate-500">UPnP 映射端口</div>
                <input
                  type="number"
                  value={form.upnpMappedPort}
                  onChange={(e) => setForm((p) => ({ ...p, upnpMappedPort: e.target.value }))}
                  placeholder="0 表示自动"
                  min={0}
                  max={65535}
                  className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
                />
              </label>

              <div className="col-span-2 grid grid-cols-2 gap-2 sm:grid-cols-4">
                {[
                  { key: 'useUpnp' as const, label: '启用 UPnP', hint: '' },
                  {
                    key: 'https' as const,
                    label: '链接显示 https',
                    // 这个勾只改链接长什么样。真正决定「怎么连内网」的是下面对外访问里的
                    // 「转发给内网时也用 HTTPS」——两者必须让人一眼看出区别。
                    hint: '只影响首页/卡片上的链接写成 http:// 还是 https://，不改变转发行为',
                  },
                  { key: 'enabled' as const, label: '启用服务', hint: '' },
                  { key: 'showOnHome' as const, label: '主页显示', hint: '' },
                ].map((opt) => (
                  <label
                    key={opt.key}
                    title={opt.hint || undefined}
                    className="flex cursor-pointer items-center gap-2 rounded-xl bg-slate-50 px-3 py-2 text-xs font-medium text-slate-600"
                  >
                    <input
                      type="checkbox"
                      checked={form[opt.key]}
                      onChange={(e) => setForm((p) => ({ ...p, [opt.key]: e.target.checked }))}
                      className="h-3.5 w-3.5"
                    />
                    {opt.label}
                  </label>
                ))}
              </div>

              {/* 对外访问：域名 + 洞口终结 TLS。洞是 LinkStar 自己 Accept 的，
                  所以可以直接在洞口把 TLS 终结掉，外部看到的就是 https:// */}
              <div className="col-span-2 space-y-3 rounded-xl border border-slate-200/80 bg-slate-50/50 p-3">
                <div className="flex items-center gap-1.5 text-xs font-bold text-slate-700">
                  <ShieldCheck className="h-3.5 w-3.5 text-blue-500" />
                  对外访问
                </div>

                <label className="block">
                  <div className="mb-1 flex items-center gap-2">
                    <span className="text-xs font-semibold text-slate-500">对外域名</span>
                    {tlsOn && certDomains.length > 0 ? (
                      <span className="rounded bg-amber-100 px-1.5 py-0.5 text-[10px] font-bold text-amber-700">
                        终结 TLS 时必填
                      </span>
                    ) : (
                      <span className="text-[11px] text-slate-400">可选</span>
                    )}
                  </div>
                  <input
                    value={form.domain}
                    onChange={(e) => setForm((p) => ({ ...p, domain: e.target.value }))}
                    placeholder={
                      tlsOn ? '如 fw.example.com；必须是证书覆盖的域名' : '如 fw.example.com；留空则用公网 IP'
                    }
                    className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
                  />
                  <div className="mt-1 text-[11px] leading-relaxed text-slate-400">
                    该域名需要解析到你的公网 IP（可在 DDNS 页面配一条记录）。
                    多个服务共用同一域名不同端口时，Cookie 按 RFC 6265 §8.5 不做端口隔离，会话会互相覆盖——
                    建议每个服务用独立子域名。
                  </div>
                </label>

                {/* 终结 TLS = 洞口出示证书，而证书是按域名签的。对外域名留空会回落公网 IP，
                    IP 不发 SNI（RFC 6066）、证书也不可能覆盖 IP，首页链接和 /go 跳转就会
                    带用户去一个必报 ERR_CERT_COMMON_NAME_INVALID 的地址。 */}
                {tlsOn && certDomains.length > 0 && !domainValue && (
                  <div className="flex gap-2 rounded-xl bg-amber-50 px-3 py-2 text-[11px] leading-relaxed text-amber-700 ring-1 ring-amber-200">
                    <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                    <span>
                      证书覆盖的是{' '}
                      <span className="font-mono font-semibold">{certDomains.join('、')}</span>
                      ，这里留空就会回落到公网 IP。证书盖不住 IP，用 IP 访问必报
                      ERR_CERT_COMMON_NAME_INVALID——填一个证书覆盖的域名。
                    </span>
                  </div>
                )}

                {tlsOn && certDomains.length > 0 && domainValue && !domainCovered && (
                  <div className="flex gap-2 rounded-xl bg-amber-50 px-3 py-2 text-[11px] leading-relaxed text-amber-700 ring-1 ring-amber-200">
                    <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                    <span>
                      <span className="font-mono font-semibold">{domainValue}</span> 不在证书覆盖范围内（
                      {certDomains.join('、')}），浏览器会报 ERR_CERT_COMMON_NAME_INVALID。
                    </span>
                  </div>
                )}

                <label
                  className={`flex items-center gap-2 rounded-xl bg-white px-3 py-2 text-xs font-medium text-slate-600 ring-1 ring-slate-200 ${
                    form.protocol === 'UDP' ? 'cursor-not-allowed opacity-60' : 'cursor-pointer'
                  }`}
                >
                  <input
                    type="checkbox"
                    disabled={form.protocol === 'UDP'}
                    checked={form.tlsTerminate && form.protocol !== 'UDP'}
                    onChange={(e) =>
                      setForm((p) => ({
                        ...p,
                        tlsTerminate: e.target.checked,
                        // 不终结就没有「LinkStar 怎么拨内网」这回事，洞是纯管道，
                        // 留着这个勾会变成双层 TLS
                        backendHttps: e.target.checked ? p.backendHttps : false,
                        // 终结要出示证书，证书按域名签，对外域名就不能再空着回落 IP。
                        // 证书上有现成的具体域名就直接填上；用户已经填了的不动
                        domain:
                          e.target.checked && !p.domain.trim()
                            ? pickCertDomain(certs, p.certId)
                            : p.domain,
                      }))
                    }
                    className="h-3.5 w-3.5"
                  />
                  由 LinkStar 在洞口终结 TLS（外部访问变成 https://）
                </label>

                {form.protocol === 'UDP' && (
                  <div className="flex gap-2 text-[11px] leading-relaxed text-amber-600">
                    <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
                    UDP 洞口不支持终结 TLS，那是 DTLS，是另一套协议。
                  </div>
                )}

                {form.tlsTerminate && form.protocol !== 'UDP' && (
                  <label className="block">
                    <div className="mb-1 text-xs font-semibold text-slate-500">使用证书</div>
                    <select
                      value={form.certId}
                      onChange={(e) =>
                        setForm((p) => ({
                          ...p,
                          certId: e.target.value,
                          // 换证书时对外域名还空着，就用新证书上的域名补上
                          domain: p.domain.trim() ? p.domain : pickCertDomain(certs, e.target.value),
                        }))
                      }
                      className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
                    >
                      <option value="0">自动（按 SNI 域名匹配，匹配不上用默认证书）</option>
                      {certs.map((c) => (
                        <option key={c.id} value={String(c.id)}>
                          {c.name}
                          {c.domains.length > 0 ? ` — ${c.domains.join('、')}` : ''}
                        </option>
                      ))}
                    </select>
                    <div className="mt-1 text-[11px] leading-relaxed text-slate-400">
                      {certs.length === 0
                        ? '还没有证书，先去「证书」页添加一张，否则握手会失败。'
                        : '证书续期时只热替换内存里的指针，洞不会断，也不用重启穿透。'}
                    </div>
                  </label>
                )}

                {/* 「转发给内网时也用 HTTPS」只有在洞口终结了 TLS 时才成立：
                    那时 LinkStar 才真的要解密再加密一遍。洞口不终结时洞是纯字节管道，
                    LinkStar 一个字节都不解析，浏览器的 TLS 直达内网服务——
                    这时再让它 tls.Dial 就是双层 TLS，浏览器必报 ERR_SSL_PROTOCOL_ERROR。
                    所以这里做成父子关系，让那个组合压根勾不出来。 */}
                {form.tlsTerminate && form.protocol !== 'UDP' && (
                  <div className="ml-4 border-l-2 border-slate-200 pl-3">
                    <label className="flex cursor-pointer items-center gap-2 rounded-xl bg-white px-3 py-2 text-xs font-medium text-slate-600 ring-1 ring-slate-200">
                      <input
                        type="checkbox"
                        checked={form.backendHttps}
                        onChange={(e) => setForm((p) => ({ ...p, backendHttps: e.target.checked }))}
                        className="h-3.5 w-3.5"
                      />
                      转发给内网时也用 HTTPS（内网服务自己带证书就勾）
                    </label>
                    <div className="mt-1 text-[11px] leading-relaxed text-slate-400">
                      {form.backendHttps
                        ? '相当于 nginx 的 proxy_pass https://，自签证书不校验。内网其实是明文时会连不上。'
                        : '相当于 nginx 的 proxy_pass http://。绝大多数自建服务都是明文，保持不勾即可。'}
                    </div>
                  </div>
                )}

                {!form.tlsTerminate && form.protocol !== 'UDP' && (
                  <div className="rounded-xl bg-slate-50 px-3 py-2 text-[11px] leading-relaxed text-slate-500 ring-1 ring-slate-200">
                    不终结时洞口是纯管道，只搬字节不拆包。
                    <span className="font-semibold">内网服务自己是 HTTPS 也不用在这里填</span>
                    ——浏览器直接和它握手，用的就是它那张证书，外面照样是{' '}
                    <span className="font-mono">https://</span>。
                  </div>
                )}
              </div>

              <label className="col-span-2 block">
                <div className="mb-1 text-xs font-semibold text-slate-500">描述（可选）</div>
                <input
                  value={form.description}
                  onChange={(e) => setForm((p) => ({ ...p, description: e.target.value }))}
                  placeholder="服务描述信息"
                  className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
                />
              </label>
            </div>
          )}

          {activeTab === 'webhook' && (
            <div className="mx-auto max-w-2xl">
              <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
                <label className="flex cursor-pointer items-center gap-2 text-sm font-bold text-slate-800">
                <input
                  type="checkbox"
                  checked={form.webhookconfig.enabled}
                  onChange={(e) => updateWebhook({ enabled: e.target.checked })}
                  className="h-3.5 w-3.5"
                />
                <Send className="h-4 w-4 text-blue-500" />
                Webhook
              </label>
                <div className="flex flex-wrap items-center gap-2">
                  <button
                    type="button"
                    onClick={() => updateWebhook({ enabled: !form.webhookconfig.enabled })}
                    className={`rounded-md px-2 py-1 text-xs font-semibold transition ${
                      form.webhookconfig.enabled
                        ? 'bg-blue-50 text-blue-600'
                        : 'bg-slate-100 text-slate-500 hover:bg-slate-200'
                    }`}
                  >
                    {form.webhookconfig.enabled ? '已启用' : '未启用'}
                  </button>
                </div>
              </div>

              <div className="mb-4 rounded-xl bg-slate-50 p-3">
                <div className="mb-2 flex items-center gap-2 text-xs font-bold text-slate-600">
                  <LayoutTemplate className="h-3.5 w-3.5 text-blue-500" />
                  选择模板，填入下方表单
                </div>
                <div className="flex gap-2 overflow-x-auto pb-1">
                  {templatesLoading && (
                    <div className="w-44 shrink-0 rounded-xl border border-slate-200 bg-white p-2.5 text-xs text-slate-400">
                      模板加载中...
                    </div>
                  )}
                  {!templatesLoading && templates.length === 0 && (
                    <div className="w-44 shrink-0 rounded-xl border border-slate-200 bg-white p-2.5 text-xs text-slate-400">
                      暂无模板
                    </div>
                  )}
                  {templates.map((template) => (
                    <div
                      key={template.id}
                      className={`flex w-52 shrink-0 items-start gap-2 rounded-xl border bg-white p-2.5 transition ${
                        selectedTemplateId === template.id
                          ? 'border-blue-400 bg-blue-50/70'
                          : 'border-slate-200 hover:border-blue-300 hover:bg-blue-50/60'
                      }`}
                    >
                      <button
                        type="button"
                        onClick={() => applyWebhookTemplate(template.id)}
                        className="min-w-0 flex-1 text-left"
                      >
                        <div className="flex items-center gap-1.5">
                          <span className="truncate text-xs font-bold text-slate-700">{template.name}</span>
                          {template.builtin && (
                            <span className="shrink-0 rounded bg-blue-50 px-1 py-0.5 text-[10px] font-semibold text-blue-500">
                              内置
                            </span>
                          )}
                        </div>
                        <div className="mt-1 line-clamp-2 text-[11px] leading-4 text-slate-400">{template.description}</div>
                        <div className="mt-2 truncate font-mono text-[10px] text-slate-400">
                          {template.config.method} {template.config.url || '未填写 URL'}
                        </div>
                      </button>
                      {!template.builtin && (
                        <button
                          type="button"
                          disabled={templateBusy}
                          onClick={() => deleteSavedWebhookTemplate(template.id)}
                          className="grid h-6 w-6 shrink-0 place-items-center rounded-md text-slate-400 transition hover:bg-rose-50 hover:text-rose-500 disabled:opacity-40"
                          title="删除模板"
                        >
                          <X className="h-3.5 w-3.5" />
                        </button>
                      )}
                    </div>
                  ))}
                </div>
              </div>

            <div className="grid grid-cols-2 gap-3">
              <label className="col-span-2 block sm:col-span-1">
                <div className="mb-1 text-xs font-semibold text-slate-500">请求方法</div>
                <select
                  value={form.webhookconfig.method}
                  disabled={!form.webhookconfig.enabled}
                  onChange={(e) => updateWebhook({ method: e.target.value })}
                  className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400"
                >
                  {webhookMethods.map((method) => (
                    <option key={method} value={method}>{method}</option>
                  ))}
                </select>
              </label>
              <label className="col-span-2 block sm:col-span-1">
                <div className="mb-1 text-xs font-semibold text-slate-500">代理（可选）</div>
                <input
                  value={form.webhookconfig.proxy}
                  disabled={!form.webhookconfig.enabled}
                  onChange={(e) => updateWebhook({ proxy: e.target.value })}
                  placeholder="http://127.0.0.1:7890"
                  className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400"
                />
              </label>
              <label className="col-span-2 block">
                <div className="mb-1 text-xs font-semibold text-slate-500">Webhook URL</div>
                <input
                  value={form.webhookconfig.url}
                  disabled={!form.webhookconfig.enabled}
                  onChange={(e) => updateWebhook({ url: e.target.value })}
                  placeholder="https://example.com/hook?addr=#{address}"
                  className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400"
                />
              </label>
              <label className="col-span-2 block">
                <div className="mb-1 text-xs font-semibold text-slate-500">Headers</div>
                <textarea
                  value={form.webhookconfig.headers}
                  disabled={!form.webhookconfig.enabled}
                  onChange={(e) => updateWebhook({ headers: e.target.value })}
                  rows={3}
                  placeholder={'Content-Type: application/json\nAuthorization: Bearer xxx'}
                  className="w-full resize-y rounded-xl border border-slate-200 bg-white px-3 py-2 font-mono text-xs outline-none focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400"
                />
              </label>
              {/* CF 重定向那份模板的 JSON 就有十几行，原来 4 行的框要一直滚才能看全一对括号 */}
              <label className="col-span-2 block">
                <div className="mb-1 text-xs font-semibold text-slate-500">Body</div>
                <textarea
                  value={form.webhookconfig.body}
                  disabled={!form.webhookconfig.enabled}
                  onChange={(e) => updateWebhook({ body: e.target.value })}
                  rows={14}
                  className="w-full resize-y rounded-xl border border-slate-200 bg-white px-3 py-2 font-mono text-xs leading-5 outline-none focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400"
                />
              </label>
              <div className="col-span-2 grid grid-cols-1 gap-2 sm:grid-cols-2">
                <label className="flex cursor-pointer items-center gap-2 rounded-xl bg-slate-50 px-3 py-2 text-xs font-medium text-slate-600">
                  <input
                    type="checkbox"
                    checked={form.webhookconfig.onlyWhenChanged}
                    disabled={!form.webhookconfig.enabled}
                    onChange={(e) => updateWebhook({ onlyWhenChanged: e.target.checked })}
                    className="h-3.5 w-3.5"
                  />
                  地址变化时才触发
                </label>
                <label className="flex cursor-pointer items-center gap-2 rounded-xl bg-slate-50 px-3 py-2 text-xs font-medium text-slate-600">
                  <input
                    type="checkbox"
                    checked={form.webhookconfig.disableSuccessCheck}
                    disabled={!form.webhookconfig.enabled}
                    onChange={(e) => updateWebhook({ disableSuccessCheck: e.target.checked })}
                    className="h-3.5 w-3.5"
                  />
                  只检查 HTTP 2xx
                </label>
              </div>
              {!form.webhookconfig.disableSuccessCheck && (
                <label className="col-span-2 block">
                  <div className="mb-1 text-xs font-semibold text-slate-500">成功包含文本</div>
                  <input
                    value={form.webhookconfig.successContains}
                    disabled={!form.webhookconfig.enabled}
                    onChange={(e) => updateWebhook({ successContains: e.target.value })}
                    className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400"
                  />
                </label>
              )}
              <div className="col-span-2 grid grid-cols-1 gap-2 sm:grid-cols-[minmax(0,1fr)_minmax(0,1fr)_auto_auto]">
                <input
                  value={templateName}
                  onChange={(e) => setTemplateName(e.target.value)}
                  placeholder="模板名称"
                  className="min-w-0 rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
                />
                <input
                  value={templateDescription}
                  onChange={(e) => setTemplateDescription(e.target.value)}
                  placeholder="模板描述"
                  className="min-w-0 rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
                />
                <button
                  type="button"
                  onClick={saveCurrentWebhookTemplate}
                  disabled={templateBusy}
                  className="grid h-9 w-9 shrink-0 place-items-center rounded-xl bg-blue-500 text-white shadow-sm shadow-blue-500/20 transition hover:bg-blue-600 disabled:opacity-50"
                  title="保存当前为模板"
                >
                  <BookmarkPlus className="h-4 w-4" />
                </button>
                <button
                  type="button"
                  onClick={updateCurrentWebhookTemplate}
                  disabled={templateBusy || !selectedTemplateId || templates.find((item) => item.id === selectedTemplateId)?.builtin}
                  className="grid h-9 w-9 shrink-0 place-items-center rounded-xl bg-slate-700 text-white shadow-sm transition hover:bg-slate-800 disabled:opacity-40"
                  title="更新选中的自定义模板"
                >
                  <Pencil className="h-4 w-4" />
                </button>
              </div>
            </div>
            </div>
          )}

          {activeTab === 'redirect' && (
            <div className="mx-auto max-w-2xl">
              <div className="mb-4 flex flex-wrap items-center justify-between gap-3">
                <label className="flex cursor-pointer items-center gap-2 text-sm font-bold text-slate-800">
                  <input
                    type="checkbox"
                    checked={form.redirect.enabled}
                    onChange={(e) => updateRedirect({ enabled: e.target.checked })}
                    className="h-3.5 w-3.5"
                  />
                  <CornerUpRight className="h-4 w-4 text-blue-500" />
                  CF 重定向
                </label>
                <span
                  className={`rounded-md px-2 py-1 text-xs font-semibold ${
                    form.redirect.enabled ? 'bg-blue-50 text-blue-600' : 'bg-slate-100 text-slate-500'
                  }`}
                >
                  {form.redirect.enabled ? '已启用' : '未启用'}
                </span>
              </div>

              <div className="mb-4 rounded-xl bg-slate-50 p-3 text-xs leading-5 text-slate-500">
                外网端口一变，Cloudflare 那条重定向规则跟着改。用户永远访问下面这个固定域名，
                Cloudflare 307 跳到这个服务当前的地址。
                <div className="mt-2 flex flex-wrap items-center gap-2 font-mono text-[11px] text-slate-600">
                  <span className="rounded-md bg-white px-2 py-1 ring-1 ring-slate-200">
                    {form.redirect.entryHost.trim() || '入口域名'}
                  </span>
                  <span className="text-slate-400">— 307 →</span>
                  <span className="rounded-md bg-white px-2 py-1 ring-1 ring-slate-200">
                    {redirectPreviewTarget || '等服务跑起来才知道端口'}
                  </span>
                </div>
                <div className="mt-2">
                  zone / ruleset / rule 三个 ID 都不用填，LinkStar 自己查；
                  你在 Cloudflare 后台手写的其它规则不会被动。
                </div>
              </div>

              {/*
                规则写对了也可能访问不了，差的是这两条解析记录。
                它们的状态只有 Cloudflare 那边知道，本地配置里一点都看不出来，
                出了问题也不报错——只表现为「打不开」。所以现场查出来摆在这儿。
              */}
              {initial && savedRedirectOn && (
                <div className="mb-4 rounded-xl border border-slate-200 p-3">
                  <div className="mb-2 flex items-center justify-between gap-2">
                    <div className="text-xs font-semibold text-slate-500">Cloudflare 上的解析记录</div>
                    <button
                      type="button"
                      disabled={inspecting}
                      onClick={() => void loadInspect()}
                      className="inline-flex items-center gap-1 rounded-lg px-1.5 py-0.5 text-[11px] font-semibold text-slate-400 transition hover:bg-slate-100 hover:text-slate-600 disabled:opacity-50"
                    >
                      <RotateCw className={`h-3 w-3 ${inspecting ? 'animate-spin' : ''}`} />
                      {inspecting ? '查询中' : '刷新'}
                    </button>
                  </div>

                  {inspectErr && <div className="text-[11px] leading-5 text-rose-500">查不了：{inspectErr}</div>}
                  {!inspectErr && !inspect && (
                    <div className="text-[11px] text-slate-400">
                      {inspecting ? '正在去 Cloudflare 查…' : '点右上角刷新看现在什么样'}
                    </div>
                  )}

                  {inspect && (
                    <div className="space-y-2.5">
                      <div>
                        <div className="flex flex-wrap items-center gap-1.5 text-[11px]">
                          <span className="rounded bg-slate-100 px-1.5 py-0.5 font-semibold text-slate-500">
                            入口
                          </span>
                          <span className="font-mono text-slate-700">{inspect.entry.host}</span>
                          {inspect.entry.found && (
                            <span className="font-mono text-slate-400">
                              {inspect.entry.type} {inspect.entry.content}
                            </span>
                          )}
                          {inspect.entry.byLinkStar && (
                            <span className="text-slate-400">· LinkStar 建的</span>
                          )}
                        </div>
                        <div className="mt-1 text-[11px] leading-5">
                          {inspect.entry.warn ? (
                            <span className="text-amber-600">{inspect.entry.warn}</span>
                          ) : !inspect.entry.found ? (
                            <span className="text-rose-500">
                              还没有这条记录，现在访问它会提示域名不存在。点下面「立即同步」，LinkStar
                              会建好
                            </span>
                          ) : inspect.entry.proxied ? (
                            <span className="text-emerald-600">小黄云开着，重定向能生效</span>
                          ) : (
                            <span className="text-amber-600">
                              小黄云是关的 ——
                              请求根本到不了 Cloudflare，重定向不会执行，访问只会停在{' '}
                              <span className="font-mono">{inspect.entry.wantIP}</span>{' '}
                              这个谁都不在的地址上。去 Cloudflare 的 DNS
                              里把这条记录的云朵点成橙色
                            </span>
                          )}
                        </div>
                      </div>

                      <div>
                        <div className="flex flex-wrap items-center gap-1.5 text-[11px]">
                          <span className="rounded bg-slate-100 px-1.5 py-0.5 font-semibold text-slate-500">
                            落地
                          </span>
                          <span className="font-mono text-slate-700">
                            {inspect.landingHost || '还没定'}
                          </span>
                          {inspect.landing.managed && inspect.landing.lastIP && (
                            <span className="font-mono text-slate-400">A {inspect.landing.lastIP}</span>
                          )}
                        </div>
                        <div className="mt-1 text-[11px] leading-5">
                          {!inspect.landingHost ? (
                            <span className="text-slate-400">
                              这个服务没填对外域名，307 直接跳到公网 IP，不需要解析记录
                            </span>
                          ) : inspect.landing.managed && inspect.landing.status === 'failed' ? (
                            // 「在改」和「没改成」凑一句话会自相矛盾，失败就只说失败
                            <span className="text-amber-600">
                              DDNS 记录「{inspect.landing.name}」上次没改成 ——{' '}
                              {inspect.landing.message || '未知原因'}
                            </span>
                          ) : inspect.landing.managed ? (
                            <span className="text-emerald-600">
                              DDNS 记录「{inspect.landing.name}」跟着公网 IP 在改
                              {fmtTime(inspect.landing.at) ? `（${fmtTime(inspect.landing.at)}）` : ''}
                            </span>
                          ) : (
                            <span className="text-amber-600">
                              DDNS 里没有记录管它，公网 IP
                              一变这个域名就指错地方了。点一次下面的「立即同步」，LinkStar 会加上
                            </span>
                          )}
                        </div>
                      </div>
                    </div>
                  )}

                  <div className="mt-2.5 border-t border-slate-100 pt-2 text-[11px] leading-5 text-slate-400">
                    两条的要求正好相反：入口那条要开小黄云，落地那条要关 —— 开了的话 Cloudflare
                    不转发打洞出来的高位端口，一样访问不了。
                  </div>
                </div>
              )}

              <div className="grid grid-cols-2 gap-3">
                <label className="col-span-2 block sm:col-span-1">
                  <div className="mb-1 text-xs font-semibold text-slate-500">Cloudflare 账号</div>
                  <select
                    value={String(form.redirect.providerId || 0)}
                    disabled={!form.redirect.enabled}
                    onChange={(e) => updateRedirect({ providerId: Number(e.target.value) || 0 })}
                    className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400"
                  >
                    <option value="0">请选择</option>
                    {cfProviders.map((p) => (
                      <option key={p.id} value={String(p.id)}>
                        {p.name}
                      </option>
                    ))}
                  </select>
                  <div className="mt-1 text-[11px] text-slate-400">
                    {cfProviders.length === 0
                      ? '"DDNS" 页面里还没有 Cloudflare 账号，先去那边加一个'
                      : '用 DDNS 里配好的那个，Token 不用再贴一遍'}
                  </div>
                  {/* 只有 DNS 权限的 Token 解析能用、重定向必 403，事后看报错很难想到这一层 */}
                  {cfProviders.length > 0 && (
                    <div className="mt-1 text-[11px] text-amber-600">
                      这个 Token 要同时有 Zone → DNS → 编辑 和 Zone → Dynamic URL Redirects →
                      编辑，两项得在同一条策略里，少一项会报 403
                    </div>
                  )}
                </label>

                <label className="col-span-2 block sm:col-span-1">
                  <div className="mb-1 text-xs font-semibold text-slate-500">入口域名</div>
                  <input
                    value={form.redirect.entryHost}
                    disabled={!form.redirect.enabled}
                    onChange={(e) => updateRedirect({ entryHost: e.target.value })}
                    placeholder="linkstar.example.com"
                    className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400"
                  />
                  <div className="mt-1 text-[11px] text-slate-400">
                    外面记的就是这个名字。保存后 LinkStar 会顺手在 Cloudflare 建好这条记录（A
                    记录 + 小黄云），不用自己去加。别填成下面那个落地域名——那条得关着小黄云
                  </div>
                </label>

                <label className="col-span-2 block sm:col-span-1">
                  <div className="mb-1 text-xs font-semibold text-slate-500">主域名</div>
                  <input
                    value={form.redirect.zoneDomain}
                    disabled={!form.redirect.enabled}
                    onChange={(e) => updateRedirect({ zoneDomain: e.target.value })}
                    placeholder={redirectGuessedZone || 'example.com'}
                    className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400"
                  />
                  <div className="mt-1 text-[11px] text-slate-400">
                    {redirectGuessedZone
                      ? `留空就按 ${redirectGuessedZone} 算；域名多一层（如 a.b.co.uk）才需要填`
                      : '留空按入口域名的后两段算'}
                  </div>
                </label>
              </div>

              {initial ? (
                <div className="mt-4 rounded-xl border border-slate-200 p-3">
                  <div className="mb-2 text-xs font-semibold text-slate-500">最近一次同步</div>
                  {status?.redirectStatus === 'ok' && (
                    <div className="text-xs text-emerald-600">
                      成功 → <span className="font-mono">{status.redirectTarget}</span>
                      {status.redirectAt ? `（${fmtTime(status.redirectAt)}）` : ''}
                      {status.redirectKeepPath === false && (
                        <div className="mt-1 text-amber-600">
                          服务商没接受保留路径的写法，从子路径进来会落到根路径
                        </div>
                      )}
                    </div>
                  )}
                  {status?.redirectStatus === 'failed' && (
                    <div className="text-xs text-rose-500">
                      失败：{status.redirectError || '未知原因'}
                      {status.redirectAt ? `（${fmtTime(status.redirectAt)}）` : ''}
                    </div>
                  )}
                  {!status?.redirectStatus && (
                    <div className="text-xs text-slate-400">还没同步过</div>
                  )}

                  <div className="mt-3 flex flex-wrap items-center gap-2">
                    <button
                      type="button"
                      disabled={redirectBusy || redirectDirty}
                      onClick={() => runRedirect('sync')}
                      className="inline-flex items-center gap-1.5 rounded-xl bg-blue-500 px-3 py-1.5 text-xs font-semibold text-white transition hover:bg-blue-600 disabled:opacity-50"
                    >
                      <RotateCw className="h-3.5 w-3.5" />
                      立即同步
                    </button>
                    <button
                      type="button"
                      disabled={redirectBusy || redirectDirty}
                      onClick={() => runRedirect('remove')}
                      className="inline-flex items-center gap-1.5 rounded-xl bg-slate-100 px-3 py-1.5 text-xs font-semibold text-slate-600 transition hover:bg-rose-50 hover:text-rose-500 disabled:opacity-50"
                    >
                      <Trash2 className="h-3.5 w-3.5" />
                      删掉 Cloudflare 上的规则
                    </button>
                    {redirectDirty ? (
                      <span className="text-[11px] font-semibold text-amber-600">
                        上面改的还没保存 —— 点右下角「保存」，几秒后自己就同步了，不用回来点这两个
                      </span>
                    ) : (
                      <span className="text-[11px] text-slate-400">
                        端口一变会自动同步，这里是手动再跑一遍
                      </span>
                    )}
                  </div>
                </div>
              ) : (
                <div className="mt-4 text-[11px] text-slate-400">先把服务保存了，再回来点同步</div>
              )}
            </div>
          )}
        </div>

        {err && <div className="px-6 pb-2 text-xs text-rose-500">{err}</div>}
        <div className="flex justify-end gap-2 border-t border-slate-100 px-6 py-4">
          <button
            type="button"
            onClick={onCancel}
            className="rounded-xl bg-slate-100 px-4 py-2 text-sm font-semibold text-slate-600 transition hover:bg-slate-200"
          >
            取消
          </button>
          <button
            type="button"
            onClick={submit}
            disabled={busy}
            className="rounded-xl bg-blue-500 px-4 py-2 text-sm font-semibold text-white shadow-md shadow-blue-500/20 transition hover:bg-blue-600 disabled:opacity-50"
          >
            {busy ? '保存中...' : '保存'}
          </button>
        </div>
      </div>
    </div>
  )
}

function LogModal({
  status,
  onClose,
}: {
  status: StunStatusEvent | null
  onClose: () => void
}) {
  const bodyRef = useRef<HTMLDivElement>(null)
  useEffect(() => {
    if (!bodyRef.current) return
    const el = bodyRef.current
    const nearBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 40
    if (nearBottom) el.scrollTop = el.scrollHeight
  }, [status?.logs?.length])

  if (!status) return null
  const logs = status.logs ?? []
  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div className="flex w-full max-w-2xl flex-col rounded-2xl bg-white shadow-2xl ring-1 ring-slate-200">
        <div className="flex items-center justify-between border-b border-slate-100 px-6 py-4">
          <div className="text-base font-bold text-slate-800">
            {status.deviceName || ''} / {status.serviceName || ''} - 服务日志
          </div>
          <button
            type="button"
            onClick={onClose}
            className="grid h-7 w-7 place-items-center rounded-full text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="px-6 py-3 text-xs text-slate-500">
          当前阶段：<span className="font-semibold text-slate-700">{phaseLabel[status.phaseStr] || status.phaseStr}</span>
          {' · '}重启次数：<span className="font-semibold text-slate-700">{status.restartCount ?? 0}</span>
          {' · '}共 {logs.length} 条日志
        </div>
        <div ref={bodyRef} className="max-h-[60vh] overflow-y-auto bg-slate-50 px-6 py-3">
          {logs.length === 0 ? (
            <div className="py-8 text-center text-sm text-slate-400">暂无日志</div>
          ) : (
            logs.map((l, i) => (
              <div
                key={i}
                className="flex items-start gap-3 border-b border-slate-100 py-1.5 text-xs last:border-0"
              >
                <span className="font-mono text-[11px] text-slate-400 whitespace-nowrap">
                  {new Date(l.createdAt).toLocaleString()}
                </span>
                <span className="shrink-0 rounded-md bg-blue-50 px-1.5 py-0.5 text-[10px] font-bold text-blue-600">
                  {phaseLabel[l.phaseStr] || l.phaseStr}
                </span>
                <span className="break-all text-slate-700">{l.message}</span>
              </div>
            ))
          )}
        </div>
      </div>
    </div>
  )
}

const mappingLabels: Record<string, string> = {
  EIM: '端点无关映射',
  ADM: '地址相关映射',
  APDM: '地址和端口相关映射',
  unknown: '未知',
}

const filteringLabels: Record<string, string> = {
  EIF: '端点无关过滤',
  ADF: '地址相关过滤',
  APDF: '地址和端口相关过滤',
  unknown: '未知',
}

function behaviorText(value: string, labels: Record<string, string>) {
  if (!value) return '--'
  const label = labels[value]
  return label ? `${label} (${value})` : value
}

function natTypeTone(natType: string) {
  if (natType.includes('NAT1')) return { panel: 'border-emerald-200 bg-emerald-50/40', badge: 'bg-emerald-100 text-emerald-700' }
  if (natType.includes('NAT2')) return { panel: 'border-sky-200 bg-sky-50/40', badge: 'bg-sky-100 text-sky-700' }
  if (natType.includes('NAT3')) return { panel: 'border-amber-200 bg-amber-50/40', badge: 'bg-amber-100 text-amber-700' }
  if (natType.includes('NAT4') || natType.includes('阻断')) return { panel: 'border-rose-200 bg-rose-50/40', badge: 'bg-rose-100 text-rose-700' }
  if (natType.includes('公网')) return { panel: 'border-teal-200 bg-teal-50/40', badge: 'bg-teal-100 text-teal-700' }
  return { panel: 'border-slate-200 bg-slate-50/70', badge: 'bg-slate-100 text-slate-600' }
}

function NatProtocolResult({ protocol, result }: { protocol: 'UDP' | 'TCP'; result: NatDetectResult | null }) {
  const tone = natTypeTone(result?.natType ?? '')
  const ProtocolIcon = protocol === 'UDP' ? Wifi : Server

  if (!result) {
    return (
      <section className="rounded-lg border border-dashed border-slate-200 bg-slate-50/70 p-4">
        <div className="flex items-center gap-2 text-sm font-bold text-slate-700">
          <ProtocolIcon className="h-4 w-4 text-slate-400" /> {protocol}
        </div>
        <div className="mt-5 text-center text-sm text-slate-400">尚无检测结果</div>
      </section>
    )
  }

  return (
    <section className={`rounded-lg border p-4 ${tone.panel}`}>
      <div className="flex items-center justify-between gap-3">
        <div className="flex items-center gap-2">
          <span className="grid h-8 w-8 shrink-0 place-items-center rounded-md bg-white text-slate-600 shadow-sm ring-1 ring-slate-200/80">
            <ProtocolIcon className="h-4 w-4" />
          </span>
          <div>
            <div className="text-sm font-bold text-slate-800">{protocol}</div>
            <div className="text-[11px] text-slate-400">{protocol === 'UDP' ? 'Mapping + Filtering' : 'Mapping'}</div>
          </div>
        </div>
        <span className={`shrink-0 rounded-md px-2 py-1 text-[11px] font-bold ${tone.badge}`}>
          {result.mappingErr ? '部分失败' : '检测完成'}
        </span>
      </div>

      <div className="mt-4">
        <div className="text-[11px] font-semibold text-slate-400">NAT 类型</div>
        <div className="mt-1 break-words text-sm font-bold leading-5 text-slate-800">{result.natType || '无法确定'}</div>
      </div>

      <dl className="mt-4 grid gap-2 text-xs sm:grid-cols-2">
        <div className="min-w-0 rounded-md bg-white/80 px-3 py-2 ring-1 ring-slate-200/70">
          <dt className="text-[11px] font-semibold text-slate-400">Mapping</dt>
          <dd className="mt-1 break-words font-medium leading-5 text-slate-700">{behaviorText(result.mapping, mappingLabels)}</dd>
          {result.mapping && result.mapping !== 'unknown' && (
            <dd className="mt-1.5 text-[10px] leading-relaxed text-slate-500">
              {result.mapping === 'EIM' && '✓ 最佳：同一端口对所有目标使用相同映射，打洞成功率最高'}
              {result.mapping === 'ADM' && '△ 中等：不同目标 IP 使用不同映射，需要端口预测'}
              {result.mapping === 'APDM' && '✗ 困难：每个目标 IP+端口都不同，打洞成功率低'}
            </dd>
          )}
        </div>
        <div className="min-w-0 rounded-md bg-white/80 px-3 py-2 ring-1 ring-slate-200/70">
          <dt className="text-[11px] font-semibold text-slate-400">Filtering</dt>
          <dd className="mt-1 break-words font-medium leading-5 text-slate-700">
            {protocol === 'TCP' ? 'TCP 不适用' : behaviorText(result.filtering, filteringLabels)}
          </dd>
          {protocol === 'TCP' ? (
            <dd className="mt-1.5 text-[10px] leading-relaxed text-slate-500">
              TCP 面向连接特性导致 Filtering 无法准确测试
            </dd>
          ) : result.filtering && result.filtering !== 'unknown' ? (
            <dd className="mt-1.5 text-[10px] leading-relaxed text-slate-500">
              {result.filtering === 'EIF' && '✓ 宽松：任何外部地址都能连入'}
              {result.filtering === 'ADF' && '△ 中等：需要先向目标发包建立连接'}
              {result.filtering === 'APDF' && '○ 严格：需要精确匹配端口，但 EIM 下仍可打洞'}
            </dd>
          ) : null}
        </div>
      </dl>
      {(result.mappingErr || result.filteringErr) && (
        <div className="mt-3 space-y-1 rounded-md border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-700">
          {result.mappingErr && <div className="break-words">Mapping: {result.mappingErr}</div>}
          {result.filteringErr && <div className="break-words">Filtering: {result.filteringErr}</div>}
        </div>
      )}
    </section>
  )
}

function NatTypeModal({ onClose }: { onClose: () => void }) {
  const [data, setData] = useState<NatTypeInfo | null>(null)
  const [loading, setLoading] = useState(true)
  const [detecting, setDetecting] = useState(false)
  const [error, setError] = useState('')

  const load = useCallback(async () => {
    setLoading(true)
    setError('')
    try {
      setData(await api.getNatType())
    } catch (e) {
      setError(e instanceof Error ? e.message : '加载失败')
    } finally {
      setLoading(false)
    }
  }, [])

  const detect = async () => {
    setDetecting(true)
    setError('')
    try {
      setData(await api.detectNatType())
    } catch (e) {
      setError(e instanceof Error ? e.message : '重新检测失败')
    } finally {
      setDetecting(false)
    }
  }

  useEffect(() => {
    let active = true
    api.getNatType()
      .then((result) => {
        if (active) setData(result)
      })
      .catch((e) => {
        if (active) setError(e instanceof Error ? e.message : '加载失败')
      })
      .finally(() => {
        if (active) setLoading(false)
      })
    return () => {
      active = false
    }
  }, [])

  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onClose()
      }}
    >
      <div className="flex max-h-[88vh] w-full max-w-2xl flex-col rounded-lg bg-white shadow-2xl ring-1 ring-slate-200">
        <div className="flex items-center justify-between gap-4 border-b border-slate-100 px-5 py-4">
          <div className="flex min-w-0 items-center gap-3">
            <span className="grid h-9 w-9 shrink-0 place-items-center rounded-lg bg-blue-50 text-blue-600 ring-1 ring-blue-100">
              <Info className="h-4.5 w-4.5" />
            </span>
            <div className="min-w-0">
              <div className="text-base font-bold text-slate-800">NAT 类型详情</div>
              <div className="mt-0.5 text-xs text-slate-400">RFC 5780 行为检测</div>
            </div>
          </div>
          <button
            type="button"
            onClick={onClose}
            title="关闭"
            aria-label="关闭"
            className="grid h-8 w-8 place-items-center rounded-md text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="overflow-y-auto p-5">
          {loading ? (
            <div className="flex min-h-48 items-center justify-center gap-2 text-sm text-slate-400">
              <LoaderCircle className="h-4 w-4 animate-spin" /> 正在加载
            </div>
          ) : error ? (
            <div className="flex min-h-48 flex-col items-center justify-center gap-3 text-sm text-rose-600">
              <AlertCircle className="h-6 w-6" />
              <span>{error}</span>
              <button
                type="button"
                onClick={load}
                className="flex items-center gap-1 rounded-md border border-slate-200 px-3 py-1.5 text-xs font-semibold text-slate-600 hover:bg-slate-50"
              >
                <RotateCw className="h-3.5 w-3.5" /> 重试
              </button>
            </div>
          ) : (
            <div className="space-y-3">
              {data?.error && (
                <div className="flex items-start gap-2 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs leading-5 text-amber-800">
                  <AlertCircle className="mt-0.5 h-4 w-4 shrink-0" />
                  <span className="break-words">部分探测未完成：{data.error}</span>
                </div>
              )}
              <div className="grid gap-3 md:grid-cols-2">
                <NatProtocolResult protocol="UDP" result={data?.udp ?? null} />
                <NatProtocolResult protocol="TCP" result={data?.tcp ?? null} />
              </div>
            </div>
          )}
        </div>
        {!loading && !error && (
          <div className="flex flex-wrap items-center justify-between gap-3 border-t border-slate-100 px-5 py-3">
            <span className="text-xs text-slate-400">重新检测通常需要数秒</span>
            <button
              type="button"
              onClick={detect}
              disabled={detecting}
              className="flex min-w-[104px] items-center justify-center gap-1.5 rounded-md bg-blue-600 px-3 py-2 text-xs font-semibold text-white shadow-sm transition hover:bg-blue-700 disabled:cursor-wait disabled:opacity-60"
            >
              <RotateCw className={`h-3.5 w-3.5 ${detecting ? 'animate-spin' : ''}`} />
              {detecting ? '检测中...' : '重新检测'}
            </button>
          </div>
        )}
      </div>
    </div>
  )
}

export function Stun() {
  const [config, setConfig] = useState<StunConfig | null>(null)
  const [loadErr, setLoadErr] = useState('')
  const [statusMap, setStatusMap] = useState<Record<string, StunStatusEvent>>({})
  const [homeSet, setHomeSet] = useState<Set<string>>(new Set())
  const [selectedIndex, setSelectedIndex] = useState(0)
  const { toasts, show: toast } = useToast()

  const [deviceModal, setDeviceModal] = useState<{
    open: boolean
    initial?: StunDevice
    prefill?: DeviceFormState
  }>({ open: false })
  const [scanModal, setScanModal] = useState<{ open: boolean; subnets: LanSubnet[] }>({
    open: false,
    subnets: [],
  })
  const [scanOpening, setScanOpening] = useState(false)
  // 正在切启停的服务，键是「设备ID-服务ID」
  const [toggling, setToggling] = useState<Record<string, boolean>>({})
  const [serviceModal, setServiceModal] = useState<{
    open: boolean
    deviceId: number
    initial?: StunService
    prefill?: Partial<ServiceFormState>
  }>({ open: false, deviceId: 0 })
  const [logKey, setLogKey] = useState<string | null>(null)
  const [natTypeOpen, setNatTypeOpen] = useState(false)

  const refresh = useCallback(async () => {
    try {
      const [cfg, home] = await Promise.all([api.getStunConfig(), api.getHomeConfig().catch(() => ({ apps: [] as api.HomeApp[] }))])
      setConfig(cfg)
      const set = new Set<string>()
      for (const app of home.apps) {
        if (app.type === 'stun') {
          const m = /^stun-(\d+)-(\d+)$/.exec(app.id || '')
          if (m) set.add(`${m[1]}-${m[2]}`)
        }
      }
      setHomeSet(set)
    } catch (e) {
      setLoadErr(e instanceof Error ? e.message : String(e))
    }
  }, [])

  useEffect(() => {
    refresh()
    const t = window.setInterval(refresh, 30000)
    return () => window.clearInterval(t)
  }, [refresh])

  useEffect(() => {
    const es = api.subscribeStunStatus((data) => {
      try {
        const evt = JSON.parse(data) as StunStatusEvent
        setStatusMap((p) => ({ ...p, [evt.key]: evt }))
      } catch {
        // ignore
      }
    })
    return () => es.close()
  }, [])

  const device = config?.devices?.[selectedIndex]

  const submitDevice = async (form: DeviceFormState) => {
    if (deviceModal.initial) {
      await api.updateStunDevice({
        deviceId: getDeviceId(deviceModal.initial),
        name: form.name.trim(),
        ip: form.ip.trim(),
      })
      toast('设备保存成功')
    } else {
      await api.addStunDevice({ name: form.name.trim(), ip: form.ip.trim() })
      toast('设备添加成功')
    }
    setDeviceModal({ open: false })
    await refresh()
  }

  // 网段列表先取好再开弹窗——这一步只读本机网卡，不发包，几毫秒的事，
  // 放进弹窗里反而要先闪一个空下拉框
  const openScan = async () => {
    setScanOpening(true)
    try {
      setScanModal({ open: true, subnets: await api.listLanSubnets() })
    } catch (e) {
      toast('读不到本机网段: ' + (e instanceof Error ? e.message : ''))
    } finally {
      setScanOpening(false)
    }
  }

  // 不关扫描弹窗：一次扫描常常要加好几台，关掉就得再等一轮。
  // 添加窗盖在上面，存完退回来，刚加的那台会变成「已加过」
  const pickScanned = (h: LanHost) => {
    setDeviceModal({ open: true, prefill: { name: h.name, ip: h.ip } })
  }

  // 扫描结果里点一个端口：名字和内网端口都填好，剩下的按默认来，直接能存
  const pickScannedPort = (dev: StunDevice, p: LanPort) => {
    setServiceModal({
      open: true,
      deviceId: getDeviceId(dev),
      prefill: { name: p.name || `端口 ${p.port}`, internalPort: String(p.port) },
    })
  }

  const removeDevice = async (d: StunDevice) => {
    if (!window.confirm(`确认删除设备「${d.name}」？该设备下所有服务也将被删除`)) return
    try {
      await api.deleteStunDevice(getDeviceId(d))
      toast('设备删除成功')
      await refresh()
    } catch (e) {
      toast('删除失败: ' + (e instanceof Error ? e.message : ''))
    }
  }

  const submitService = async (form: ServiceFormState) => {
    const payload: api.StunServicePayload = {
      deviceId: serviceModal.deviceId,
      name: form.name.trim(),
      internalPort: Number(form.internalPort),
      protocol: form.protocol,
      upnpMappedPort: Number(form.upnpMappedPort) || 0,
      useUpnp: form.useUpnp,
      https: form.https,
      domain: form.domain.trim().toLowerCase(),
      tlsTerminate: form.tlsTerminate,
      certId: Number(form.certId) || 0,
      backendHttps: form.backendHttps,
      enabled: form.enabled,
      description: form.description.trim(),
      webhookconfig: {
        ...form.webhookconfig,
        url: form.webhookconfig.url.trim(),
        proxy: form.webhookconfig.proxy.trim(),
      },
      redirect: {
        ...form.redirect,
        entryHost: form.redirect.entryHost.trim().toLowerCase(),
        zoneDomain: form.redirect.zoneDomain.trim().toLowerCase(),
      },
    }
    let serviceId: number | undefined
    if (serviceModal.initial) {
      await api.updateStunService({ ...payload, serviceId: serviceModal.initial.id })
      serviceId = serviceModal.initial.id
      toast('服务保存成功')
    } else {
      const res = await api.addStunService(payload)
      serviceId = res?.id
      toast('服务添加成功')
    }
    if (serviceId) {
      const key = `${serviceModal.deviceId}-${serviceId}`
      const wasShown = homeSet.has(key)
      if (form.showOnHome !== wasShown) {
        try {
          await api.setStunShowOnHome(serviceModal.deviceId, serviceId, form.showOnHome)
        } catch (e) {
          toast('主页同步失败: ' + (e instanceof Error ? e.message : ''))
        }
      }
    }
    setServiceModal({ open: false, deviceId: 0 })
    await refresh()
  }

  /**
   * 照着现有服务再开一个，只改要改的那几项。
   *
   * 有三样不能照抄：
   * 入口域名两个服务共用一个，后同步的那个会把先前那条 Cloudflare 规则顶掉，
   * 而且两边都不报错；UPnP 固定端口抄过来就是两个服务抢同一个外部端口；
   * 名字一样的话列表上根本分不出谁是谁。
   */
  const copyService = (deviceId: number, svc: StunService) => {
    const src = serviceToForm(svc, false)
    setServiceModal({
      open: true,
      deviceId,
      prefill: {
        ...src,
        name: `${svc.name || '未命名服务'} 副本`,
        upnpMappedPort: '0',
        redirect: { ...src.redirect, enabled: false, entryHost: '' },
      },
    })
  }

  /**
   * 列表上直接启停，不用为了勾一个框把整个服务表单打开一遍。
   *
   * 后端要停掉再重开打洞，这一下要好几秒。这几秒里按钮上什么都不变的话，
   * 用户以为没点着，再点一下就又切回去了——所以切换中先把按钮锁上。
   */
  const toggleServiceEnabled = async (deviceId: number, svc: StunService) => {
    const next = svc.enabled === false
    setToggling((m) => ({ ...m, [`${deviceId}-${svc.id}`]: true }))
    try {
      await api.updateStunService({
        ...serviceToPayload(deviceId, svc),
        enabled: next,
        serviceId: svc.id,
      })
      toast(next ? `已启用「${svc.name}」` : `已停用「${svc.name}」`)
      await refresh()
    } catch (e) {
      toast('切换失败: ' + (e instanceof Error ? e.message : ''))
    } finally {
      setToggling((m) => {
        const n = { ...m }
        delete n[`${deviceId}-${svc.id}`]
        return n
      })
    }
  }

  const removeService = async (deviceId: number, svc: StunService) => {
    if (!window.confirm(`确认删除服务「${svc.name}」？`)) return
    try {
      await api.deleteStunService(deviceId, svc.id)
      toast('服务删除成功')
      await refresh()
    } catch (e) {
      toast('删除失败: ' + (e instanceof Error ? e.message : ''))
    }
  }

  const toggleHome = async (deviceId: number, serviceId: number) => {
    const key = `${deviceId}-${serviceId}`
    const current = homeSet.has(key)
    try {
      await api.setStunShowOnHome(deviceId, serviceId, !current)
      toast(current ? '已从主页移除' : '已添加到主页')
      await refresh()
    } catch (e) {
      toast('操作失败: ' + (e instanceof Error ? e.message : ''))
    }
  }

  const copy = (text: string) => {
    navigator.clipboard
      .writeText(text)
      .then(() => toast(`已复制: ${text}`))
      .catch(() => toast('复制失败'))
  }

  const openAddress = (svc: StunService, addr: string) => {
    if (!addr) return
    if ((svc.protocol || '').toLowerCase() === 'ssh') {
      const [host, port] = addr.split(':')
      const cmd = `ssh root@${host} -p ${port || '22'}`
      copy(cmd)
      window.setTimeout(() => alert(`SSH 连接命令已复制：\n\n${cmd}`), 100)
      return
    }
    // 和后端 PublicScheme 保持一致：洞口终结 TLS 时外部就是 https；
    // 不终结却勾了「转发给内网时也用 HTTPS」时洞口在替内网加密，外面那一段是明文，
    // 写成 https:// 会直接给出一个 ERR_SSL_PROTOCOL_ERROR 的链接
    const scheme =
      svc.tlsTerminate || (!svc.backendHttps && svc.https) ? 'https://' : 'http://'
    window.open(`${scheme}${addr}`, '_blank', 'noopener,noreferrer')
  }

  const logStatus = useMemo(() => (logKey ? statusMap[logKey] ?? null : null), [logKey, statusMap])

  if (loadErr && !config) {
    return (
      <Card>
        <div className="py-12 text-center text-rose-500">
          <AlertCircle className="mx-auto h-8 w-8" />
          <div className="mt-2 text-sm font-semibold">数据加载失败</div>
          <div className="mt-1 text-xs text-slate-500">{loadErr}</div>
          <button
            onClick={refresh}
            className="mt-4 rounded-xl bg-blue-500 px-4 py-2 text-xs font-semibold text-white"
          >
            重试
          </button>
        </div>
      </Card>
    )
  }

  if (!config) {
    return <Card><div className="py-12 text-center text-sm text-slate-400">正在加载...</div></Card>
  }

  return (
    <div className="space-y-4">
      {/* 网络拓扑 */}
      <Card>
        <CardHeader
          title="基础网络与路由拓扑"
          className="flex-wrap gap-2"
          action={
            <div className="flex max-w-full flex-wrap items-center justify-end gap-2 text-xs">
              <span className="text-slate-400">最优 STUN:</span>
              <span className="max-w-[220px] break-all text-right font-mono font-semibold text-blue-500 sm:max-w-none">
                {config.bestStun || '--'}
              </span>
              <button
                type="button"
                onClick={() => setNatTypeOpen(true)}
                title="查看 NAT 类型详情"
                className="flex items-center gap-1 rounded-md border border-slate-200 px-2 py-1 text-slate-600 transition hover:border-blue-200 hover:bg-blue-50 hover:text-blue-600"
              >
                <Info className="h-3.5 w-3.5" /> NAT 类型
              </button>
              <button
                onClick={refresh}
                className="ml-2 flex items-center gap-1 rounded-md text-slate-500 transition hover:text-blue-500"
              >
                <RotateCw className="h-3.5 w-3.5" /> 刷新
              </button>
            </div>
          }
        />
        <div className="flex flex-wrap items-center gap-2 rounded-xl bg-slate-50 p-4">
          <div className="flex min-w-[140px] flex-col items-center gap-1 rounded-xl bg-sky-50 px-4 py-3 ring-1 ring-sky-200">
            <div className="flex items-center gap-1.5">
              <Computer className="h-4 w-4 text-sky-500" />
              <span className="text-xs font-semibold text-sky-600">本机 IP</span>
            </div>
            <span className="font-mono text-xs text-slate-600">{config.localIP || '--'}</span>
          </div>
          {[...(config.natRouterList || [])]
            .sort((a, b) => a.natLevel - b.natLevel)
            .map((r) => (
              <div key={r.natLevel} className="flex items-center gap-2">
                <ChevronRight className="h-4 w-4 text-slate-300" />
                <div
                  className={`flex min-w-[140px] flex-col items-center gap-1 rounded-xl px-4 py-3 ring-1 ${
                    r.ipType === 'cgn' ? 'bg-violet-50 ring-violet-200' : 'bg-amber-50 ring-amber-200'
                  }`}
                >
                  <div className="flex items-center gap-1.5">
                    <Wifi className={`h-4 w-4 ${r.ipType === 'cgn' ? 'text-violet-500' : 'text-amber-500'}`} />
                    <span
                      className={`text-xs font-semibold ${
                        r.ipType === 'cgn' ? 'text-violet-600' : 'text-amber-600'
                      }`}
                    >
                      {r.ipType === 'cgn' ? 'CGN 网关' : `路由 NAT ${r.natLevel}`}
                    </span>
                  </div>
                  <span className="font-mono text-xs text-slate-600">{r.lanIP}</span>
                </div>
              </div>
            ))}
          <ChevronRight className="h-4 w-4 text-slate-300" />
          <div className="flex min-w-[140px] flex-col items-center gap-1 rounded-xl bg-emerald-50 px-4 py-3 ring-1 ring-emerald-200">
            <div className="flex items-center gap-1.5">
              <Globe className="h-4 w-4 text-emerald-500" />
              <span className="text-xs font-semibold text-emerald-600">公网 IP</span>
            </div>
            <span className="font-mono text-xs text-slate-600">{config.publicIP || '--'}</span>
          </div>
        </div>
      </Card>

      {/* 设备 + 服务 */}
      <div className="grid grid-cols-1 gap-4 lg:grid-cols-12">
        <Card className="lg:col-span-3">
          <CardHeader
            title="设备列表"
            action={
              <div className="flex items-center gap-1">
                {/* 扫描摆在前面：不用去路由器后台翻 IP，是更省事的那条路 */}
                <button
                  onClick={openScan}
                  disabled={scanOpening}
                  className="flex items-center gap-1 rounded-md bg-blue-500 px-2 py-1 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 hover:bg-blue-600 disabled:opacity-50"
                >
                  {scanOpening ? (
                    <LoaderCircle className="h-3 w-3 animate-spin" />
                  ) : (
                    <Radar className="h-3 w-3" />
                  )}
                  扫描
                </button>
                <button
                  onClick={() => setDeviceModal({ open: true })}
                  className="flex items-center gap-1 rounded-md bg-white px-2 py-1 text-xs font-semibold text-slate-600 ring-1 ring-slate-200 hover:bg-slate-50"
                >
                  <Plus className="h-3 w-3" /> 添加
                </button>
              </div>
            }
          />
          {!config.devices || config.devices.length === 0 ? (
            <div className="py-8 text-center text-xs text-slate-400">暂无设备接入</div>
          ) : (
            <ul className="space-y-1.5">
              {config.devices.map((d, idx) => {
                const active = idx === selectedIndex
                return (
                  <li
                    key={getDeviceId(d)}
                    onClick={() => setSelectedIndex(idx)}
                    className={`group cursor-pointer rounded-xl border px-3 py-2 transition ${
                      active
                        ? 'border-blue-200 bg-blue-50 text-blue-700'
                        : 'border-transparent hover:bg-slate-50'
                    }`}
                  >
                    <div className="flex items-center gap-2">
                      <Server className={`h-4 w-4 ${active ? 'text-blue-500' : 'text-slate-400'}`} />
                      <span className="flex-1 truncate text-sm font-semibold">{d.name || '未知设备'}</span>
                    </div>
                    <div className="ml-6 truncate font-mono text-xs text-slate-400">{d.ip || '--'}</div>
                    <div
                      className={`ml-6 mt-1.5 flex gap-1 transition ${
                        active ? 'opacity-100' : 'opacity-0 group-hover:opacity-100'
                      }`}
                    >
                      <button
                        onClick={(e) => {
                          e.stopPropagation()
                          setDeviceModal({ open: true, initial: d })
                        }}
                        className="rounded-md bg-white px-1.5 py-0.5 text-[10px] font-semibold text-emerald-600 ring-1 ring-emerald-200 hover:bg-emerald-50"
                      >
                        编辑
                      </button>
                      <button
                        onClick={(e) => {
                          e.stopPropagation()
                          removeDevice(d)
                        }}
                        className="rounded-md bg-white px-1.5 py-0.5 text-[10px] font-semibold text-rose-500 ring-1 ring-rose-200 hover:bg-rose-50"
                      >
                        删除
                      </button>
                    </div>
                  </li>
                )
              })}
            </ul>
          )}
        </Card>

        <div className="lg:col-span-9">
          <Card>
            <CardHeader
              title={device ? `${device.name} 的服务` : '服务列表'}
              action={
                device && (
                  <button
                    onClick={() =>
                      setServiceModal({ open: true, deviceId: getDeviceId(device) })
                    }
                    className="flex items-center gap-1 rounded-md bg-blue-500 px-3 py-1 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 hover:bg-blue-600"
                  >
                    <Plus className="h-3 w-3" /> 添加服务
                  </button>
                )
              }
            />
            {!device ? (
              <div className="py-12 text-center text-sm text-slate-400">请从左侧选择设备</div>
            ) : !device.services || device.services.length === 0 ? (
              <div className="py-12 text-center text-sm text-slate-400">
                该设备暂无服务配置，点击右上角"添加服务"开始配置
              </div>
            ) : (
              <div className="grid grid-cols-1 gap-3 md:grid-cols-2 xl:grid-cols-3">
                {device.services.map((svc) => {
                  const key = `${getDeviceId(device)}-${svc.id}`
                  const status = statusMap[key]
                  const phase = status?.phaseStr || 'STOPPED'
                  const running = phase === 'RUNNING'
                  const externalPort = status?.externalPort ?? 0
                  // 配了对外域名就用域名——终结 TLS 时证书是按域名签的，用 IP 访问必然报证书错
                  const externalHost = svc.domain || config.publicIP
                  const fullAddr = externalPort > 0 && externalHost ? `${externalHost}:${externalPort}` : ''
                  const inHome = homeSet.has(key)
                  const lastError = status?.lastError || ''
                  const restartCount = status?.restartCount ?? 0
                  const webhookEnabled = !!svc.webhookconfig?.enabled
                  const webhookStatus = webhookEnabled ? getWebhookStatus(status) : null
                  // 停用的服务本来就不该跑，别拿「❌ 已停止」去吓人
                  const off = svc.enabled === false
                  const busy = !!toggling[`${getDeviceId(device)}-${svc.id}`]
                  return (
                    <div
                      key={svc.id}
                      className={`flex flex-col rounded-xl border p-3 transition hover:border-blue-200 hover:shadow-md ${
                        off ? 'border-slate-200/80 bg-slate-50/60' : 'border-slate-200/80'
                      }`}
                    >
                      <div className="mb-2 flex items-center gap-1.5 border-b border-slate-100 pb-2">
                        <span
                          className={`h-2 w-2 shrink-0 rounded-full ${
                            off
                              ? 'bg-slate-300'
                              : running
                                ? 'bg-emerald-500 shadow-[0_0_6px_rgba(16,185,129,0.6)]'
                                : 'bg-rose-400 shadow-[0_0_6px_rgba(248,113,113,0.5)]'
                          }`}
                        />
                        <span
                          className={`mr-0.5 flex-1 truncate text-sm font-bold ${
                            off ? 'text-slate-400' : 'text-slate-800'
                          }`}
                        >
                          {svc.name || '未命名服务'}
                        </span>
                        <button
                          onClick={() => toggleServiceEnabled(getDeviceId(device), svc)}
                          disabled={busy}
                          title={busy ? '正在切换，打洞要几秒' : off ? '点一下启用' : '点一下停用'}
                          className={`grid h-6 w-6 place-items-center rounded-md transition disabled:cursor-wait ${
                            off
                              ? 'bg-slate-200 text-slate-500 hover:bg-slate-300'
                              : 'bg-emerald-50 text-emerald-600 hover:bg-emerald-100'
                          }`}
                        >
                          {busy ? (
                            <LoaderCircle className="h-3 w-3 animate-spin" />
                          ) : (
                            <Power className="h-3 w-3" />
                          )}
                        </button>
                        <button
                          onClick={() => toggleHome(getDeviceId(device), svc.id)}
                          title={inHome ? '从主页移除' : '添加到主页'}
                          className={`grid h-6 w-6 place-items-center rounded-md transition ${
                            inHome ? 'bg-blue-100 text-blue-600' : 'bg-slate-100 text-slate-500 hover:bg-slate-200'
                          }`}
                        >
                          <HomeIcon className="h-3 w-3" />
                        </button>
                        <button
                          onClick={() => setServiceModal({ open: true, deviceId: getDeviceId(device), initial: svc })}
                          className="grid h-6 w-6 place-items-center rounded-md bg-slate-100 text-slate-600 transition hover:bg-slate-200"
                          title="编辑"
                        >
                          <Pencil className="h-3 w-3" />
                        </button>
                        <button
                          onClick={() => copyService(getDeviceId(device), svc)}
                          className="grid h-6 w-6 place-items-center rounded-md bg-slate-100 text-slate-600 transition hover:bg-slate-200"
                          title="照着这个再开一个"
                        >
                          <Copy className="h-3 w-3" />
                        </button>
                        <button
                          onClick={() => removeService(getDeviceId(device), svc)}
                          className="grid h-6 w-6 place-items-center rounded-md bg-rose-50 text-rose-500 transition hover:bg-rose-100"
                          title="删除"
                        >
                          <Trash2 className="h-3 w-3" />
                        </button>
                      </div>

                      <div className="space-y-1 text-xs">
                        <div className="flex items-center justify-between">
                          <span className="text-slate-500">协议类型</span>
                          <span className="flex items-center gap-1">
                            <span className="rounded-md bg-blue-50 px-1.5 py-0.5 text-[10px] font-bold text-blue-600">
                              {(svc.protocol || 'TCP').toUpperCase()}
                            </span>
                            {svc.useUpnp && (
                              <span className="rounded-md bg-amber-50 px-1.5 py-0.5 text-[10px] font-bold text-amber-600">
                                UPnP
                              </span>
                            )}
                            {svc.https && (
                              <span
                                className="rounded-md bg-emerald-50 px-1.5 py-0.5 text-[10px] font-bold text-emerald-600"
                                title="链接按 https:// 展示（不影响转发）"
                              >
                                HTTPS
                              </span>
                            )}
                            {svc.backendHttps && (
                              <span
                                className="rounded-md bg-teal-50 px-1.5 py-0.5 text-[10px] font-bold text-teal-600"
                                title="洞口终结 TLS 后，再用 HTTPS 转发给内网（proxy_pass https://）"
                              >
                                内网 TLS
                              </span>
                            )}
                            {svc.tlsTerminate && (
                              <span
                                className="rounded-md bg-violet-50 px-1.5 py-0.5 text-[10px] font-bold text-violet-600"
                                title="LinkStar 在洞口终结 TLS"
                              >
                                TLS 终结
                              </span>
                            )}
                          </span>
                        </div>
                        {svc.domain && (
                          <div className="flex items-center justify-between gap-2">
                            <span className="shrink-0 text-slate-500">对外域名</span>
                            <span className="truncate font-mono text-slate-700" title={svc.domain}>
                              {svc.domain}
                            </span>
                          </div>
                        )}
                        <div className="flex items-center justify-between">
                          <span className="text-slate-500">内部端口</span>
                          <span className="font-mono font-semibold text-slate-700">{svc.internalPort}</span>
                        </div>
                        <div className="flex items-center justify-between">
                          <span className="text-slate-500">穿透状态</span>
                          <span
                            className={`font-semibold ${
                              off ? 'text-slate-400' : running ? 'text-emerald-600' : 'text-rose-500'
                            }`}
                          >
                            {off ? '已停用' : running ? '✅ 穿透成功' : `❌ ${phaseLabel[phase] || phase}`}
                          </span>
                        </div>
                        <div className="flex items-center justify-between gap-2">
                          <span className="text-slate-500">Webhook</span>
                          {webhookEnabled && webhookStatus ? (
                            <span
                              className={`truncate text-right font-semibold ${
                                webhookStatus.state === 'success'
                                  ? 'text-emerald-600'
                                  : webhookStatus.state === 'failed'
                                    ? 'text-rose-500'
                                    : 'text-amber-600'
                              }`}
                              title={webhookStatus.at ? new Date(webhookStatus.at).toLocaleString() : ''}
                            >
                              {webhookStatus.state === 'success' ? '✅ ' : webhookStatus.state === 'failed' ? '❌ ' : '⏳ '}
                              {webhookStatus.text}
                            </span>
                          ) : (
                            <span className="font-semibold text-slate-400">未启用</span>
                          )}
                        </div>
                      </div>

                      {/* 停用之前那次的报错就别留着了，用户是自己关的，不是出了毛病 */}
                      {!off && lastError && (
                        <div className="mt-2 rounded-lg border border-rose-200 bg-rose-50 px-2 py-1.5 text-[11px] text-rose-600">
                          ⚠️ <span className="font-semibold">错误:</span> {lastError}
                          {restartCount > 0 && ` (重启 ${restartCount} 次)`}
                        </div>
                      )}

                      {/*
                        停用之后洞就关了，上一轮那个地址已经连不上。
                        还摆着「一键直达」的话，用户点进去打不开，只会以为是穿透坏了
                      */}
                      {off ? (
                        <div className="mt-2 rounded-lg bg-slate-100 px-2 py-2 text-center text-xs text-slate-400">
                          已停用，外部地址不通
                        </div>
                      ) : fullAddr ? (
                        <>
                          <div className="mt-2 text-[11px] text-slate-400">外部连接地址</div>
                          <div className="mt-1 break-all rounded-lg border border-dashed border-slate-300 bg-slate-50 px-2 py-1.5 text-center font-mono text-sm font-semibold text-blue-600">
                            {fullAddr}
                          </div>
                          <div className="mt-2 flex gap-1.5">
                            <button
                              onClick={() => copy(fullAddr)}
                              className="flex flex-1 items-center justify-center gap-1 rounded-lg border border-slate-200 bg-white px-2 py-1.5 text-xs font-semibold text-slate-600 transition hover:bg-slate-50"
                            >
                              <ClipboardCopy className="h-3 w-3" /> 复制地址
                            </button>
                            <button
                              onClick={() => openAddress(svc, fullAddr)}
                              className="flex flex-1 items-center justify-center gap-1 rounded-lg border border-blue-200 bg-blue-50 px-2 py-1.5 text-xs font-semibold text-blue-600 transition hover:bg-blue-100"
                            >
                              <ExternalLink className="h-3 w-3" /> 一键直达
                            </button>
                          </div>
                        </>
                      ) : (
                        <div className="mt-2 rounded-lg bg-slate-100 px-2 py-2 text-center text-xs text-slate-400">
                          尚未获取外部地址
                        </div>
                      )}

                      <button
                        onClick={() => setLogKey(key)}
                        className="mt-2 flex items-center justify-center gap-1 rounded-lg border border-slate-200 px-2 py-1.5 text-xs text-slate-600 transition hover:bg-slate-50"
                      >
                        <FileText className="h-3 w-3" /> 查看日志 ({(status?.logs ?? []).length})
                      </button>
                    </div>
                  )
                })}
              </div>
            )}
          </Card>
        </div>
      </div>

      {/* 弹窗 */}
      {/* 扫描窗排在前面，添加窗才能盖在它上面 */}
      {scanModal.open && (
        <LanScanModal
          subnets={scanModal.subnets}
          devices={config.devices ?? []}
          onCancel={() => setScanModal({ open: false, subnets: [] })}
          onPick={pickScanned}
          onPickPort={pickScannedPort}
        />
      )}
      {deviceModal.open && (
        <DeviceModal
          initial={deviceModal.initial}
          prefill={deviceModal.prefill}
          onCancel={() => setDeviceModal({ open: false })}
          onSubmit={submitDevice}
        />
      )}
      {serviceModal.open && (
        <ServiceModal
          deviceId={serviceModal.deviceId}
          initial={serviceModal.initial}
          prefill={serviceModal.prefill}
          initialShowOnHome={
            serviceModal.initial
              ? homeSet.has(`${serviceModal.deviceId}-${serviceModal.initial.id}`)
              : false
          }
          status={
            serviceModal.initial
              ? statusMap[`${serviceModal.deviceId}-${serviceModal.initial.id}`]
              : undefined
          }
          onCancel={() => setServiceModal({ open: false, deviceId: 0 })}
          onSubmit={submitService}
          onToast={toast}
        />
      )}
      {logKey && <LogModal status={logStatus} onClose={() => setLogKey(null)} />}
      {natTypeOpen && <NatTypeModal onClose={() => setNatTypeOpen(false)} />}

      {/* Toasts */}
      <div className="pointer-events-none fixed bottom-6 left-1/2 z-[60] flex -translate-x-1/2 flex-col items-center gap-2">
        {toasts.map((t) => (
          <div
            key={t.id}
            // 限宽换行：长的那几句不限宽会拉成一条横穿屏幕的线，反而看不下去
            className="max-w-[min(90vw,34rem)] break-words rounded-lg bg-slate-900/85 px-4 py-2 text-sm leading-relaxed text-white shadow-lg"
          >
            {t.text}
          </div>
        ))}
      </div>
    </div>
  )
}
