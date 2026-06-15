import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertCircle,
  CircleCheck,
  Cloud,
  Globe,
  Pencil,
  Plus,
  Power,
  RefreshCw,
  Search,
  Server,
  Trash2,
  TriangleAlert,
  X,
  type LucideIcon,
} from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import * as api from '../lib/api'
import type {
  DdnsConfig,
  DdnsProvider,
  DdnsRecord,
  DdnsRecordStatus,
} from '../types'

const statusInfo: Record<DdnsRecordStatus, { label: string; tone: string; icon: LucideIcon }> = {
  success: { label: '正常', tone: 'bg-emerald-50 text-emerald-600', icon: CircleCheck },
  pending: { label: '等待同步', tone: 'bg-amber-50 text-amber-600', icon: RefreshCw },
  skipped: { label: '无变化', tone: 'bg-slate-100 text-slate-500', icon: CircleCheck },
  failed: { label: '同步失败', tone: 'bg-rose-50 text-rose-500', icon: TriangleAlert },
}

const ipSourceLabel: Record<string, string> = {
  stun: 'STUN 公网',
  web: 'Web 查询',
  dns: 'DNS 解析',
  interface: '本地网卡',
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
    window.setTimeout(() => setToasts((p) => p.filter((t) => t.id !== id)), 2200)
  }, [])
  return { toasts, show }
}

function fmtTime(s?: string): string {
  if (!s) return '--'
  const d = new Date(s)
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return '--'
  return d.toLocaleString()
}

// ===================== 服务商弹窗 =====================

interface ProviderFormState {
  name: string
  type: 'cloudflare'
  apiToken: string
}

