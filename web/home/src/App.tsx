import { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState } from 'react'
import {
  ExternalLink,
  Globe2,
  Home as HomeIcon,
  Image as ImageIcon,
  LayoutGrid,
  Pencil,
  Plus,
  Search,
  Settings,
  Tags,
  Trash2,
  Upload,
  X,
} from 'lucide-react'
import * as api from './api'
import type {
  AppAddresses,
  AppView,
  Category,
  HomeConfig,
  LayoutMode,
  NetworkPrefer,
  SearchEngine,
  WallpaperMode,
  WallpaperResolution,
} from './types'
import './App.css'

const networkPreferOrder: NetworkPrefer[] = ['wanV4', 'wanV6', 'lan']
const networkPreferLabel: Record<NetworkPrefer, string> = {
  wanV4: '公网 v4',
  wanV6: '公网 v6',
  lan: '内网',
}
const pageCols = 8
const pageRows = 5
const pageSize = pageCols * pageRows
const searchHistoryMax = 24
const wallpaperFallbackMs = 3000

/* ============ 辅助函数 ============ */

function iconSrc(icon: string): string {
  if (!icon) return ''
  if (/^(https?:|data:)/i.test(icon)) return icon
  if (icon.startsWith('/')) return icon
  return `/${icon}`
}

function resolveOpenUrl(app: AppView, prefer: NetworkPrefer): string | null {
  const fallback: Record<NetworkPrefer, (keyof AppAddresses)[]> = {
    wanV4: ['wanV4', 'wanV6', 'lan'],
    wanV6: ['wanV6', 'wanV4', 'lan'],
    lan: ['lan', 'wanV4', 'wanV6'],
  }
  for (const key of fallback[prefer]) {
    const v = app.addresses?.[key]
    if (v) return v
  }
  return null
}

function openWithWhiteLoading(url: string) {
  const page = window.open('', '_blank')
  if (!page) {
    window.open(url, '_blank')
    return
  }
  page.document.write(
    `<!doctype html><html><head><title>Loading...</title><style>html,body{margin:0;width:100%;height:100%;background:#fff;color:#64748b;font-family:system-ui,-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;}body{display:grid;place-items:center;}</style></head><body></body></html>`,
  )
  page.document.close()
  window.setTimeout(() => {
    page.location.href = url
  }, 50)
}

function sortByPagedOrder(apps: AppView[]): AppView[] {
  return [...apps].sort((a, b) => a.pagedOrder - b.pagedOrder)
}

function groupByCategory(apps: AppView[], categories: Category[]) {
  const sorted = [...categories].sort((a, b) => a.order - b.order)
  const validIds = new Set(sorted.map((c) => c.id))
  const byCategoryScroll = (a: AppView, b: AppView) => a.scrollOrder - b.scrollOrder
  const groups = sorted.map((cat) => ({
    category: cat,
    apps: apps.filter((a) => a.categoryId === cat.id).sort(byCategoryScroll),
  }))
  const uncategorized = apps
    .filter((a) => !a.categoryId || !validIds.has(a.categoryId))
    .sort(byCategoryScroll)
  return { groups, uncategorized }
}

/* ============ 基础组件 ============ */

function Clock({ showTime, title }: { showTime: boolean; title: string }) {
  const [now, setNow] = useState(() => new Date())
  useEffect(() => {
    if (!showTime) return
    const t = window.setInterval(() => setNow(new Date()), 1000)
    return () => window.clearInterval(t)
  }, [showTime])
  const time = now.toLocaleTimeString('zh-CN', { hour12: false })
  const date = now.toLocaleDateString('zh-CN', { month: 'long', day: 'numeric', weekday: 'long' })
  return (
    <div className="flex items-end justify-center gap-3 text-white drop-shadow-[0_2px_8px_rgba(0,0,0,0.65)]">
      <h1 className="text-5xl font-black tracking-tight">{title || 'LinkStar'}</h1>
      {showTime && (
        <div className="mb-1 text-left">
          <div className="text-2xl font-bold leading-none">{time}</div>
          <div className="mt-1 text-sm font-medium opacity-90">{date}</div>
        </div>
      )}
    </div>
  )
}

function IconPreview({
  icon,
  fallback,
  color,
  className,
}: {
  icon: string
  fallback: string
  color: string
  className?: string
}) {
  if (icon) {
    return <img src={iconSrc(icon)} alt="" className={`object-contain ${className ?? ''}`} />
  }
  return (
    <div className={`grid place-items-center bg-gradient-to-br ${color || 'from-slate-400 to-slate-600'} text-white ${className ?? ''}`}>
      <span className="text-lg font-black tracking-tight">{fallback.charAt(0).toUpperCase()}</span>
    </div>
  )
}

function AppIcon({
  app,
  prefer,
  onContextMenu,
  draggable,
  isDragging,
  dragGhost,
  onDragStart,
  onDragEnter,
  onDragEnd,
  onPointerDown,
  suppressClickRef,
}: {
  app: AppView
  prefer: NetworkPrefer
  onContextMenu: (e: React.MouseEvent, app: AppView) => void
  draggable?: boolean
  isDragging?: boolean
  dragGhost?: 'fade' | 'hidden'
  onDragStart?: (e: React.DragEvent<HTMLButtonElement>, app: AppView) => void
  onDragEnter?: (e: React.DragEvent<HTMLButtonElement>, app: AppView) => void
  onDragEnd?: () => void
  onPointerDown?: (e: React.PointerEvent<HTMLButtonElement>, app: AppView) => void
  suppressClickRef?: { current: boolean }
}) {
  const handleClick = () => {
    if (suppressClickRef?.current) {
      suppressClickRef.current = false
      return
    }
    const url = resolveOpenUrl(app, prefer)
    if (!url) return
    openWithWhiteLoading(url)
  }
  const ghostClass = isDragging
    ? dragGhost === 'hidden'
      ? 'opacity-0'
      : 'opacity-40'
    : ''
  return (
    <button
      type="button"
      draggable={draggable}
      onDragStart={(e) => {
        if (suppressClickRef) suppressClickRef.current = true
        if (e.dataTransfer) {
          e.dataTransfer.effectAllowed = 'move'
          try {
            e.dataTransfer.setData('text/plain', app.id)
          } catch {
            // ignore
          }
        }
        onDragStart?.(e, app)
      }}
      onDragEnter={(e) => onDragEnter?.(e, app)}
      onDragOver={(e) => {
        if (draggable) {
          e.preventDefault()
          if (e.dataTransfer) e.dataTransfer.dropEffect = 'move'
        }
      }}
      onDragEnd={() => onDragEnd?.()}
      onPointerDown={(e) => onPointerDown?.(e, app)}
      onClick={handleClick}
      onContextMenu={(e) => onContextMenu(e, app)}
      style={onPointerDown ? { touchAction: 'none' } : undefined}
      className={`group flex w-24 flex-col items-center gap-2 text-white outline-none transition-opacity ${ghostClass}`}
    >
      <div className="relative grid h-16 w-16 place-items-center overflow-hidden rounded-2xl shadow-lg ring-1 ring-white/30 transition duration-200 group-hover:-translate-y-1 group-hover:scale-105 group-hover:shadow-2xl">
        <IconPreview icon={app.icon} fallback={app.name} color={app.color} className="h-full w-full" />
        {app.online === true && (
          <span className="absolute -right-1 -top-1 h-3 w-3 rounded-full border-2 border-white bg-emerald-400" />
        )}
        {app.online === false && (
          <span className="absolute -right-1 -top-1 h-3 w-3 rounded-full border-2 border-white bg-slate-400" />
        )}
      </div>
      <span className="max-w-24 truncate text-sm font-medium drop-shadow-[0_1px_2px_rgba(0,0,0,0.9)]">{app.name}</span>
    </button>
  )
}

function PagedGrid({
  page,
  pageIndex,
  interactive,
  draggingAppId,
  hoverSlot,
  networkPrefer,
  onContextMenu,
  onEmptyContextMenu,
  onIconPointerDown,
  suppressClickRef,
}: {
  page: (AppView | null)[]
  pageIndex: number
  interactive: boolean
  draggingAppId: string | null
  hoverSlot: number | null
  networkPrefer: NetworkPrefer
  onContextMenu: (e: React.MouseEvent, app: AppView) => void
  onEmptyContextMenu?: (e: React.MouseEvent, absPagedOrder: number) => void
  onIconPointerDown?: (e: React.PointerEvent<HTMLButtonElement>, app: AppView) => void
  suppressClickRef?: { current: boolean }
}) {
  return (
    <div
      className="grid h-full w-full content-start justify-items-center gap-x-3 gap-y-4 px-2 pb-4 pt-4"
      style={{
        gridTemplateColumns: `repeat(${pageCols}, minmax(0, 1fr))`,
        gridTemplateRows: `repeat(${pageRows}, minmax(0, auto))`,
      }}
    >
      {page.map((slot, slotIdx) => {
        const absPO = pageIndex * pageSize + slotIdx + 1
        const isHover =
          interactive && draggingAppId !== null && hoverSlot === absPO && slot?.id !== draggingAppId
        return (
          <div
            key={slotIdx}
            data-slot={absPO}
            onContextMenu={
              !slot && interactive && onEmptyContextMenu
                ? (e) => {
                    e.preventDefault()
                    onEmptyContextMenu(e, absPO)
                  }
                : undefined
            }
            className={`flex h-24 w-full items-center justify-center rounded-2xl transition-colors duration-100 ${
              isHover ? 'bg-white/15 ring-2 ring-white/40' : ''
            }`}
          >
            {slot ? (
              <AppIcon
                app={slot}
                prefer={networkPrefer}
                isDragging={draggingAppId === slot.id}
                dragGhost="hidden"
                onPointerDown={interactive ? onIconPointerDown : undefined}
                suppressClickRef={suppressClickRef}
                onContextMenu={onContextMenu}
              />
            ) : null}
          </div>
        )
      })}
    </div>
  )
}

