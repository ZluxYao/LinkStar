import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertCircle,
  ChevronRight,
  ClipboardCopy,
  Computer,
  ExternalLink,
  FileText,
  Globe,
  Home as HomeIcon,
  Pencil,
  Plus,
  RotateCw,
  Server,
  Trash2,
  Wifi,
  X,
} from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import * as api from '../lib/api'
import type {
  StunConfig,
  StunDevice,
  StunService,
  StunStatusEvent,
} from '../types'

const phaseLabel: Record<string, string> = {
  PROBING: '探测中',
  RUNNING: '穿透成功',
  RESTARTING: '重启中',
  FAILED: '探测失败',
  STOPPED: '已停止',
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
    window.setTimeout(() => setToasts((p) => p.filter((t) => t.id !== id)), 2000)
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
  onCancel,
  onSubmit,
}: {
  initial?: StunDevice
  onCancel: () => void
  onSubmit: (form: DeviceFormState) => Promise<void>
}) {
  const [form, setForm] = useState<DeviceFormState>(() =>
    initial ? { name: initial.name, ip: initial.ip } : emptyDevice,
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
      className="fixed inset-0 z-50 grid place-items-center bg-black/30 px-4 py-6 backdrop-blur-sm"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="w-full max-w-md rounded-2xl bg-white p-6 text-slate-700 shadow-2xl ring-1 ring-slate-200">
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

interface ServiceFormState {
  name: string
  internalPort: string
  protocol: 'TCP' | 'UDP'
  upnpMappedPort: string
  useUpnp: boolean
  tls: boolean
  enabled: boolean
  showOnHome: boolean
  description: string
}
const emptyService: ServiceFormState = {
  name: '',
  internalPort: '',
  protocol: 'TCP',
  upnpMappedPort: '0',
  useUpnp: true,
  tls: false,
  enabled: true,
  showOnHome: false,
  description: '',
}

function ServiceModal({
  initial,
  initialShowOnHome,
  onCancel,
  onSubmit,
}: {
  initial?: StunService
  initialShowOnHome: boolean
  onCancel: () => void
  onSubmit: (form: ServiceFormState) => Promise<void>
}) {
  const [form, setForm] = useState<ServiceFormState>(() => {
    if (!initial) return emptyService
    return {
      name: initial.name,
      internalPort: String(initial.internalPort || ''),
      protocol: (initial.protocol as 'TCP' | 'UDP') || 'TCP',
      upnpMappedPort: String(initial.upnpMappedPort || 0),
      useUpnp: !!initial.useUpnp,
      tls: !!initial.tls,
      enabled: initial.enabled !== false,
      showOnHome: initialShowOnHome,
      description: initial.description || '',
    }
  })
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

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
      <div className="w-full max-w-lg rounded-2xl bg-white p-6 text-slate-700 shadow-2xl ring-1 ring-slate-200">
        <div className="mb-5 flex items-center justify-between">
          <div className="text-base font-bold text-slate-800">{initial ? '编辑服务' : '添加服务'}</div>
          <button
            type="button"
            onClick={onCancel}
            className="grid h-7 w-7 place-items-center rounded-full text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
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
              { key: 'useUpnp' as const, label: '启用 UPnP' },
              { key: 'tls' as const, label: '启用 TLS' },
              { key: 'enabled' as const, label: '启用服务' },
              { key: 'showOnHome' as const, label: '主页显示' },
            ].map((opt) => (
              <label
                key={opt.key}
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

          <label className="col-span-2 block">
            <div className="mb-1 text-xs font-semibold text-slate-500">描述（可选）</div>
            <input
              value={form.description}
              onChange={(e) => setForm((p) => ({ ...p, description: e.target.value }))}
              placeholder="服务描述信息"
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>
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
      className="fixed inset-0 z-50 grid place-items-center bg-black/30 px-4 py-6 backdrop-blur-sm"
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

export function Stun() {
  const [config, setConfig] = useState<StunConfig | null>(null)
  const [loadErr, setLoadErr] = useState('')
  const [statusMap, setStatusMap] = useState<Record<string, StunStatusEvent>>({})
  const [homeSet, setHomeSet] = useState<Set<string>>(new Set())
  const [selectedIndex, setSelectedIndex] = useState(0)
  const { toasts, show: toast } = useToast()

  const [deviceModal, setDeviceModal] = useState<{ open: boolean; initial?: StunDevice }>({ open: false })
  const [serviceModal, setServiceModal] = useState<{
    open: boolean
    deviceId: number
    initial?: StunService
  }>({ open: false, deviceId: 0 })
  const [logKey, setLogKey] = useState<string | null>(null)

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
    const es = new EventSource('/api/stun/status/events')
    es.onmessage = (e) => {
      try {
        const evt = JSON.parse(e.data) as StunStatusEvent
        setStatusMap((p) => ({ ...p, [evt.key]: evt }))
      } catch {
        // ignore
      }
    }
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
      tls: form.tls,
      enabled: form.enabled,
      description: form.description.trim(),
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

  const openAddress = (protocol: string, addr: string) => {
    if (!addr) return
    if ((protocol || '').toLowerCase() === 'ssh') {
      const [host, port] = addr.split(':')
      const cmd = `ssh root@${host} -p ${port || '22'}`
      copy(cmd)
      window.setTimeout(() => alert(`SSH 连接命令已复制：\n\n${cmd}`), 100)
      return
    }
    window.open(`http://${addr}`, '_blank', 'noopener,noreferrer')
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
          action={
            <div className="flex items-center gap-2 text-xs">
              <span className="text-slate-400">最优 STUN:</span>
              <span className="font-mono font-semibold text-blue-500">{config.bestStun || '--'}</span>
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
                <div className="flex min-w-[140px] flex-col items-center gap-1 rounded-xl bg-amber-50 px-4 py-3 ring-1 ring-amber-200">
                  <div className="flex items-center gap-1.5">
                    <Wifi className="h-4 w-4 text-amber-500" />
                    <span className="text-xs font-semibold text-amber-600">路由 NAT {r.natLevel}</span>
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
              <button
                onClick={() => setDeviceModal({ open: true })}
                className="flex items-center gap-1 rounded-md bg-blue-500 px-2 py-1 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 hover:bg-blue-600"
              >
                <Plus className="h-3 w-3" /> 添加
              </button>
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
                  const fullAddr =
                    externalPort > 0 && config.publicIP ? `${config.publicIP}:${externalPort}` : ''
                  const inHome = homeSet.has(key)
                  const lastError = status?.lastError || ''
                  const restartCount = status?.restartCount ?? 0
                  return (
                    <div
                      key={svc.id}
                      className="flex flex-col rounded-xl border border-slate-200/80 p-3 transition hover:border-blue-200 hover:shadow-md"
                    >
                      <div className="mb-2 flex items-center gap-2 border-b border-slate-100 pb-2">
                        <span
                          className={`h-2 w-2 rounded-full ${running ? 'bg-emerald-500 shadow-[0_0_6px_rgba(16,185,129,0.6)]' : 'bg-rose-400 shadow-[0_0_6px_rgba(248,113,113,0.5)]'}`}
                        />
                        <span className="flex-1 truncate text-sm font-bold text-slate-800">
                          {svc.name || '未命名服务'}
                        </span>
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
                          className="grid h-6 w-6 place-items-center rounded-md bg-emerald-50 text-emerald-600 transition hover:bg-emerald-100"
                          title="编辑"
                        >
                          <Pencil className="h-3 w-3" />
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
                          </span>
                        </div>
                        <div className="flex items-center justify-between">
                          <span className="text-slate-500">内部端口</span>
                          <span className="font-mono font-semibold text-slate-700">{svc.internalPort}</span>
                        </div>
                        <div className="flex items-center justify-between">
                          <span className="text-slate-500">穿透状态</span>
                          <span
                            className={`font-semibold ${running ? 'text-emerald-600' : 'text-rose-500'}`}
                          >
                            {running ? '✅ 穿透成功' : `❌ ${phaseLabel[phase] || phase}`}
                          </span>
                        </div>
                      </div>

                      {lastError && (
                        <div className="mt-2 rounded-lg border border-rose-200 bg-rose-50 px-2 py-1.5 text-[11px] text-rose-600">
                          ⚠️ <span className="font-semibold">错误:</span> {lastError}
                          {restartCount > 0 && ` (重启 ${restartCount} 次)`}
                        </div>
                      )}

                      {fullAddr ? (
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
                              onClick={() => openAddress(svc.protocol, fullAddr)}
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
      {deviceModal.open && (
        <DeviceModal
          initial={deviceModal.initial}
          onCancel={() => setDeviceModal({ open: false })}
          onSubmit={submitDevice}
        />
      )}
      {serviceModal.open && (
        <ServiceModal
          initial={serviceModal.initial}
          initialShowOnHome={
            serviceModal.initial
              ? homeSet.has(`${serviceModal.deviceId}-${serviceModal.initial.id}`)
              : false
          }
          onCancel={() => setServiceModal({ open: false, deviceId: 0 })}
          onSubmit={submitService}
        />
      )}
      {logKey && <LogModal status={logStatus} onClose={() => setLogKey(null)} />}

      {/* Toasts */}
      <div className="pointer-events-none fixed bottom-6 left-1/2 z-[60] flex -translate-x-1/2 flex-col items-center gap-2">
        {toasts.map((t) => (
          <div
            key={t.id}
            className="rounded-lg bg-slate-900/85 px-4 py-2 text-sm text-white shadow-lg"
          >
            {t.text}
          </div>
        ))}
      </div>
    </div>
  )
}