function ProviderModal({
  initial,
  onCancel,
  onSubmit,
}: {
  initial?: DdnsProvider
  onCancel: () => void
  onSubmit: (form: ProviderFormState) => Promise<void>
}) {
  const [form, setForm] = useState<ProviderFormState>(() => ({
    name: initial?.name ?? '',
    type: 'cloudflare',
    apiToken: '',
  }))
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const submit = async () => {
    if (!form.name.trim()) {
      setErr('请填写服务商名称')
      return
    }
    if (!initial && !form.apiToken.trim()) {
      setErr('请填写 API Token')
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
      className="fixed inset-0 z-50 grid place-items-center bg-black/30 px-4 py-6 backdrop-blur-sm"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="w-full max-w-md rounded-2xl bg-white p-6 text-slate-700 shadow-2xl ring-1 ring-slate-200">
        <div className="mb-5 flex items-center justify-between">
          <div className="text-base font-bold text-slate-800">{initial ? '编辑服务商' : '添加服务商'}</div>
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
            <div className="mb-1 text-xs font-semibold text-slate-500">服务商名称</div>
            <input
              autoFocus
              value={form.name}
              onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
              placeholder="如 我的 Cloudflare"
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-slate-500">服务商类型</div>
            <select
              value={form.type}
              onChange={(e) => setForm((p) => ({ ...p, type: e.target.value as 'cloudflare' }))}
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
            >
              <option value="cloudflare">Cloudflare</option>
            </select>
          </label>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-slate-500">
              API Token{initial && <span className="ml-1 font-normal text-slate-400">（留空表示不修改）</span>}
            </div>
            <input
              type="password"
              value={form.apiToken}
              onChange={(e) => setForm((p) => ({ ...p, apiToken: e.target.value }))}
              placeholder={initial ? '••••••••（已配置）' : '粘贴 Cloudflare API Token'}
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 font-mono text-sm outline-none focus:border-blue-400"
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

// ===================== 记录弹窗 =====================

interface RecordFormState {
  enabled: boolean
  providerId: string
  name: string
  domain: string
  subDomain: string
  recordType: 'A' | 'AAAA'
  ipSource: 'stun' | 'web' | 'dns' | 'interface'
  ipSourceArg: string
  ttl: string
  proxied: boolean
}

function RecordModal({
  initial,
  providers,
  onCancel,
  onSubmit,
}: {
  initial?: DdnsRecord
  providers: DdnsProvider[]
  onCancel: () => void
  onSubmit: (form: RecordFormState) => Promise<void>
}) {
  const [form, setForm] = useState<RecordFormState>(() => {
    if (!initial) {
      return {
        enabled: true,
        providerId: providers[0] ? String(providers[0].id) : '',
        name: '',
        domain: '',
        subDomain: '',
        recordType: 'A',
        ipSource: 'stun',
        ipSourceArg: '',
        ttl: '1',
        proxied: false,
      }
    }
    return {
      enabled: initial.enabled,
      providerId: String(initial.providerId),
      name: initial.name,
      domain: initial.domain,
      subDomain: initial.subDomain,
      recordType: initial.recordType,
      ipSource: initial.ipSource,
      ipSourceArg: initial.ipSourceArg,
      ttl: String(initial.ttl || 1),
      proxied: initial.proxied,
    }
  })
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const submit = async () => {
    if (!form.providerId) {
      setErr('请先选择服务商')
      return
    }
    if (!form.name.trim()) {
      setErr('请填写记录名称')
      return
    }
    if (!form.domain.trim()) {
      setErr('请填写主域名')
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

  const needArg = form.ipSource === 'web' || form.ipSource === 'dns'

  return (
    <div
      className="fixed inset-0 z-50 grid place-items-center bg-black/30 px-4 py-6 backdrop-blur-sm"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="w-full max-w-lg rounded-2xl bg-white p-6 text-slate-700 shadow-2xl ring-1 ring-slate-200">
        <div className="mb-5 flex items-center justify-between">
          <div className="text-base font-bold text-slate-800">{initial ? '编辑解析记录' : '添加解析记录'}</div>
          <button
            type="button"
            onClick={onCancel}
            className="grid h-7 w-7 place-items-center rounded-full text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="grid grid-cols-2 gap-3">
          <label className="col-span-2 block sm:col-span-1">
            <div className="mb-1 text-xs font-semibold text-slate-500">记录名称</div>
            <input
              autoFocus
              value={form.name}
              onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
              placeholder="如 家庭宽带"
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          <label className="col-span-2 block sm:col-span-1">
            <div className="mb-1 text-xs font-semibold text-slate-500">服务商</div>
            <select
              value={form.providerId}
              onChange={(e) => setForm((p) => ({ ...p, providerId: e.target.value }))}
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
            >
              {providers.length === 0 && <option value="">请先添加服务商</option>}
              {providers.map((p) => (
                <option key={p.id} value={String(p.id)}>
                  {p.name}
                </option>
              ))}
            </select>
          </label>
          <label className="col-span-2 block sm:col-span-1">
            <div className="mb-1 text-xs font-semibold text-slate-500">主域名</div>
            <input
              value={form.domain}
              onChange={(e) => setForm((p) => ({ ...p, domain: e.target.value }))}
              placeholder="如 example.com"
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          <label className="col-span-2 block sm:col-span-1">
            <div className="mb-1 text-xs font-semibold text-slate-500">子域名（@ 或留空表示主域名）</div>
            <input
              value={form.subDomain}
              onChange={(e) => setForm((p) => ({ ...p, subDomain: e.target.value }))}
              placeholder="如 home"
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          <label className="col-span-2 block sm:col-span-1">
            <div className="mb-1 text-xs font-semibold text-slate-500">记录类型</div>
            <select
              value={form.recordType}
              onChange={(e) => setForm((p) => ({ ...p, recordType: e.target.value as 'A' | 'AAAA' }))}
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
            >
              <option value="A">A (IPv4)</option>
              <option value="AAAA">AAAA (IPv6)</option>
            </select>
          </label>
          <label className="col-span-2 block sm:col-span-1">
            <div className="mb-1 text-xs font-semibold text-slate-500">TTL（1 = 自动）</div>
            <input
              type="number"
              value={form.ttl}
              onChange={(e) => setForm((p) => ({ ...p, ttl: e.target.value }))}
              min={1}
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
          <label className="col-span-2 block sm:col-span-1">
            <div className="mb-1 text-xs font-semibold text-slate-500">IP 来源</div>
            <select
              value={form.ipSource}
              onChange={(e) => setForm((p) => ({ ...p, ipSource: e.target.value as RecordFormState['ipSource'] }))}
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
            >
              <option value="stun">STUN 公网 IP</option>
              <option value="web">Web 查询</option>
              <option value="dns">DNS 解析</option>
              <option value="interface">本地网卡</option>
            </select>
          </label>
          <label className="col-span-2 block sm:col-span-1">
            <div className="mb-1 text-xs font-semibold text-slate-500">
              来源参数{needArg ? '' : '（当前来源无需填写）'}
            </div>
            <input
              value={form.ipSourceArg}
              onChange={(e) => setForm((p) => ({ ...p, ipSourceArg: e.target.value }))}
              disabled={!needArg}
              placeholder={form.ipSource === 'web' ? '如 https://ip.sb（留空用内置源）' : form.ipSource === 'dns' ? '如 example.com' : ''}
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400"
            />
          </label>

          <div className="col-span-2 grid grid-cols-2 gap-2">
            <label className="flex cursor-pointer items-center gap-2 rounded-xl bg-slate-50 px-3 py-2 text-xs font-medium text-slate-600">
              <input
                type="checkbox"
                checked={form.enabled}
                onChange={(e) => setForm((p) => ({ ...p, enabled: e.target.checked }))}
                className="h-3.5 w-3.5"
              />
              启用自动同步
            </label>
            <label className="flex cursor-pointer items-center gap-2 rounded-xl bg-slate-50 px-3 py-2 text-xs font-medium text-slate-600">
              <input
                type="checkbox"
                checked={form.proxied}
                onChange={(e) => setForm((p) => ({ ...p, proxied: e.target.checked }))}
                className="h-3.5 w-3.5"
              />
              Cloudflare 小云朵代理
            </label>
          </div>
          {err && <div className="col-span-2 text-xs text-rose-500">{err}</div>}
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

// ===================== 主页面 =====================

export function Ddns() {
  const [config, setConfig] = useState<DdnsConfig | null>(null)
  const [loadErr, setLoadErr] = useState('')
  const [query, setQuery] = useState('')
  const [filterStatus, setFilterStatus] = useState<'all' | DdnsRecordStatus>('all')
  const [syncingId, setSyncingId] = useState<number | null>(null)
  const { toasts, show: toast } = useToast()

  const [providerModal, setProviderModal] = useState<{ open: boolean; initial?: DdnsProvider }>({ open: false })
  const [recordModal, setRecordModal] = useState<{ open: boolean; initial?: DdnsRecord }>({ open: false })

  const refresh = useCallback(async () => {
    try {
      const cfg = await api.getDdnsConfig()
      setConfig(cfg)
      setLoadErr('')
    } catch (e) {
      setLoadErr(e instanceof Error ? e.message : String(e))
    }
  }, [])

  useEffect(() => {
    refresh()
    const t = window.setInterval(refresh, 15000)
    return () => window.clearInterval(t)
  }, [refresh])

  const providers = config?.providers ?? []
  const records = config?.records ?? []

  const providerName = useMemo(() => {
    const m = new Map<number, string>()
    for (const p of providers) m.set(p.id, p.name)
    return m
  }, [providers])

  const filtered = useMemo(() => {
    const q = query.trim().toLowerCase()
    return records.filter((r) => {
      if (filterStatus !== 'all' && r.lastStatus !== filterStatus) return false
      if (q) {
        const hay = `${r.name} ${r.subDomain}.${r.domain} ${r.lastIP}`.toLowerCase()
        if (!hay.includes(q)) return false
      }
      return true
    })
  }, [records, query, filterStatus])

  const total = records.length
  const success = records.filter((r) => r.lastStatus === 'success').length
  const failed = records.filter((r) => r.lastStatus === 'failed').length

  // ---- 全局设置 ----
  const toggleEnabled = async () => {
    if (!config) return
    try {
      await api.updateDdnsSettings({ enabled: !config.enabled, intervalSec: config.intervalSec })
      toast(config.enabled ? 'DDNS 已停用' : 'DDNS 已启用')
      await refresh()
    } catch (e) {
      toast('操作失败: ' + (e instanceof Error ? e.message : ''))
    }
  }

  const changeInterval = async (sec: number) => {
    if (!config || sec < 30) return
    try {
      await api.updateDdnsSettings({ enabled: config.enabled, intervalSec: sec })
      toast('同步间隔已更新')
      await refresh()
    } catch (e) {
      toast('操作失败: ' + (e instanceof Error ? e.message : ''))
    }
  }

  // ---- 服务商 ----
  const submitProvider = async (form: ProviderFormState) => {
    if (providerModal.initial) {
      await api.updateDdnsProvider({
        id: providerModal.initial.id,
        name: form.name.trim(),
        apiToken: form.apiToken.trim(),
      })
      toast('服务商已更新')
    } else {
      await api.addDdnsProvider({ name: form.name.trim(), type: form.type, apiToken: form.apiToken.trim() })
      toast('服务商已添加')
    }
    setProviderModal({ open: false })
    await refresh()
  }

  const removeProvider = async (p: DdnsProvider) => {
    if (!window.confirm(`确认删除服务商「${p.name}」？`)) return
    try {
      await api.deleteDdnsProvider(p.id)
      toast('服务商已删除')
      await refresh()
    } catch (e) {
      toast('删除失败: ' + (e instanceof Error ? e.message : ''))
    }
  }

  // ---- 记录 ----
  const submitRecord = async (form: RecordFormState) => {
    const payload: api.DdnsRecordPayload = {
      enabled: form.enabled,
      providerId: Number(form.providerId),
      name: form.name.trim(),
      domain: form.domain.trim(),
      subDomain: form.subDomain.trim(),
      recordType: form.recordType,
      ipSource: form.ipSource,
      ipSourceArg: form.ipSourceArg.trim(),
      ttl: Number(form.ttl) || 1,
      proxied: form.proxied,
    }
    if (recordModal.initial) {
      await api.updateDdnsRecord({ ...payload, id: recordModal.initial.id })
      toast('记录已更新')
    } else {
      await api.addDdnsRecord(payload)
      toast('记录已添加')
    }
    setRecordModal({ open: false })
    await refresh()
  }

  const removeRecord = async (r: DdnsRecord) => {
    if (!window.confirm(`确认删除记录「${r.name}」？`)) return
    try {
      await api.deleteDdnsRecord(r.id)
      toast('记录已删除')
      await refresh()
    } catch (e) {
      toast('删除失败: ' + (e instanceof Error ? e.message : ''))
    }
  }

  const syncRecord = async (r: DdnsRecord) => {
    setSyncingId(r.id)
    try {
      await api.syncDdnsRecord(r.id)
      toast('同步成功')
      await refresh()
    } catch (e) {
      toast('同步失败: ' + (e instanceof Error ? e.message : ''))
    } finally {
      setSyncingId(null)
    }
  }

  const toggleRecordEnabled = async (r: DdnsRecord) => {
    try {
      await api.updateDdnsRecord({
        id: r.id,
        enabled: !r.enabled,
        providerId: r.providerId,
        name: r.name,
        domain: r.domain,
        subDomain: r.subDomain,
        recordType: r.recordType,
        ipSource: r.ipSource,
        ipSourceArg: r.ipSourceArg,
        ttl: r.ttl,
        proxied: r.proxied,
      })
      toast(r.enabled ? '已关闭自动同步' : '已开启自动同步')
      await refresh()
    } catch (e) {
      toast('操作失败: ' + (e instanceof Error ? e.message : ''))
    }
  }

  if (loadErr && !config) {
    return (
      <Card>
        <div className="py-12 text-center text-rose-500">
          <AlertCircle className="mx-auto h-8 w-8" />
          <div className="mt-2 text-sm font-semibold">数据加载失败</div>
          <div className="mt-1 text-xs text-slate-500">{loadErr}</div>
          <button onClick={refresh} className="mt-4 rounded-xl bg-blue-500 px-4 py-2 text-xs font-semibold text-white">
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
      {/* 顶部概览 */}
      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Card>
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-blue-50 text-blue-500">
              <Globe className="h-5 w-5" />
            </div>
            <div>
              <div className="text-xs text-slate-500">解析记录</div>
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
              <div className="text-2xl font-bold text-slate-800">{success}</div>
            </div>
          </div>
        </Card>
        <Card>
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-rose-50 text-rose-500">
              <TriangleAlert className="h-5 w-5" />
            </div>
            <div>
              <div className="text-xs text-slate-500">异常记录</div>
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
              <div className="text-2xl font-bold text-slate-800">{providers.length}</div>
            </div>
          </div>
        </Card>
      </div>

      {/* 全局设置 */}
      <Card>
        <div className="flex flex-wrap items-center gap-4">
          <div className="flex items-center gap-3">
            <button
              onClick={toggleEnabled}
              className={`flex items-center gap-1.5 rounded-xl px-3 py-2 text-sm font-semibold transition ${
                config.enabled
                  ? 'bg-emerald-50 text-emerald-600 ring-1 ring-emerald-200 hover:bg-emerald-100'
                  : 'bg-slate-100 text-slate-500 ring-1 ring-slate-200 hover:bg-slate-200'
              }`}
            >
              <Power className="h-4 w-4" />
              {config.enabled ? 'DDNS 已启用' : 'DDNS 已停用'}
            </button>
            <span className="text-xs text-slate-400">
              {config.enabled ? '调度器运行中，按间隔自动同步' : '已关闭，记录不会自动更新'}
            </span>
          </div>
          <div className="ml-auto flex items-center gap-2 text-sm">
            <span className="text-xs font-semibold text-slate-500">全局同步间隔</span>
            <select
              value={config.intervalSec}
              onChange={(e) => changeInterval(Number(e.target.value))}
              className="rounded-xl border border-slate-200 bg-white px-3 py-1.5 text-sm outline-none focus:border-blue-400"
            >
              <option value={60}>1 分钟</option>
              <option value={300}>5 分钟</option>
              <option value={600}>10 分钟</option>
              <option value={1800}>30 分钟</option>
              <option value={3600}>1 小时</option>
            </select>
          </div>
        </div>
      </Card>

      {/* 服务商 */}
      <Card>
        <CardHeader
          title="DNS 服务商"
          action={
            <button
              onClick={() => setProviderModal({ open: true })}
              className="flex items-center gap-1 rounded-md bg-blue-500 px-2.5 py-1 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 hover:bg-blue-600"
            >
              <Plus className="h-3 w-3" /> 添加凭证
            </button>
          }
        />
        {providers.length === 0 ? (
          <div className="py-6 text-center text-xs text-slate-400">尚未配置服务商，先添加一个 Cloudflare 凭证</div>
        ) : (
          <div className="flex flex-wrap gap-2">
            {providers.map((p) => (
              <div
                key={p.id}
                className="group flex items-center gap-2 rounded-xl bg-orange-50 px-3 py-2 text-xs font-semibold text-orange-600 ring-1 ring-orange-200"
              >
                <Server className="h-3.5 w-3.5" />
                {p.name}
                <span className="text-[10px] font-normal text-orange-400">({p.type})</span>
                <button
                  onClick={() => setProviderModal({ open: true, initial: p })}
                  className="ml-1 grid h-5 w-5 place-items-center rounded-md text-orange-400 transition hover:bg-white hover:text-emerald-600"
                  title="编辑"
                >
                  <Pencil className="h-3 w-3" />
                </button>
                <button
                  onClick={() => removeProvider(p)}
                  className="grid h-5 w-5 place-items-center rounded-md text-orange-400 transition hover:bg-white hover:text-rose-500"
                  title="删除"
                >
                  <Trash2 className="h-3 w-3" />
                </button>
              </div>
            ))}
          </div>
        )}
      </Card>

      {/* 记录列表 */}
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
              setFilterStatus((p) =>
                p === 'all' ? 'success' : p === 'success' ? 'pending' : p === 'pending' ? 'failed' : 'all',
              )
            }
          >
            状态: {filterStatus === 'all' ? '全部' : statusInfo[filterStatus].label}
          </button>
          <button
            onClick={() => {
              if (providers.length === 0) {
                toast('请先添加 DNS 服务商')
                return
              }
              setRecordModal({ open: true })
            }}
            className="flex items-center gap-1 rounded-md bg-blue-500 px-3 py-1.5 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 hover:bg-blue-600"
          >
            <Plus className="h-3 w-3" /> 添加域名
          </button>
        </div>

        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-slate-50/60 text-xs text-slate-500">
                <th className="px-5 py-2.5 text-left font-medium">域名</th>
                <th className="px-3 py-2.5 text-left font-medium">类型</th>
                <th className="px-3 py-2.5 text-left font-medium">当前 IP</th>
                <th className="px-3 py-2.5 text-left font-medium">IP 来源</th>
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
                    {records.length === 0 ? '暂无解析记录，点击右上角"添加域名"开始' : '没有匹配的记录'}
                  </td>
                </tr>
              )}
              {filtered.map((r) => {
                const s = statusInfo[r.lastStatus] ?? statusInfo.pending
                const Icon = s.icon
                const fqdn = !r.subDomain || r.subDomain === '@' ? r.domain : `${r.subDomain}.${r.domain}`
                const busy = syncingId === r.id
                return (
                  <tr key={r.id} className="text-slate-700 transition hover:bg-slate-50/60">
                    <td className="px-5 py-3">
                      <div className="font-semibold">{fqdn}</div>
                      <div className="text-[11px] text-slate-400">{r.name}</div>
                    </td>
                    <td className="px-3 py-3">
                      <span className="rounded-md bg-blue-50 px-1.5 py-0.5 text-[11px] font-bold text-blue-600">
                        {r.recordType}
                      </span>
                    </td>
                    <td className="px-3 py-3 font-mono text-xs">{r.lastIP || '-'}</td>
                    <td className="px-3 py-3 text-xs text-slate-500">{ipSourceLabel[r.ipSource] ?? r.ipSource}</td>
                    <td className="px-3 py-3 text-xs">{providerName.get(r.providerId) ?? '已删除'}</td>
                    <td className="px-3 py-3">
                      <span
                        className={`inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] font-semibold ${s.tone}`}
                        title={r.lastMessage}
                      >
                        <Icon className={`h-3 w-3 ${r.lastStatus === 'pending' ? 'animate-spin' : ''}`} />
                        {s.label}
                      </span>
                    </td>
                    <td className="px-3 py-3 text-xs text-slate-500">{fmtTime(r.lastSyncAt)}</td>
                    <td className="px-3 py-3">
                      <button onClick={() => toggleRecordEnabled(r)} className="flex items-center gap-1.5" title="切换自动同步">
                        <span className={`inline-block h-2 w-2 rounded-full ${r.enabled ? 'bg-emerald-500' : 'bg-slate-300'}`} />
                        <span className="text-xs text-slate-600">{r.enabled ? '已开启' : '已关闭'}</span>
                      </button>
                    </td>
                    <td className="px-5 py-3 text-right">
                      <div className="inline-flex items-center gap-1">
                        <button
                          onClick={() => syncRecord(r)}
                          disabled={busy}
                          className="grid h-6 w-6 place-items-center rounded-md text-slate-400 hover:bg-blue-50 hover:text-blue-600 disabled:opacity-40"
                          title="立即同步"
                        >
                          <RefreshCw className={`h-3 w-3 ${busy ? 'animate-spin' : ''}`} />
                        </button>
                        <button
                          onClick={() => setRecordModal({ open: true, initial: r })}
                          className="grid h-6 w-6 place-items-center rounded-md text-slate-400 hover:bg-emerald-50 hover:text-emerald-600"
                          title="编辑"
                        >
                          <Pencil className="h-3 w-3" />
                        </button>
                        <button
                          onClick={() => removeRecord(r)}
                          className="grid h-6 w-6 place-items-center rounded-md text-slate-400 hover:bg-rose-50 hover:text-rose-500"
                          title="删除"
                        >
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
          共 {filtered.length} 条记录
        </div>
      </Card>

      {/* 弹窗 */}
      {providerModal.open && (
        <ProviderModal
          initial={providerModal.initial}
          onCancel={() => setProviderModal({ open: false })}
          onSubmit={submitProvider}
        />
      )}
      {recordModal.open && (
        <RecordModal
          initial={recordModal.initial}
          providers={providers}
          onCancel={() => setRecordModal({ open: false })}
          onSubmit={submitRecord}
        />
      )}

      {/* Toasts */}
      <div className="pointer-events-none fixed bottom-6 left-1/2 z-[60] flex -translate-x-1/2 flex-col items-center gap-2">
        {toasts.map((t) => (
          <div key={t.id} className="rounded-lg bg-slate-900/85 px-4 py-2 text-sm text-white shadow-lg">
            {t.text}
          </div>
        ))}
      </div>
    </div>
  )
}