function SearchEngineIcon({
  engine,
  active,
  onClick,
}: {
  engine: SearchEngine
  active: boolean
  onClick: () => void
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className="group flex w-16 flex-col items-center gap-1.5 outline-none"
    >
      <div
        className={`grid h-14 w-14 place-items-center overflow-hidden rounded-xl bg-slate-100 text-white shadow-sm transition duration-200 group-hover:-translate-y-1 group-hover:shadow-lg ${
          active ? 'ring-4 ring-blue-200' : 'ring-1 ring-slate-200'
        }`}
      >
        {engine.icon ? (
          <img src={iconSrc(engine.icon)} alt="" className="h-11 w-11 object-contain" />
        ) : (
          <div className={`grid h-full w-full place-items-center rounded-xl bg-gradient-to-br ${engine.color || 'from-slate-400 to-slate-600'} text-white`}>
            <span className="text-lg font-black tracking-tight">{(engine.shortName || engine.name).charAt(0).toUpperCase()}</span>
          </div>
        )}
      </div>
      <span className={`max-w-16 truncate text-xs ${active ? 'font-semibold text-blue-600' : 'text-slate-600'}`}>
        {engine.name}
      </span>
    </button>
  )
}

function IconUploader({ value, onChange }: { value: string; onChange: (v: string) => void }) {
  const ref = useRef<HTMLInputElement>(null)
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const pick = async (file: File | null | undefined) => {
    if (!file) return
    setErr('')
    setBusy(true)
    try {
      const path = await api.uploadIcon(file)
      onChange(path)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '上传失败')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div>
      <div className="flex items-center gap-3">
        <div className="grid h-12 w-12 place-items-center overflow-hidden rounded-xl bg-slate-100 ring-1 ring-slate-200">
          {value ? (
            <img src={iconSrc(value)} alt="" className="h-full w-full object-contain" />
          ) : (
            <ImageIcon className="h-5 w-5 text-slate-400" />
          )}
        </div>
        <button
          type="button"
          onClick={() => ref.current?.click()}
          disabled={busy}
          className="flex items-center gap-1.5 rounded-xl bg-slate-100 px-3 py-2 text-sm font-semibold text-slate-600 transition hover:bg-slate-200 disabled:opacity-50"
        >
          <Upload className="h-4 w-4" />
          {busy ? '上传中...' : value ? '替换' : '上传'}
        </button>
        {value && (
          <button
            type="button"
            onClick={() => onChange('')}
            className="rounded-xl px-2 py-1 text-xs text-slate-400 hover:text-rose-500"
          >
            移除
          </button>
        )}
        <input
          ref={ref}
          type="file"
          accept="image/png,image/jpeg,image/webp,image/svg+xml,image/x-icon,image/vnd.microsoft.icon"
          className="hidden"
          onChange={(e) => {
            pick(e.target.files?.[0])
            e.target.value = ''
          }}
        />
      </div>
      {err && <div className="mt-1 text-xs text-rose-500">{err}</div>}
    </div>
  )
}

/* ============ Add / Edit Site Modal ============ */

interface AppFormState {
  name: string
  icon: string
  color: string
  categoryId: string
  wanV4: string
  wanV6: string
  lan: string
}

const emptyForm: AppFormState = {
  name: '',
  icon: '',
  color: 'from-sky-400 to-blue-600',
  categoryId: '',
  wanV4: '',
  wanV6: '',
  lan: '',
}

const colorPresets = [
  'from-sky-400 to-blue-600',
  'from-violet-400 to-fuchsia-700',
  'from-rose-400 to-red-600',
  'from-amber-300 to-orange-600',
  'from-emerald-400 to-teal-700',
  'from-pink-400 to-rose-700',
  'from-slate-600 to-zinc-900',
  'from-cyan-300 to-sky-600',
]

function AppFormModal({
  initial,
  defaultCategoryId,
  categories,
  isStun,
  onCancel,
  onSubmit,
}: {
  initial?: AppView
  defaultCategoryId?: string
  categories: Category[]
  isStun: boolean
  onCancel: () => void
  onSubmit: (form: AppFormState) => Promise<void>
}) {
  const [form, setForm] = useState<AppFormState>(() => {
    if (!initial) return { ...emptyForm, categoryId: defaultCategoryId ?? '' }
    return {
      name: initial.name,
      icon: initial.icon,
      color: initial.color || emptyForm.color,
      categoryId: initial.categoryId,
      wanV4: initial.addresses?.wanV4 ?? '',
      wanV6: initial.addresses?.wanV6 ?? '',
      lan: initial.addresses?.lan ?? '',
    }
  })
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const submit = async () => {
    if (!form.name.trim()) {
      setErr('请填写名称')
      return
    }
    if (!isStun && !initial && !form.wanV4 && !form.wanV6 && !form.lan) {
      setErr('至少填写一个地址')
      return
    }
    setErr('')
    setBusy(true)
    try {
      await onSubmit(form)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '保存失败')
      setBusy(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 grid place-items-center bg-black/40 px-4 py-6 backdrop-blur-sm"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="w-full max-w-md rounded-3xl bg-white p-6 text-slate-700 shadow-2xl ring-1 ring-slate-200">
        <div className="mb-5 flex items-center justify-between">
          <div className="text-base font-bold text-slate-800">{initial ? '编辑应用' : '添加常用网站'}</div>
          <button
            type="button"
            onClick={onCancel}
            className="grid h-7 w-7 place-items-center rounded-full text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="space-y-4">
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-slate-500">名称</div>
            <input
              autoFocus
              value={form.name}
              onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
              placeholder="GitHub"
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
            />
          </label>

          <div>
            <div className="mb-1 text-xs font-semibold text-slate-500">图标</div>
            <IconUploader value={form.icon} onChange={(v) => setForm((p) => ({ ...p, icon: v }))} />
          </div>

          <div>
            <div className="mb-1 text-xs font-semibold text-slate-500">无图标时的背景</div>
            <div className="flex flex-wrap gap-1.5">
              {colorPresets.map((c) => (
                <button
                  key={c}
                  type="button"
                  onClick={() => setForm((p) => ({ ...p, color: c }))}
                  className={`h-7 w-7 rounded-lg bg-gradient-to-br ${c} ${
                    form.color === c ? 'ring-2 ring-blue-400 ring-offset-2' : ''
                  }`}
                />
              ))}
            </div>
          </div>

          <label className="block">
            <div className="mb-1 text-xs font-semibold text-slate-500">分类</div>
            <select
              value={form.categoryId}
              onChange={(e) => setForm((p) => ({ ...p, categoryId: e.target.value }))}
              className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
            >
              <option value="">未分类</option>
              {categories.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.name}
                </option>
              ))}
            </select>
          </label>

          {!isStun && (
            <div className="space-y-2 rounded-2xl bg-slate-50 p-3">
              <div className="text-xs font-semibold text-slate-500">访问地址 (至少一项)</div>
              <input
                value={form.wanV4}
                onChange={(e) => setForm((p) => ({ ...p, wanV4: e.target.value }))}
                placeholder="公网 v4 / URL  如 https://github.com"
                className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
              />
              <input
                value={form.wanV6}
                onChange={(e) => setForm((p) => ({ ...p, wanV6: e.target.value }))}
                placeholder="公网 v6  如 http://[2001:db8::1]:8080"
                className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
              />
              <input
                value={form.lan}
                onChange={(e) => setForm((p) => ({ ...p, lan: e.target.value }))}
                placeholder="内网  如 http://192.168.1.10:8080"
                className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
              />
            </div>
          )}

          {isStun && (
            <div className="rounded-2xl bg-amber-50 p-3 text-xs text-amber-700">
              这是来自 STUN 的应用，地址由内网穿透自动维护，仅可修改名称 / 图标 / 颜色 / 分类。
            </div>
          )}

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

/* ============ Search Engine Modal ============ */

interface SearchEngineFormState {
  id: string
  name: string
  shortName: string
  url: string
  color: string
  icon: string
}

const emptyEngineForm: SearchEngineFormState = {
  id: '',
  name: '',
  shortName: '',
  url: '',
  color: 'from-sky-400 to-cyan-600',
  icon: '',
}

function SearchEngineFormModal({
  initial,
  onCancel,
  onSubmit,
}: {
  initial?: SearchEngine
  onCancel: () => void
  onSubmit: (form: SearchEngineFormState) => Promise<void>
}) {
  const [form, setForm] = useState<SearchEngineFormState>(() =>
    initial
      ? {
          id: initial.id,
          name: initial.name,
          shortName: initial.shortName,
          url: initial.url,
          color: initial.color || emptyEngineForm.color,
          icon: initial.icon ?? '',
        }
      : emptyEngineForm,
  )
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const submit = async () => {
    if (!form.name.trim() || !form.url.trim()) {
      setErr('name / url 不能为空')
      return
    }
    setErr('')
    setBusy(true)
    try {
      await onSubmit(form)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '保存失败')
      setBusy(false)
    }
  }

  return (
    <div
      className="fixed inset-0 z-50 grid place-items-center bg-black/40 px-4 py-6 backdrop-blur-sm"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="w-full max-w-md rounded-3xl bg-white p-6 text-slate-700 shadow-2xl ring-1 ring-slate-200">
        <div className="mb-5 flex items-center justify-between">
          <div className="text-base font-bold text-slate-800">{initial ? '编辑搜索引擎' : '添加搜索引擎'}</div>
          <button
            type="button"
            onClick={onCancel}
            className="grid h-7 w-7 place-items-center rounded-full text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>

        <div className="space-y-3">
          <input
            value={form.name}
            onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
            placeholder="名称  如 必应"
            className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
          />
          <input
            value={form.shortName}
            onChange={(e) => setForm((p) => ({ ...p, shortName: e.target.value }))}
            placeholder="简称  如 B"
            className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
          />
          <input
            value={form.url}
            onChange={(e) => setForm((p) => ({ ...p, url: e.target.value }))}
            placeholder="URL  如 https://www.bing.com/search?q="
            className="w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400"
          />
          <div>
            <div className="mb-1 text-xs font-semibold text-slate-500">图标</div>
            <IconUploader value={form.icon} onChange={(v) => setForm((p) => ({ ...p, icon: v }))} />
          </div>
          <div>
            <div className="mb-1 text-xs font-semibold text-slate-500">背景</div>
            <div className="flex flex-wrap gap-1.5">
              {colorPresets.map((c) => (
                <button
                  key={c}
                  type="button"
                  onClick={() => setForm((p) => ({ ...p, color: c }))}
                  className={`h-7 w-7 rounded-lg bg-gradient-to-br ${c} ${
                    form.color === c ? 'ring-2 ring-blue-400 ring-offset-2' : ''
                  }`}
                />
              ))}
            </div>
          </div>
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

/* ============ 主组件 ============ */

function App() {
  const [config, setConfig] = useState<HomeConfig | null>(null)
  const [loadErr, setLoadErr] = useState('')

  // UI 状态
  const [query, setQuery] = useState('')
  const [appPage, setAppPage] = useState(0)
  const [showHistory, setShowHistory] = useState(false)
  const [showEngines, setShowEngines] = useState(false)
  const [showSettings, setShowSettings] = useState(false)
  const [settingsTab, setSettingsTab] = useState<'appearance' | 'layout' | 'categories' | 'search'>('appearance')

  // 模态
  const [appForm, setAppForm] = useState<{ open: boolean; initial?: AppView; defaultCategoryId?: string }>({ open: false })
  const [engineForm, setEngineForm] = useState<{ open: boolean; initial?: SearchEngine }>({ open: false })
  type CtxMenu =
    | { kind: 'app'; x: number; y: number; app: AppView }
    | { kind: 'add'; x: number; y: number; pagedOrder?: number; categoryId?: string }
  const [contextMenu, setContextMenu] = useState<CtxMenu | null>(null)

  // 壁纸状态
  const [showDefaultWallpaper, setShowDefaultWallpaper] = useState(false)
  const [backgroundReady, setBackgroundReady] = useState(false)
  const [wallpaperUrl, setWallpaperUrl] = useState<string | null>(null)
  const [wallpaperLoaded, setWallpaperLoaded] = useState(false)
  const wallpaperFallbackTimer = useRef<number | null>(null)

  // 翻页动画
  const searchInputRef = useRef<HTMLInputElement>(null)
  const appsPagerRef = useRef<HTMLDivElement>(null)
  const animatingRef = useRef(false)
  const slideTimeoutRef = useRef<number | null>(null)
  const touchStartXRef = useRef<number | null>(null)
  const touchStartYRef = useRef<number | null>(null)
  const dragStartXRef = useRef<number | null>(null)
  const dragStartYRef = useRef<number | null>(null)

  type AppSlide = { from: number; axis: 'x' | 'y'; dir: 1 | -1 }
  const [appSlide, setAppSlide] = useState<AppSlide | null>(null)

  // 分类拖拽
  const [dragOverCategoryId, setDragOverCategoryId] = useState<string | null>(null)
  const [editingCategoryId, setEditingCategoryId] = useState<string | null>(null)
  const [editingCategoryName, setEditingCategoryName] = useState('')

  // 图标拖拽布局
  const [draggingAppId, setDraggingAppId] = useState<string | null>(null)
  const dragContextRef = useRef<{ kind: 'scroll'; categoryId: string; sourceId: string } | null>(null)
  const dragDirtyRef = useRef(false)

  // paged 模式自由布局 (pointer events)
  const [pagedDragSource, setPagedDragSource] = useState<{ id: string; sourceSlot: number } | null>(null)
  const [pagedHoverSlot, setPagedHoverSlot] = useState<number | null>(null)
  const pagedDragInfoRef = useRef<{
    appId: string
    sourceSlot: number
    pointerId: number
    offsetX: number
    offsetY: number
    startX: number
    startY: number
    lastX: number
    lastY: number
    moved: boolean
  } | null>(null)
  const floatingIconRef = useRef<HTMLDivElement | null>(null)
  const suppressIconClickRef = useRef(false)
  const pendingAddSlotRef = useRef<number | null>(null)
  const edgeFlipTimerRef = useRef<number | null>(null)
  const dotHoverTimerRef = useRef<number | null>(null)
  const edgeDirectionRef = useRef<1 | -1 | null>(null)
  const hoveredDotRef = useRef<number | null>(null)

  /* ============ 初始加载 + 定期刷新 ============ */

  const reload = useCallback(async () => {
    const data = await api.getConfig()
    setConfig(data)
    return data
  }, [])

  useEffect(() => {
    reload().catch((e) => setLoadErr(e instanceof Error ? e.message : String(e)))
    const t = window.setInterval(() => {
      api.getConfig().then(setConfig).catch(() => undefined)
    }, 15000)
    return () => window.clearInterval(t)
  }, [reload])

  /* ============ 派生数据 ============ */

  const layoutMode = config?.layoutMode ?? 'paged-free'
  const isScrollMode = layoutMode === 'scroll'
  const allowHorizontal = layoutMode === 'paged-horizontal' || layoutMode === 'paged-free'
  const allowVertical = layoutMode === 'paged-vertical' || layoutMode === 'paged-free'
  const networkPrefer = config?.networkPrefer ?? 'wanV4'

  const sortedEngines = useMemo(
    () => (config ? [...config.searchEngines].sort((a, b) => a.order - b.order) : []),
    [config],
  )
  const engine = useMemo(
    () => sortedEngines.find((e) => e.id === config?.defaultSearchEngineId) ?? sortedEngines[0],
    [sortedEngines, config?.defaultSearchEngineId],
  )

  const pagedApps = useMemo(() => (config ? sortByPagedOrder(config.apps) : []), [config])
  const [desiredPageTotal, setDesiredPageTotal] = useState(0)
  const autoPageCount = useMemo(() => {
    let maxPO = 0
    for (const app of pagedApps) {
      const po = app.pagedOrder || 1
      if (po > maxPO) maxPO = po
    }
    return maxPO > 0 ? Math.ceil(maxPO / pageSize) : 0
  }, [pagedApps])
  // 槽位型分页:每页 pageSize 个格子,空格为 null。pagedOrder = page*pageSize+slot+1
  const appPages = useMemo<(AppView | null)[][]>(() => {
    const slotMap = new Map<number, AppView>()
    for (const app of pagedApps) {
      const po = app.pagedOrder || 1
      slotMap.set(po, app)
    }
    const total = Math.max(autoPageCount, desiredPageTotal, 1)
    const pages: (AppView | null)[][] = []
    for (let p = 0; p < total; p++) {
      const slots: (AppView | null)[] = []
      for (let s = 0; s < pageSize; s++) {
        slots.push(slotMap.get(p * pageSize + s + 1) ?? null)
      }
      pages.push(slots)
    }
    return pages
  }, [pagedApps, autoPageCount, desiredPageTotal])
  const pendingNavigatePageRef = useRef<number | null>(null)

  const grouped = useMemo(
    () => (config ? groupByCategory(config.apps, config.categories) : { groups: [], uncategorized: [] }),
    [config],
  )

  /* ============ 壁纸 ============ */

  const clearWallpaperFallbackTimer = () => {
    if (wallpaperFallbackTimer.current !== null) {
      window.clearTimeout(wallpaperFallbackTimer.current)
      wallpaperFallbackTimer.current = null
    }
  }

  useEffect(() => {
    if (!config) return
    let canceled = false
    clearWallpaperFallbackTimer()

    if (config.wallpaper.mode === 'default') {
      setShowDefaultWallpaper(true)
      setBackgroundReady(true)
      setWallpaperLoaded(false)
      setWallpaperUrl(null)
      return
    }

    setShowDefaultWallpaper(false)
    setBackgroundReady(false)
    setWallpaperLoaded(false)
    wallpaperFallbackTimer.current = window.setTimeout(() => {
      if (canceled) return
      canceled = true
      setWallpaperLoaded(false)
      setWallpaperUrl(null)
      setShowDefaultWallpaper(true)
      setBackgroundReady(true)
      clearWallpaperFallbackTimer()
    }, wallpaperFallbackMs)

    api
      .getBingWallpaper(config.wallpaper.resolution)
      .then((data) => {
        if (!data.url || canceled) return
        const image = new Image()
        image.onload = () => {
          if (canceled) return
          clearWallpaperFallbackTimer()
          setShowDefaultWallpaper(false)
          setWallpaperUrl(data.url)
          setWallpaperLoaded(true)
          setBackgroundReady(true)
        }
        image.onerror = () => {
          if (!canceled) {
            setShowDefaultWallpaper(true)
            setBackgroundReady(true)
          }
          clearWallpaperFallbackTimer()
        }
        image.src = data.url
      })
      .catch(() => {
        if (!canceled) {
          setShowDefaultWallpaper(true)
          setBackgroundReady(true)
        }
        clearWallpaperFallbackTimer()
      })

    return () => {
      canceled = true
      clearWallpaperFallbackTimer()
    }
  }, [config?.wallpaper.mode, config?.wallpaper.resolution])

  /* ============ 翻页动画 ============ */

  const movePage = (delta: number, axis: 'x' | 'y') => {
    if (animatingRef.current) return
    setAppPage((curr) => {
      const next = curr + delta
      if (next === curr || next < 0 || next >= appPages.length) return curr
      animatingRef.current = true
      setAppSlide({ from: curr, axis, dir: delta > 0 ? 1 : -1 })
      if (slideTimeoutRef.current !== null) window.clearTimeout(slideTimeoutRef.current)
      slideTimeoutRef.current = window.setTimeout(() => {
        setAppSlide(null)
        animatingRef.current = false
      }, 360)
      return next
    })
  }

  const goToAppPage = (target: number, axis: 'x' | 'y') => {
    if (animatingRef.current) return
    setAppPage((curr) => {
      if (target === curr || target < 0 || target >= appPages.length) return curr
      animatingRef.current = true
      setAppSlide({ from: curr, axis, dir: target > curr ? 1 : -1 })
      if (slideTimeoutRef.current !== null) window.clearTimeout(slideTimeoutRef.current)
      slideTimeoutRef.current = window.setTimeout(() => {
        setAppSlide(null)
        animatingRef.current = false
      }, 360)
      return target
    })
  }

  useEffect(() => {
    if (appPage > appPages.length - 1) setAppPage(Math.max(0, appPages.length - 1))
  }, [appPages.length, appPage])

  useEffect(() => {
    const pending = pendingNavigatePageRef.current
    if (pending !== null && pending < appPages.length) {
      pendingNavigatePageRef.current = null
      goToAppPage(pending, allowHorizontal ? 'x' : 'y')
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [appPages.length])

  const addNewPage = () => {
    setDesiredPageTotal((prev) => {
      const next = Math.max(prev, autoPageCount) + 1
      pendingNavigatePageRef.current = next - 1
      return next
    })
  }

  useEffect(() => {
    if (showSettings || isScrollMode || !config) return

    const resolveAxis = (absX: number, absY: number): 'x' | 'y' | null => {
      if (allowHorizontal && allowVertical) return absX > absY ? 'x' : 'y'
      if (allowHorizontal) return absX > absY ? 'x' : null
      if (allowVertical) return absY > absX ? 'y' : null
      return null
    }

    const onWheel = (event: WheelEvent) => {
      const absX = Math.abs(event.deltaX)
      const absY = Math.abs(event.deltaY)
      if (Math.max(absX, absY) < 8) return
      const axis = resolveAxis(absX, absY)
      if (!axis) return
      event.preventDefault()
      const delta = axis === 'x' ? event.deltaX : event.deltaY
      const dir: 1 | -1 = delta > 0 ? 1 : -1
      // 拖拽中: 不走滑动动画,直接换页;避免源元素被卸载导致跟手中断
      if (pagedDragInfoRef.current?.moved) {
        setAppPage((curr) => {
          const next = curr + dir
          if (next < 0 || next >= appPages.length) return curr
          return next
        })
        return
      }
      movePage(dir, axis)
    }
    const onTouchStart = (event: TouchEvent) => {
      touchStartXRef.current = event.touches[0].clientX
      touchStartYRef.current = event.touches[0].clientY
    }
    const onTouchEnd = (event: TouchEvent) => {
      if (touchStartXRef.current === null || touchStartYRef.current === null) return
      const dx = touchStartXRef.current - event.changedTouches[0].clientX
      const dy = touchStartYRef.current - event.changedTouches[0].clientY
      touchStartXRef.current = null
      touchStartYRef.current = null
      const absX = Math.abs(dx)
      const absY = Math.abs(dy)
      if (Math.max(absX, absY) < 40) return
      const axis = resolveAxis(absX, absY)
      if (!axis) return
      const delta = axis === 'x' ? dx : dy
      movePage(delta > 0 ? 1 : -1, axis)
    }
    const onMouseDown = (event: MouseEvent) => {
      if (event.button !== 0) return
      const target = event.target as HTMLElement | null
      if (target?.closest('input, textarea, select, button, a')) return
      dragStartXRef.current = event.clientX
      dragStartYRef.current = event.clientY
    }
    const onMouseUp = (event: MouseEvent) => {
      if (dragStartXRef.current === null || dragStartYRef.current === null) return
      const dx = dragStartXRef.current - event.clientX
      const dy = dragStartYRef.current - event.clientY
      dragStartXRef.current = null
      dragStartYRef.current = null
      const absX = Math.abs(dx)
      const absY = Math.abs(dy)
      if (Math.max(absX, absY) < 40) return
      const axis = resolveAxis(absX, absY)
      if (!axis) return
      const delta = axis === 'x' ? dx : dy
      movePage(delta > 0 ? 1 : -1, axis)
    }

    window.addEventListener('wheel', onWheel, { passive: false })
    window.addEventListener('touchstart', onTouchStart, { passive: true })
    window.addEventListener('touchend', onTouchEnd, { passive: true })
    window.addEventListener('mousedown', onMouseDown)
    window.addEventListener('mouseup', onMouseUp)
    return () => {
      window.removeEventListener('wheel', onWheel)
      window.removeEventListener('touchstart', onTouchStart)
      window.removeEventListener('touchend', onTouchEnd)
      window.removeEventListener('mousedown', onMouseDown)
      window.removeEventListener('mouseup', onMouseUp)
    }
  }, [appPages.length, showSettings, isScrollMode, allowHorizontal, allowVertical, config])

  const getSlideInClass = (axis: 'x' | 'y', dir: 1 | -1) => {
    if (axis === 'x') return dir === 1 ? 'app-slide-in-from-right' : 'app-slide-in-from-left'
    return dir === 1 ? 'app-slide-in-from-bottom' : 'app-slide-in-from-top'
  }
  const getSlideOutClass = (axis: 'x' | 'y', dir: 1 | -1) => {
    if (axis === 'x') return dir === 1 ? 'app-slide-out-to-left' : 'app-slide-out-to-right'
    return dir === 1 ? 'app-slide-out-to-top' : 'app-slide-out-to-bottom'
  }

  /* ============ 关闭右键菜单 ============ */
  useEffect(() => {
    if (!contextMenu) return
    const close = () => setContextMenu(null)
    window.addEventListener('click', close)
    window.addEventListener('contextmenu', close)
    return () => {
      window.removeEventListener('click', close)
      window.removeEventListener('contextmenu', close)
    }
  }, [contextMenu])

  /* ============ 业务动作 ============ */

  const submitSearch = (value = query) => {
    const keyword = value.trim()
    if (!keyword || !engine) return

    const isUrl = /^(https?:\/\/)?([\w-]+\.)+[\w-]{2,}/.test(keyword)
    if (isUrl) {
      openWithWhiteLoading(keyword.startsWith('http') ? keyword : `https://${keyword}`)
      return
    }

    // 同步打开新标签,保留 user activation 不被弹窗拦截
    openWithWhiteLoading(`${engine.url}${encodeURIComponent(keyword)}`)
    setShowHistory(false)

    // 历史后台异步写入,失败不影响搜索
    api.addSearchHistory(keyword).catch(() => undefined)
    setConfig((prev) =>
      prev
        ? {
            ...prev,
            searchHistory: [keyword, ...prev.searchHistory.filter((h) => h !== keyword)].slice(0, searchHistoryMax),
          }
        : prev,
    )
  }

  const clearHistory = async () => {
    try {
      await api.clearSearchHistory()
    } catch {
      // ignore
    }
    setConfig((prev) => (prev ? { ...prev, searchHistory: [] } : prev))
  }

  const setWallpaperMode = async (mode: WallpaperMode) => {
    if (!config) return
    const next = { ...config.wallpaper, mode }
    setConfig({ ...config, wallpaper: next })
    api.updateWallpaper(next).catch(() => reload())
  }
  const setWallpaperResolution = async (resolution: WallpaperResolution) => {
    if (!config) return
    const next = { ...config.wallpaper, resolution }
    setConfig({ ...config, wallpaper: next })
    api.updateWallpaper(next).catch(() => reload())
  }
  const blurDebounceRef = useRef<number | null>(null)
  const setWallpaperBlur = (blur: number) => {
    if (!config) return
    const next = { ...config.wallpaper, blur }
    setConfig({ ...config, wallpaper: next })
    if (blurDebounceRef.current !== null) window.clearTimeout(blurDebounceRef.current)
    blurDebounceRef.current = window.setTimeout(() => {
      api.updateWallpaper(next).catch(() => reload())
    }, 300)
  }

  const setLayoutMode = async (m: LayoutMode) => {
    if (!config) return
    setConfig({ ...config, layoutMode: m })
    api.updateLayout(m).catch(() => reload())
  }

  const cycleNetworkPrefer = async () => {
    if (!config) return
    const idx = networkPreferOrder.indexOf(config.networkPrefer)
    const next = networkPreferOrder[(idx + 1) % networkPreferOrder.length]
    setConfig({ ...config, networkPrefer: next })
    api.updateNetwork(next).catch(() => reload())
  }

  const setDefaultEngine = async (id: string) => {
    if (!config) return
    setConfig({ ...config, defaultSearchEngineId: id })
    api.setDefaultSearchEngine(id).catch(() => reload())
  }

  const submitEngine = async (form: SearchEngineFormState) => {
    const body = {
      id: form.id,
      name: form.name.trim(),
      shortName: form.shortName.trim(),
      url: form.url.trim(),
      color: form.color,
      icon: form.icon,
    }
    if (engineForm.initial) {
      await api.updateSearchEngine(body)
    } else {
      await api.addSearchEngine(body)
    }
    await reload()
    setEngineForm({ open: false })
  }

  const removeEngine = async (id: string) => {
    if (!window.confirm('删除该搜索引擎?')) return
    await api.deleteSearchEngine(id)
    await reload()
  }

  const newCategory = async () => {
    const name = window.prompt('分类名称', '新分类')
    if (!name) return
    await api.addCategory(name.trim() || '未命名')
    await reload()
  }

  const renameCategory = async (id: string, name: string) => {
    const trimmed = name.trim() || '未命名'
    await api.updateCategory(id, trimmed)
    await reload()
  }

  const removeCategory = async (id: string) => {
    if (!window.confirm('删除该分类? 其下应用将变为未分类')) return
    await api.deleteCategory(id)
    await reload()
  }

  const assignAppToCategory = async (appId: string, categoryId: string) => {
    setConfig((prev) =>
      prev
        ? {
            ...prev,
            apps: prev.apps.map((a) => (a.id === appId ? { ...a, categoryId } : a)),
          }
        : prev,
    )
    try {
      await api.setAppCategory(appId, categoryId)
    } catch {
      await reload()
    }
  }

  const submitApp = async (form: AppFormState) => {
    if (appForm.initial) {
      const body: api.UpdateAppRequest = {
        id: appForm.initial.id,
        name: form.name.trim(),
        icon: form.icon,
        color: form.color,
        categoryId: form.categoryId,
      }
      if (appForm.initial.type === 'static') {
        body.addresses = { wanV4: form.wanV4, wanV6: form.wanV6, lan: form.lan }
      }
      await api.updateApp(body)
    } else {
      const pagedOrder = pendingAddSlotRef.current ?? undefined
      pendingAddSlotRef.current = null
      await api.addApp({
        name: form.name.trim(),
        icon: form.icon,
        color: form.color,
        categoryId: form.categoryId,
        addresses: { wanV4: form.wanV4, wanV6: form.wanV6, lan: form.lan },
        pagedOrder,
      })
    }
    await reload()
    setAppForm({ open: false })
  }

  const startAppDrag = (
    e: React.DragEvent<HTMLButtonElement>,
    app: AppView,
    categoryId: string,
  ) => {
    setDraggingAppId(app.id)
    dragContextRef.current = { kind: 'scroll', categoryId, sourceId: app.id }
    dragDirtyRef.current = false
    if (e.dataTransfer) {
      e.dataTransfer.effectAllowed = 'move'
    }
  }

  const onDragEnterApp = (_e: React.DragEvent<HTMLButtonElement>, target: AppView) => {
    const ctx = dragContextRef.current
    if (!ctx || ctx.sourceId === target.id) return
    if ((target.categoryId || '') !== ctx.categoryId) return

    setConfig((prev) => {
      if (!prev) return prev
      const arr = prev.apps
        .filter((a) => (a.categoryId || '') === ctx.categoryId)
        .sort((a, b) => a.scrollOrder - b.scrollOrder)
      const fromIdx = arr.findIndex((a) => a.id === ctx.sourceId)
      const toIdx = arr.findIndex((a) => a.id === target.id)
      if (fromIdx < 0 || toIdx < 0 || fromIdx === toIdx) return prev
      const [moved] = arr.splice(fromIdx, 1)
      arr.splice(toIdx, 0, moved)
      const orderMap = new Map<string, number>()
      arr.forEach((a, i) => orderMap.set(a.id, i + 1))
      dragDirtyRef.current = true
      return {
        ...prev,
        apps: prev.apps.map((a) => {
          const next = orderMap.get(a.id)
          if (next === undefined) return a
          return { ...a, scrollOrder: next }
        }),
      }
    })
  }

  const endAppDrag = () => {
    const ctx = dragContextRef.current
    const dirty = dragDirtyRef.current
    setDraggingAppId(null)
    dragContextRef.current = null
    dragDirtyRef.current = false
    if (!ctx || !dirty) return

    setConfig((prev) => {
      if (!prev) return prev
      const ids = prev.apps
        .filter((a) => (a.categoryId || '') === ctx.categoryId)
        .sort((a, b) => a.scrollOrder - b.scrollOrder)
        .map((a) => a.id)
      api.reorderApps('scroll', ids, ctx.categoryId).catch(() => reload())
      return prev
    })
  }

  // ===== Paged 模式自由布局 (pointer events) =====
  const cancelEdgeFlip = () => {
    if (edgeFlipTimerRef.current !== null) {
      window.clearTimeout(edgeFlipTimerRef.current)
      edgeFlipTimerRef.current = null
    }
    edgeDirectionRef.current = null
  }
  const cancelDotHover = () => {
    if (dotHoverTimerRef.current !== null) {
      window.clearTimeout(dotHoverTimerRef.current)
      dotHoverTimerRef.current = null
    }
    hoveredDotRef.current = null
  }

  const scheduleEdgeFlip = (delta: 1 | -1) => {
    if (edgeFlipTimerRef.current !== null) return
    edgeFlipTimerRef.current = window.setTimeout(() => {
      edgeFlipTimerRef.current = null
      edgeDirectionRef.current = null // 允许进入后再次触发
      setAppPage((curr) => {
        const next = curr + delta
        if (next < 0 || next >= appPages.length) return curr
        return next
      })
    }, 500)
  }

  const detectEdgeDuringDrag = (x: number, y: number) => {
    const pager = appsPagerRef.current
    if (!pager) return
    const rect = pager.getBoundingClientRect()
    if (x < rect.left || x > rect.right || y < rect.top || y > rect.bottom) {
      if (edgeDirectionRef.current !== null) cancelEdgeFlip()
      return
    }
    const localX = x - rect.left
    const localY = y - rect.top
    const edge = 60
    let dir: 1 | -1 | null = null
    if (allowHorizontal) {
      if (localX < edge) dir = -1
      else if (localX > rect.width - edge) dir = 1
    }
    if (dir === null && allowVertical) {
      if (localY < edge) dir = -1
      else if (localY > rect.height - edge) dir = 1
    }
    if (dir === null) {
      if (edgeDirectionRef.current !== null) cancelEdgeFlip()
      return
    }
    if (edgeDirectionRef.current !== dir) {
      cancelEdgeFlip()
      edgeDirectionRef.current = dir
      scheduleEdgeFlip(dir)
    }
  }

  const detectDotHoverDuringDrag = (target: HTMLElement | null) => {
    const dotEl = target?.closest('[data-dot-index]') as HTMLElement | null
    if (!dotEl) {
      if (hoveredDotRef.current !== null) cancelDotHover()
      return
    }
    const idx = Number(dotEl.dataset.dotIndex)
    if (Number.isNaN(idx) || idx === appPage) {
      if (hoveredDotRef.current !== null) cancelDotHover()
      return
    }
    if (hoveredDotRef.current === idx) return
    cancelDotHover()
    hoveredDotRef.current = idx
    dotHoverTimerRef.current = window.setTimeout(() => {
      dotHoverTimerRef.current = null
      hoveredDotRef.current = null
      setAppPage(idx)
    }, 220)
  }

  const handleIconPointerDown = (e: React.PointerEvent<HTMLButtonElement>, app: AppView) => {
    if (e.button !== 0 && e.pointerType === 'mouse') return
    const target = e.currentTarget as HTMLButtonElement
    const rect = target.getBoundingClientRect()
    const info = {
      appId: app.id,
      sourceSlot: app.pagedOrder || 1,
      pointerId: e.pointerId,
      offsetX: e.clientX - rect.left,
      offsetY: e.clientY - rect.top,
      startX: e.clientX,
      startY: e.clientY,
      lastX: e.clientX,
      lastY: e.clientY,
      moved: false,
    }
    pagedDragInfoRef.current = info

    const onMove = (ev: PointerEvent) => {
      if (ev.pointerId !== info.pointerId) return
      info.lastX = ev.clientX
      info.lastY = ev.clientY
      if (!info.moved) {
        const dx = ev.clientX - info.startX
        const dy = ev.clientY - info.startY
        if (Math.hypot(dx, dy) < 6) return
        info.moved = true
        suppressIconClickRef.current = true
        setPagedDragSource({ id: info.appId, sourceSlot: info.sourceSlot })
      }
      // 浮动图标跟手 (直接改 transform,跳过 React)
      const node = floatingIconRef.current
      if (node) {
        node.style.transform = `translate3d(${ev.clientX}px, ${ev.clientY}px, 0)`
      }
      // 命中目标格 + 小圆点
      const el = document.elementFromPoint(ev.clientX, ev.clientY) as HTMLElement | null
      const slotEl = el?.closest('[data-slot]') as HTMLElement | null
      const slot = slotEl ? Number(slotEl.dataset.slot) : null
      setPagedHoverSlot((prev) => (prev !== slot ? slot : prev))
      detectDotHoverDuringDrag(el)
      detectEdgeDuringDrag(ev.clientX, ev.clientY)
    }

    const onUp = (ev: PointerEvent) => {
      if (ev.type !== 'pointercancel' && ev.pointerId !== info.pointerId) return
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('pointercancel', onUp)
      pagedDragInfoRef.current = null
      cancelEdgeFlip()
      cancelDotHover()

      if (!info.moved) {
        setPagedDragSource(null)
        setPagedHoverSlot(null)
        return // 当成点击,handleClick 会照常打开 URL
      }

      const el = document.elementFromPoint(ev.clientX, ev.clientY) as HTMLElement | null
      const slotEl = el?.closest('[data-slot]') as HTMLElement | null
      const targetSlot = slotEl ? Number(slotEl.dataset.slot) : null

      setPagedDragSource(null)
      setPagedHoverSlot(null)

      if (targetSlot === null || targetSlot === info.sourceSlot) return

      setConfig((prev) => {
        if (!prev) return prev
        const occupant = prev.apps.find((a) => a.id !== info.appId && a.pagedOrder === targetSlot)
        const newApps = prev.apps.map((a) => {
          if (a.id === info.appId) return { ...a, pagedOrder: targetSlot }
          if (occupant && a.id === occupant.id) return { ...a, pagedOrder: info.sourceSlot }
          return a
        })
        const positions = newApps.map((a) => ({ id: a.id, pagedOrder: a.pagedOrder }))
        api.setAppPositions(positions).catch(() => reload())
        return { ...prev, apps: newApps }
      })
    }

    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onUp)
  }

  // 浮动图标初始位置: 第一次出现时,直接放到当前光标位置
  useLayoutEffect(() => {
    if (!pagedDragSource) return
    const info = pagedDragInfoRef.current
    const node = floatingIconRef.current
    if (!info || !node) return
    node.style.transform = `translate3d(${info.lastX}px, ${info.lastY}px, 0)`
  }, [pagedDragSource?.id])

  const removeApp = async (id: string) => {
    if (!window.confirm('从主页移除该应用?')) return
    setConfig((prev) => (prev ? { ...prev, apps: prev.apps.filter((a) => a.id !== id) } : prev))
    try {
      await api.deleteApp(id)
    } catch {
      await reload()
    }
  }

  /* ============ 渲染 ============ */

  if (!config) {
    return (
      <main className="default-wallpaper grid min-h-screen place-items-center text-white">
        {loadErr ? (
          <div className="text-center">
            <div className="text-lg">加载失败</div>
            <div className="mt-2 text-sm opacity-80">{loadErr}</div>
          </div>
        ) : (
          <div className="text-sm opacity-80">加载中...</div>
        )}
      </main>
    )
  }

  return (
    <main
      className={`${showDefaultWallpaper ? 'default-wallpaper' : 'bg-transparent'} relative min-h-screen ${
        isScrollMode ? '' : 'overflow-hidden'
      } text-white`}
    >
      {wallpaperUrl && (
        <img
          src={wallpaperUrl}
          alt=""
          onLoad={() => {
            clearWallpaperFallbackTimer()
            setWallpaperLoaded(true)
          }}
          onError={() => {
            clearWallpaperFallbackTimer()
            setWallpaperLoaded(false)
            setWallpaperUrl(null)
          }}
          className={`fixed inset-0 h-full w-full object-cover transition-opacity duration-700 ${
            wallpaperLoaded ? 'opacity-100' : 'opacity-0'
          }`}
          style={{
            filter: `blur(${config.wallpaper.blur}px)`,
            transform: config.wallpaper.blur > 0 ? 'scale(1.04)' : 'scale(1)',
          }}
        />
      )}
      {wallpaperLoaded && <div className="fixed inset-0 bg-black/10" />}

      <section
        className={`relative mx-auto flex min-h-screen max-w-6xl flex-col px-6 py-14 transition-opacity duration-500 ${
          backgroundReady ? 'opacity-100' : 'opacity-0'
        }`}
      >
        <Clock showTime={config.showTime} title={config.title} />

        {/* 搜索框 */}
        <div
          className="relative mx-auto mt-10 h-16 w-full max-w-3xl cursor-text rounded-[1.75rem] bg-white px-5 text-slate-700 shadow-2xl"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) {
              event.preventDefault()
              searchInputRef.current?.focus()
            }
          }}
        >
          <div
            className="relative flex h-full items-center gap-4"
            onMouseDown={(event) => {
              if (event.target === event.currentTarget) {
                event.preventDefault()
                searchInputRef.current?.focus()
              }
            }}
          >
            <button
              type="button"
              onClick={() => {
                setShowEngines((v) => !v)
                setShowHistory(false)
              }}
              onBlur={() => window.setTimeout(() => setShowEngines(false), 140)}
              className="grid h-9 w-9 shrink-0 place-items-center overflow-hidden rounded-xl bg-transparent outline-none transition hover:bg-slate-100/70"
              title="切换搜索引擎"
            >
              {engine?.icon ? (
                <img src={iconSrc(engine.icon)} alt="" className="h-[72%] w-[72%] object-contain" />
              ) : (
                <div
                  className={`grid h-[72%] w-[72%] place-items-center rounded-lg bg-gradient-to-br ${
                    engine?.color || 'from-sky-400 to-blue-600'
                  } text-white`}
                >
                  <span className="text-[0.7em] font-black">{engine?.shortName || '?'}</span>
                </div>
              )}
            </button>
            <input
              ref={searchInputRef}
              autoComplete="off"
              autoCorrect="off"
              autoCapitalize="off"
              spellCheck={false}
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              onFocus={() => {
                setShowHistory(true)
                setShowEngines(false)
              }}
              onBlur={() => window.setTimeout(() => setShowHistory(false), 140)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') submitSearch()
              }}
              className="h-full w-full bg-transparent text-[1.25rem] leading-none text-slate-600 outline-none placeholder:text-slate-400"
              placeholder={`${engine?.name ?? ''} 搜索`}
            />
            <button
              type="button"
              onClick={() => submitSearch()}
              className="grid h-12 w-12 shrink-0 place-items-center rounded-xl bg-transparent text-slate-400 outline-none transition hover:bg-slate-100/70 hover:text-slate-600"
              title="搜索"
            >
              <Search className="h-[58%] w-[58%]" />
            </button>
          </div>

          {showEngines && (
            <div
              className="absolute left-0 right-0 top-[4.75rem] z-40 rounded-[2rem] bg-white px-8 py-7 shadow-2xl ring-1 ring-slate-200"
              onMouseDown={(e) => e.preventDefault()}
            >
              <div className="grid grid-cols-4 gap-x-5 gap-y-5 md:grid-cols-6 lg:grid-cols-8">
                {sortedEngines.map((item) => (
                  <SearchEngineIcon
                    key={item.id}
                    engine={item}
                    active={item.id === config.defaultSearchEngineId}
                    onClick={() => {
                      setDefaultEngine(item.id)
                      setShowEngines(false)
                    }}
                  />
                ))}
                <button
                  type="button"
                  onClick={() => {
                    setShowEngines(false)
                    setEngineForm({ open: true })
                  }}
                  className="group flex w-16 flex-col items-center gap-1.5 outline-none"
                >
                  <div className="grid h-14 w-14 place-items-center rounded-xl bg-slate-100 text-slate-500 transition duration-200 group-hover:-translate-y-1 group-hover:bg-slate-200">
                    <Plus className="h-6 w-6" />
                  </div>
                  <span className="text-xs text-slate-600">添加</span>
                </button>
              </div>
            </div>
          )}

          {showHistory && config.searchHistory.length > 0 && (
            <div className="absolute left-8 right-8 top-24 z-30 overflow-hidden rounded-2xl bg-white p-2 shadow-xl ring-1 ring-slate-200">
              <div className="flex items-center justify-between px-3 py-2 text-xs font-semibold uppercase tracking-wider text-slate-400">
                <span>历史搜索</span>
                <button
                  type="button"
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={clearHistory}
                  className="rounded-md px-2 py-1 text-[11px] font-medium normal-case tracking-normal text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
                >
                  清空
                </button>
              </div>
              {config.searchHistory.map((item) => (
                <button
                  key={item}
                  type="button"
                  onMouseDown={(e) => e.preventDefault()}
                  onClick={() => {
                    setQuery(item)
                    submitSearch(item)
                  }}
                  className="flex w-full items-center gap-3 rounded-xl px-3 py-2 text-left text-sm text-slate-600 transition hover:bg-slate-100"
                >
                  <Globe2 className="h-4 w-4 text-slate-400" />
                  {item}
                </button>
              ))}
            </div>
          )}
        </div>

        {/* 整页滚动模式 */}
        {isScrollMode && (
          <div className="mx-auto mt-10 w-full max-w-5xl px-6 pb-24">
            <div className="space-y-12 pt-6">
              {grouped.groups.map(({ category, apps }) => (
                <section
                  key={category.id}
                  onContextMenu={(e) => {
                    e.preventDefault()
                    setContextMenu({ kind: 'add', x: e.clientX, y: e.clientY, categoryId: category.id })
                  }}
                >
                  <h3 className="mb-4 px-2 text-sm font-semibold uppercase tracking-wider text-white/85 drop-shadow-[0_1px_2px_rgba(0,0,0,0.65)]">
                    {category.name}
                  </h3>
                  <div className="grid min-h-[5rem] grid-cols-8 content-start justify-items-center gap-x-3 gap-y-8 px-2">
                    {apps.map((app) => (
                      <AppIcon
                        key={app.id}
                        app={app}
                        prefer={networkPrefer}
                        draggable
                        isDragging={draggingAppId === app.id}
                        onDragStart={(e, a) => startAppDrag(e, a, category.id)}
                        onDragEnter={(e, a) => onDragEnterApp(e, a)}
                        onDragEnd={endAppDrag}
                        onContextMenu={(e, a) => {
                          e.preventDefault()
                          e.stopPropagation()
                          setContextMenu({ kind: 'app', x: e.clientX, y: e.clientY, app: a })
                        }}
                      />
                    ))}
                  </div>
                </section>
              ))}
              <section
                onContextMenu={(e) => {
                  e.preventDefault()
                  setContextMenu({ kind: 'add', x: e.clientX, y: e.clientY, categoryId: '' })
                }}
              >
                <h3 className="mb-4 px-2 text-sm font-semibold uppercase tracking-wider text-white/85 drop-shadow-[0_1px_2px_rgba(0,0,0,0.65)]">
                  未分类
                </h3>
                <div className="grid min-h-[5rem] grid-cols-8 content-start justify-items-center gap-x-3 gap-y-8 px-2">
                  {grouped.uncategorized.map((app) => (
                    <AppIcon
                      key={app.id}
                      app={app}
                      prefer={networkPrefer}
                      draggable
                      isDragging={draggingAppId === app.id}
                      onDragStart={(e, a) => startAppDrag(e, a, '')}
                      onDragEnter={(e, a) => onDragEnterApp(e, a)}
                      onDragEnd={endAppDrag}
                      onContextMenu={(e, a) => {
                        e.preventDefault()
                        e.stopPropagation()
                        setContextMenu({ kind: 'app', x: e.clientX, y: e.clientY, app: a })
                      }}
                    />
                  ))}
                </div>
              </section>
            </div>
          </div>
        )}
      </section>

      {/* 翻页模式应用层 */}
      {!isScrollMode && (
        <div
          className={`fixed inset-x-0 top-[17rem] bottom-24 z-20 transition-opacity duration-500 ${
            backgroundReady ? 'opacity-100' : 'opacity-0'
          }`}
        >
          <div ref={appsPagerRef} className="relative mx-auto h-full w-full max-w-5xl select-none overflow-hidden px-6">
            {appSlide && (
              <div
                key={`out-${appSlide.from}-${appSlide.axis}-${appSlide.dir}`}
                className={`absolute inset-x-6 inset-y-0 ${getSlideOutClass(appSlide.axis, appSlide.dir)}`}
              >
                <PagedGrid
                  page={appPages[appSlide.from] ?? []}
                  pageIndex={appSlide.from}
                  interactive={false}
                  draggingAppId={null}
                  hoverSlot={null}
                  networkPrefer={networkPrefer}
                  onContextMenu={(e, a) => {
                    e.preventDefault()
                    e.stopPropagation()
                    setContextMenu({ kind: 'app', x: e.clientX, y: e.clientY, app: a })
                  }}
                />
              </div>
            )}
            <div
              key={`in-${appPage}-${appSlide?.axis ?? 'static'}-${appSlide?.dir ?? 0}`}
              className={`absolute inset-x-6 inset-y-0 ${appSlide ? getSlideInClass(appSlide.axis, appSlide.dir) : ''}`}
            >
              <PagedGrid
                page={appPages[appPage] ?? []}
                pageIndex={appPage}
                interactive={!appSlide}
                draggingAppId={pagedDragSource?.id ?? null}
                hoverSlot={pagedHoverSlot}
                networkPrefer={networkPrefer}
                onContextMenu={(e, a) => {
                  e.preventDefault()
                  e.stopPropagation()
                  setContextMenu({ kind: 'app', x: e.clientX, y: e.clientY, app: a })
                }}
                onIconPointerDown={handleIconPointerDown}
                suppressClickRef={suppressIconClickRef}
                onEmptyContextMenu={(e, absPO) =>
                  setContextMenu({ kind: 'add', x: e.clientX, y: e.clientY, pagedOrder: absPO })
                }
              />
            </div>
          </div>
        </div>
      )}

      {/* 跟手浮动图标 */}
      {pagedDragSource && (() => {
        const app = config.apps.find((a) => a.id === pagedDragSource.id)
        if (!app) return null
        return (
          <div
            ref={floatingIconRef}
            className="pointer-events-none fixed left-0 top-0 z-50 will-change-transform"
          >
            <div
              className="flex w-24 flex-col items-center gap-2 text-white"
              style={{ transform: 'translate(-50%, -50%) scale(1.12)', transformOrigin: 'center' }}
            >
              <div className="relative grid h-16 w-16 place-items-center overflow-hidden rounded-2xl shadow-2xl ring-2 ring-white/50">
                <IconPreview icon={app.icon} fallback={app.name} color={app.color} className="h-full w-full" />
              </div>
              <span className="max-w-24 truncate text-sm font-medium drop-shadow-[0_1px_2px_rgba(0,0,0,0.9)]">{app.name}</span>
            </div>
          </div>
        )
      })()}

      {/* 翻页指示器 */}
      {!isScrollMode &&
        (layoutMode === 'paged-horizontal' ? (
          <div className="fixed bottom-7 left-1/2 z-30 flex -translate-x-1/2 flex-row items-center gap-2.5">
            {appPages.map((_, index) => {
              const active = index === appPage
              return (
                <button
                  key={index}
                  type="button"
                  data-dot-index={index}
                  onClick={() => goToAppPage(index, 'x')}
                  className={`h-2 rounded-full bg-white/55 shadow-md transition-all duration-300 hover:bg-white ${
                    active ? 'w-6 bg-white' : 'w-2'
                  }`}
                  title={`第 ${index + 1} 页`}
                />
              )
            })}
            <button
              type="button"
              onClick={addNewPage}
              className="ml-1 grid h-5 w-5 place-items-center rounded-full bg-white/30 text-white shadow-md ring-1 ring-white/40 backdrop-blur-md transition hover:bg-white/55"
              title="新建页面"
            >
              <Plus className="h-3 w-3" />
            </button>
          </div>
        ) : (
          <div className="fixed left-3 top-1/2 z-30 flex -translate-y-1/2 flex-col items-center gap-2.5">
            {appPages.map((_, index) => {
              const active = index === appPage
              return (
                <button
                  key={index}
                  type="button"
                  data-dot-index={index}
                  onClick={() => goToAppPage(index, allowVertical ? 'y' : 'x')}
                  className={`w-2 rounded-full bg-white/55 shadow-md transition-all duration-300 hover:bg-white ${
                    active ? 'h-6 bg-white' : 'h-2'
                  }`}
                  title={`第 ${index + 1} 页`}
                />
              )
            })}
            <button
              type="button"
              onClick={addNewPage}
              className="mt-1 grid h-5 w-5 place-items-center rounded-full bg-white/30 text-white shadow-md ring-1 ring-white/40 backdrop-blur-md transition hover:bg-white/55"
              title="新建页面"
            >
              <Plus className="h-3 w-3" />
            </button>
          </div>
        ))}

      {/* 右键菜单 */}
      {contextMenu && (
        <div
          className="fixed z-50 min-w-32 overflow-hidden rounded-xl bg-white text-sm text-slate-700 shadow-2xl ring-1 ring-slate-200"
          style={{ left: contextMenu.x, top: contextMenu.y }}
          onClick={(e) => e.stopPropagation()}
        >
          {contextMenu.kind === 'app' ? (
            <>
              <button
                type="button"
                onClick={() => {
                  const app = contextMenu.app
                  setContextMenu(null)
                  setAppForm({ open: true, initial: app })
                }}
                className="flex w-full items-center gap-2 px-3 py-2 text-left transition hover:bg-slate-100"
              >
                <Pencil className="h-4 w-4 text-slate-400" />
                编辑
              </button>
              <button
                type="button"
                onClick={() => {
                  const app = contextMenu.app
                  setContextMenu(null)
                  removeApp(app.id)
                }}
                className="flex w-full items-center gap-2 px-3 py-2 text-left text-rose-500 transition hover:bg-rose-50"
              >
                <Trash2 className="h-4 w-4" />
                删除
              </button>
            </>
          ) : (
            <button
              type="button"
              onClick={() => {
                if (contextMenu.pagedOrder !== undefined) {
                  pendingAddSlotRef.current = contextMenu.pagedOrder
                }
                const defaultCategoryId = contextMenu.categoryId
                setContextMenu(null)
                setAppForm({ open: true, defaultCategoryId })
              }}
              className="flex w-full items-center gap-2 px-3 py-2 text-left transition hover:bg-slate-100"
            >
              <Plus className="h-4 w-4 text-slate-400" />
              添加
            </button>
          )}
        </div>
      )}

      {/* 设置模态 */}
      {showSettings && (
        <div
          className="fixed inset-0 z-40 grid place-items-center bg-black/40 px-4 py-6 backdrop-blur-sm"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget) setShowSettings(false)
          }}
        >
          <div className="flex h-[32rem] w-full max-w-3xl overflow-hidden rounded-3xl bg-white text-slate-700 shadow-2xl ring-1 ring-slate-200">
            <aside className="flex w-48 shrink-0 flex-col gap-1 border-r border-slate-100 bg-slate-50/80 p-4">
              <div className="mb-3 px-2 text-base font-bold text-slate-800">Home 设置</div>
              {(
                [
                  { id: 'appearance' as const, name: '外观', icon: ImageIcon },
                  { id: 'layout' as const, name: '布局', icon: LayoutGrid },
                  { id: 'categories' as const, name: '分类', icon: Tags },
                  { id: 'search' as const, name: '搜索', icon: Search },
                ]
              ).map((tab) => {
                const Icon = tab.icon
                const active = settingsTab === tab.id
                return (
                  <button
                    key={tab.id}
                    type="button"
                    onClick={() => setSettingsTab(tab.id)}
                    className={`flex items-center gap-2.5 rounded-xl px-3 py-2 text-sm font-medium transition ${
                      active ? 'bg-blue-500 text-white shadow-md shadow-blue-500/20' : 'text-slate-600 hover:bg-slate-200/60'
                    }`}
                  >
                    <Icon className="h-4 w-4" />
                    {tab.name}
                  </button>
                )
              })}
            </aside>

            <div className="relative flex-1 overflow-y-auto p-6">
              <button
                type="button"
                onClick={() => setShowSettings(false)}
                className="absolute right-4 top-4 grid h-8 w-8 place-items-center rounded-full text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
                title="关闭"
              >
                <X className="h-4 w-4" />
              </button>

              {settingsTab === 'appearance' && (
                <div className="space-y-6">
                  <div>
                    <div className="text-base font-bold text-slate-800">外观</div>
                    <div className="mt-1 text-sm text-slate-500">调整主页背景</div>
                  </div>

                  <div>
                    <div className="mb-2 text-sm font-semibold text-slate-600">背景</div>
                    <div className="grid grid-cols-2 gap-2">
                      {(['bing', 'default'] as WallpaperMode[]).map((m) => (
                        <button
                          key={m}
                          type="button"
                          onClick={() => setWallpaperMode(m)}
                          className={`rounded-2xl px-4 py-3 text-sm font-semibold transition ${
                            config.wallpaper.mode === m
                              ? 'bg-blue-500 text-white shadow-lg shadow-blue-500/25'
                              : 'bg-slate-100 text-slate-600 hover:bg-slate-200'
                          }`}
                        >
                          {m === 'bing' ? 'Bing 壁纸' : '默认背景'}
                        </button>
                      ))}
                    </div>
                  </div>

                  <div>
                    <div className="mb-2 text-sm font-semibold text-slate-600">清晰度</div>
                    <div className="grid grid-cols-2 gap-2">
                      {(['1080', 'uhd'] as WallpaperResolution[]).map((r) => (
                        <button
                          key={r}
                          type="button"
                          onClick={() => setWallpaperResolution(r)}
                          disabled={config.wallpaper.mode === 'default'}
                          className={`rounded-2xl px-4 py-3 text-sm font-semibold transition disabled:cursor-not-allowed disabled:opacity-45 ${
                            config.wallpaper.resolution === r
                              ? 'bg-slate-800 text-white shadow-lg shadow-slate-800/20'
                              : 'bg-slate-100 text-slate-600 hover:bg-slate-200'
                          }`}
                        >
                          {r === '1080' ? '1080P' : '4K / UHD'}
                        </button>
                      ))}
                    </div>
                  </div>

                  <label className="block">
                    <div className="mb-2 flex items-center justify-between text-sm font-semibold text-slate-600">
                      <span>模糊度</span>
                      <span className="text-slate-400">{config.wallpaper.blur}px</span>
                    </div>
                    <input
                      type="range"
                      min={0}
                      max={12}
                      step={1}
                      value={config.wallpaper.blur}
                      disabled={config.wallpaper.mode === 'default'}
                      onChange={(e) => setWallpaperBlur(Number(e.target.value))}
                      className="w-full accent-blue-500 disabled:opacity-45"
                    />
                  </label>
                </div>
              )}

              {settingsTab === 'layout' && (
                <div className="space-y-6">
                  <div>
                    <div className="text-base font-bold text-slate-800">布局</div>
                    <div className="mt-1 text-sm text-slate-500">控制应用页面的滑动方向</div>
                  </div>

                  <div className="grid grid-cols-2 gap-2">
                    {(
                      [
                        { id: 'paged-horizontal' as LayoutMode, name: '左右翻页', desc: '仅允许左右滑动切换' },
                        { id: 'paged-vertical' as LayoutMode, name: '上下翻页', desc: '仅允许上下滑动切换' },
                        { id: 'paged-free' as LayoutMode, name: '自由翻页', desc: '上下/左右都可切换' },
                        { id: 'scroll' as LayoutMode, name: '整页滚动', desc: '所有应用按分类纵向滚动' },
                      ]
                    ).map((mode) => {
                      const active = config.layoutMode === mode.id
                      return (
                        <button
                          key={mode.id}
                          type="button"
                          onClick={() => setLayoutMode(mode.id)}
                          className={`rounded-2xl px-4 py-3 text-left transition ${
                            active ? 'bg-blue-500 text-white shadow-lg shadow-blue-500/25' : 'bg-slate-100 text-slate-600 hover:bg-slate-200'
                          }`}
                        >
                          <div className="text-sm font-semibold">{mode.name}</div>
                          <div className={`mt-0.5 text-xs ${active ? 'text-white/85' : 'text-slate-400'}`}>{mode.desc}</div>
                        </button>
                      )
                    })}
                  </div>
                </div>
              )}

              {settingsTab === 'categories' && (
                <div className="space-y-5">
                  <div>
                    <div className="text-base font-bold text-slate-800">分类</div>
                    <div className="mt-1 text-sm text-slate-500">拖拽应用图标到对应分类，仅在「整页滚动」布局下分组显示</div>
                  </div>

                  <button
                    type="button"
                    onClick={newCategory}
                    className="flex items-center gap-2 rounded-xl bg-slate-100 px-3 py-2 text-sm font-semibold text-slate-600 transition hover:bg-slate-200"
                  >
                    <Plus className="h-4 w-4" />
                    新建分类
                  </button>

                  <div className="space-y-3">
                    {config.categories.map((cat) => {
                      const apps = config.apps.filter((app) => app.categoryId === cat.id)
                      const isOver = dragOverCategoryId === cat.id
                      const isEditing = editingCategoryId === cat.id
                      return (
                        <div
                          key={cat.id}
                          onDragOver={(e) => {
                            e.preventDefault()
                            setDragOverCategoryId(cat.id)
                          }}
                          onDragLeave={() => setDragOverCategoryId((p) => (p === cat.id ? null : p))}
                          onDrop={(e) => {
                            e.preventDefault()
                            const appId = e.dataTransfer.getData('text/app-id')
                            setDragOverCategoryId(null)
                            if (appId) assignAppToCategory(appId, cat.id)
                          }}
                          className={`rounded-2xl border-2 border-dashed p-3 transition ${
                            isOver ? 'border-blue-400 bg-blue-50' : 'border-slate-200 bg-slate-50/60'
                          }`}
                        >
                          <div className="mb-2 flex items-center justify-between gap-2">
                            {isEditing ? (
                              <input
                                autoFocus
                                value={editingCategoryName}
                                onChange={(e) => setEditingCategoryName(e.target.value)}
                                onBlur={() => {
                                  renameCategory(cat.id, editingCategoryName)
                                  setEditingCategoryId(null)
                                }}
                                onKeyDown={(e) => {
                                  if (e.key === 'Enter') {
                                    renameCategory(cat.id, editingCategoryName)
                                    setEditingCategoryId(null)
                                  } else if (e.key === 'Escape') {
                                    setEditingCategoryId(null)
                                  }
                                }}
                                className="flex-1 rounded-lg border border-slate-200 bg-white px-2 py-1 text-sm text-slate-700 outline-none focus:border-blue-400"
                              />
                            ) : (
                              <div className="flex flex-1 items-center gap-2">
                                <span className="text-sm font-semibold text-slate-700">{cat.name}</span>
                                <span className="text-xs text-slate-400">{apps.length}</span>
                              </div>
                            )}
                            <button
                              type="button"
                              onClick={() => {
                                setEditingCategoryId(cat.id)
                                setEditingCategoryName(cat.name)
                              }}
                              className="grid h-7 w-7 place-items-center rounded-lg text-slate-400 transition hover:bg-slate-200 hover:text-slate-600"
                              title="重命名"
                            >
                              <Pencil className="h-3.5 w-3.5" />
                            </button>
                            <button
                              type="button"
                              onClick={() => removeCategory(cat.id)}
                              className="grid h-7 w-7 place-items-center rounded-lg text-slate-400 transition hover:bg-rose-100 hover:text-rose-600"
                              title="删除分类"
                            >
                              <Trash2 className="h-3.5 w-3.5" />
                            </button>
                          </div>
                          <div className="flex flex-wrap gap-2">
                            {apps.length === 0 ? (
                              <span className="px-1 py-2 text-xs text-slate-400">拖拽应用到此处</span>
                            ) : (
                              apps.map((app) => (
                                <div
                                  key={app.id}
                                  draggable
                                  onDragStart={(e) => {
                                    e.dataTransfer.setData('text/app-id', app.id)
                                    e.dataTransfer.effectAllowed = 'move'
                                  }}
                                  className="flex cursor-grab items-center gap-1.5 rounded-xl bg-white px-2.5 py-1.5 text-xs font-medium text-slate-600 shadow-sm ring-1 ring-slate-200 transition hover:shadow-md active:cursor-grabbing"
                                >
                                  <span
                                    className={`grid h-5 w-5 place-items-center overflow-hidden rounded-md bg-gradient-to-br ${
                                      app.color || 'from-slate-400 to-slate-600'
                                    } text-white`}
                                  >
                                    {app.icon ? (
                                      <img src={iconSrc(app.icon)} alt="" className="h-full w-full object-contain" />
                                    ) : (
                                      <span className="text-[10px] font-black">{app.name.charAt(0)}</span>
                                    )}
                                  </span>
                                  {app.name}
                                </div>
                              ))
                            )}
                          </div>
                        </div>
                      )
                    })}

                    <div
                      onDragOver={(e) => {
                        e.preventDefault()
                        setDragOverCategoryId('__uncategorized__')
                      }}
                      onDragLeave={() => setDragOverCategoryId((p) => (p === '__uncategorized__' ? null : p))}
                      onDrop={(e) => {
                        e.preventDefault()
                        const appId = e.dataTransfer.getData('text/app-id')
                        setDragOverCategoryId(null)
                        if (appId) assignAppToCategory(appId, '')
                      }}
                      className={`rounded-2xl border-2 border-dashed p-3 transition ${
                        dragOverCategoryId === '__uncategorized__' ? 'border-blue-400 bg-blue-50' : 'border-slate-200 bg-slate-50/60'
                      }`}
                    >
                      <div className="mb-2 flex items-center gap-2">
                        <span className="text-sm font-semibold text-slate-700">未分类</span>
                        <span className="text-xs text-slate-400">{grouped.uncategorized.length}</span>
                      </div>
                      <div className="flex flex-wrap gap-2">
                        {grouped.uncategorized.length === 0 ? (
                          <span className="px-1 py-2 text-xs text-slate-400">拖拽应用到此处可移除分类</span>
                        ) : (
                          grouped.uncategorized.map((app) => (
                            <div
                              key={app.id}
                              draggable
                              onDragStart={(e) => {
                                e.dataTransfer.setData('text/app-id', app.id)
                                e.dataTransfer.effectAllowed = 'move'
                              }}
                              className="flex cursor-grab items-center gap-1.5 rounded-xl bg-white px-2.5 py-1.5 text-xs font-medium text-slate-600 shadow-sm ring-1 ring-slate-200 transition hover:shadow-md active:cursor-grabbing"
                            >
                              <span
                                className={`grid h-5 w-5 place-items-center overflow-hidden rounded-md bg-gradient-to-br ${
                                  app.color || 'from-slate-400 to-slate-600'
                                } text-white`}
                              >
                                {app.icon ? (
                                  <img src={iconSrc(app.icon)} alt="" className="h-full w-full object-contain" />
                                ) : (
                                  <span className="text-[10px] font-black">{app.name.charAt(0)}</span>
                                )}
                              </span>
                              {app.name}
                            </div>
                          ))
                        )}
                      </div>
                    </div>
                  </div>
                </div>
              )}

              {settingsTab === 'search' && (
                <div className="space-y-6">
                  <div>
                    <div className="text-base font-bold text-slate-800">搜索</div>
                    <div className="mt-1 text-sm text-slate-500">管理搜索引擎与默认引擎</div>
                  </div>

                  <div>
                    <div className="mb-2 flex items-center justify-between text-sm font-semibold text-slate-600">
                      <span>搜索引擎</span>
                      <button
                        type="button"
                        onClick={() => setEngineForm({ open: true })}
                        className="rounded-md bg-slate-100 px-2 py-1 text-xs font-medium text-slate-600 transition hover:bg-slate-200"
                      >
                        + 添加
                      </button>
                    </div>
                    <div className="space-y-2">
                      {sortedEngines.map((item) => {
                        const isDefault = item.id === config.defaultSearchEngineId
                        return (
                          <div
                            key={item.id}
                            className="flex items-center gap-3 rounded-xl bg-slate-50 px-3 py-2"
                          >
                            <div className="grid h-9 w-9 shrink-0 place-items-center overflow-hidden rounded-lg bg-white ring-1 ring-slate-200">
                              {item.icon ? (
                                <img src={iconSrc(item.icon)} alt="" className="h-7 w-7 object-contain" />
                              ) : (
                                <div className={`grid h-full w-full place-items-center rounded-lg bg-gradient-to-br ${item.color || 'from-slate-400 to-slate-600'} text-white`}>
                                  <span className="text-sm font-black tracking-tight">{(item.shortName || item.name).charAt(0).toUpperCase()}</span>
                                </div>
                              )}
                            </div>
                            <div className="min-w-0 flex-1">
                              <div className="truncate text-sm font-semibold text-slate-700">{item.name}</div>
                              <div className="truncate text-xs text-slate-400">{item.url}</div>
                            </div>
                            <button
                              type="button"
                              onClick={() => setDefaultEngine(item.id)}
                              className={`rounded-md px-2 py-1 text-xs font-medium transition ${
                                isDefault
                                  ? 'bg-blue-500 text-white shadow-sm shadow-blue-500/20'
                                  : 'text-slate-500 hover:bg-slate-200'
                              }`}
                            >
                              {isDefault ? '默认' : '设为默认'}
                            </button>
                            <button
                              type="button"
                              onClick={() => setEngineForm({ open: true, initial: item })}
                              className="grid h-7 w-7 place-items-center rounded-md text-slate-400 transition hover:bg-slate-200 hover:text-slate-600"
                              title="编辑"
                            >
                              <Pencil className="h-3.5 w-3.5" />
                            </button>
                            <button
                              type="button"
                              onClick={() => removeEngine(item.id)}
                              className="grid h-7 w-7 place-items-center rounded-md text-slate-400 transition hover:bg-rose-100 hover:text-rose-600"
                              title="删除"
                            >
                              <Trash2 className="h-3.5 w-3.5" />
                            </button>
                          </div>
                        )
                      })}
                    </div>
                  </div>

                  <div>
                    <div className="mb-2 flex items-center justify-between text-sm font-semibold text-slate-600">
                      <span>搜索历史</span>
                      <span className="text-xs font-normal text-slate-400">已保存 {config.searchHistory.length} 条</span>
                    </div>
                    <button
                      type="button"
                      onClick={clearHistory}
                      disabled={config.searchHistory.length === 0}
                      className="rounded-2xl bg-slate-100 px-4 py-2.5 text-sm font-semibold text-slate-600 transition hover:bg-rose-100 hover:text-rose-600 disabled:cursor-not-allowed disabled:opacity-45 disabled:hover:bg-slate-100 disabled:hover:text-slate-600"
                    >
                      清空搜索历史
                    </button>
                  </div>
                </div>
              )}
            </div>
          </div>
        </div>
      )}

      {/* 右下浮动按钮组 */}
      <div className="fixed bottom-7 right-7 z-30 flex flex-col gap-3">
        <button
          type="button"
          onClick={cycleNetworkPrefer}
          className="grid h-11 w-11 place-items-center rounded-full bg-white/15 text-white shadow-lg ring-1 ring-white/20 backdrop-blur-md transition hover:bg-white/25"
          title={`当前: ${networkPreferLabel[networkPrefer]}, 点击切换`}
        >
          <HomeIcon className="h-5 w-5" />
        </button>
        <button
          type="button"
          onClick={() => setShowSettings((v) => !v)}
          className="grid h-11 w-11 place-items-center rounded-full bg-white/15 text-white shadow-lg ring-1 ring-white/20 backdrop-blur-md transition hover:bg-white/25"
          title="Home 设置"
        >
          <Settings className="h-5 w-5" />
        </button>
        <a
          href="/linkstar/"
          className="grid h-11 w-11 place-items-center rounded-full bg-white/15 text-white shadow-lg ring-1 ring-white/20 backdrop-blur-md transition hover:bg-white/25"
          title="打开后台"
        >
          <ExternalLink className="h-5 w-5" />
        </a>
      </div>

      {/* 弹窗 */}
      {appForm.open && (
        <AppFormModal
          initial={appForm.initial}
          defaultCategoryId={appForm.defaultCategoryId}
          categories={config.categories}
          isStun={appForm.initial?.type === 'stun'}
          onCancel={() => setAppForm({ open: false })}
          onSubmit={submitApp}
        />
      )}
      {engineForm.open && (
        <SearchEngineFormModal
          initial={engineForm.initial}
          onCancel={() => setEngineForm({ open: false })}
          onSubmit={submitEngine}
        />
      )}
    </main>
  )
}

export default App
