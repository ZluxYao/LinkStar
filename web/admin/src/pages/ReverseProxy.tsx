import { useCallback, useEffect, useMemo, useRef, useState, type ReactNode } from 'react'
import {
  AlertCircle,
  ArrowRight,
  ExternalLink,
  Globe,
  HardDrive,
  Pencil,
  Plus,
  Settings2,
  ShieldCheck,
  TriangleAlert,
  Trash2,
  X,
  Zap,
} from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import { modalBackdrop } from '../components/modal'
import * as api from '../lib/api'
import type { Certificate, ProxyConfig, ProxyEntry, ProxyListener, ProxySite } from '../types'

// ===================== 小工具 =====================

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
    window.setTimeout(() => setToasts((p) => p.filter((t) => t.id !== id)), 3200)
  }, [])
  return { toasts, show }
}

const errText = (e: unknown) => (e instanceof Error ? e.message : String(e))

/** 80 和 443 不用写出来，写了反而显得脏 */
export const portText = (https: boolean, port: number) => ((https ? port === 443 : port === 80) ? '' : `:${port}`)

/**
 * 浏览器自己写死不让连的端口，抄自 Chromium 的 net::kRestrictedPorts
 * （net/base/port_util.cc），Firefox 那份几乎一模一样。
 *
 * 全是上古协议的端口（IRC 6665-6669、NFS 2049、SMTP 25……），历史上被拿来
 * 做跨协议攻击，从 2000 年代禁到现在。这件事阴在：反代监听这些端口不会报错，
 * 后端探测是通的，curl 也拿得到 200，唯独浏览器甩一句 ERR_UNSAFE_PORT，
 * 配的人完全不知道是谁的问题。所以在填的时候就要说出来。
 *
 * 不含 0——这里 0 表示「留空」，不是真端口。
 */
const BROWSER_BLOCKED_PORTS = new Set([
  1, 7, 9, 11, 13, 15, 17, 19, 20, 21, 22, 23, 25, 37, 42, 43, 53, 69, 77, 79, 87, 95, 101, 102,
  103, 104, 109, 110, 111, 113, 115, 117, 119, 123, 135, 137, 139, 143, 161, 179, 389, 427, 465,
  512, 513, 514, 515, 526, 530, 531, 532, 540, 548, 554, 556, 563, 587, 601, 636, 989, 990, 993,
  995, 1719, 1720, 1723, 2049, 3659, 4045, 5060, 5061, 6000, 6566, 6665, 6666, 6667, 6668, 6669,
  6697, 10080,
])

const portBlocked = (port: number) => port > 0 && BROWSER_BLOCKED_PORTS.has(port)

/**
 * 「按域名自动匹配」在客户端没发 SNI 的时候还能不能挑出一张证书。
 *
 * 逐条对着 modules/cert.Manager.Lookup 的第 ④⑤ 步：先找默认证书，
 * 再看是不是只有一张可用的。两条都不满足，握手就地失败——浏览器那头
 * 只有一句 ERR_SSL_PROTOCOL_ERROR，什么都看不出来，所以得提前说。
 *
 * 敲 IP 的地址栏是不发 SNI 的（RFC 6066 只允许域名），所以「没填域名的
 * HTTPS 站点」正好撞在这个坑上。
 */
const autoCertNeedsSNI = (certs: Certificate[]) => {
  const usable = certs.filter((c) => c.enabled && c.loaded)
  return !usable.some((c) => c.isDefault) && usable.length !== 1
}

/** 淡黄提示条。只是提醒，不拦保存——挡不挡得住由用户自己判断 */
function Warn({ children }: { children: ReactNode }) {
  return (
    <div className="flex gap-2 rounded-xl bg-amber-50 px-3 py-2.5 text-[11px] leading-relaxed text-amber-700 ring-1 ring-amber-200">
      <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
      <span>{children}</span>
    </div>
  )
}

/** 端口落在浏览器禁用名单上时的说辞，两个表单共用 */
function BlockedPortWarn({ label, port }: { label: string; port: number }) {
  return (
    <Warn>
      {label} <span className="font-mono font-semibold">{port}</span>{' '}
      在浏览器的禁用端口名单里，Chrome / Firefox 会直接拒连，报{' '}
      <span className="font-mono">ERR_UNSAFE_PORT</span>。反代照样监听得好好的、
      <span className="font-mono">curl</span> 也能通，只有浏览器进不来——换一个端口
    </Warn>
  )
}

/**
 * 站点对外的地址：自己占端口就用那个，否则挂在默认入口上。
 *
 * 挂默认入口且勾了 HTTPS 时，HTTP 入口上其实也有一份——
 * 但用户真正会用的是 HTTPS 那个，这里就显示它。
 */
export function siteAddr(entry: ProxyEntry, site: ProxySite): { https: boolean; port: number } | null {
  if (site.listenPort > 0) return { https: site.https, port: site.listenPort }
  if (site.https && entry.httpsPort > 0) return { https: true, port: entry.httpsPort }
  if (entry.httpPort > 0) return { https: false, port: entry.httpPort }
  return null
}

/** 可点的访问链接，用第一个域名。通配域名拼不出具体地址，返回空 */
export function siteURL(entry: ProxyEntry, site: ProxySite): string {
  const addr = siteAddr(entry, site)
  const host = site.hosts?.[0] ?? ''
  if (!addr || !host || host.startsWith('*')) return ''
  return `${addr.https ? 'https' : 'http'}://${host}${portText(addr.https, addr.port)}${site.pathPrefix}`
}

// 不带宽度，留给调用方定。想窄的那几个（代理地址前面的 http/https 下拉）
// 不能写成 `${inputCls} w-24`——Tailwind 的 w-full 在样式表里排在 w-24 后面，
// 类名写在后面也不管用，只会把整行撑满、把后面的输入框顶出弹窗。
const fieldCls =
  'rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400'
const inputCls = `w-full ${fieldCls}`
const labelCls = 'mb-1 text-xs font-semibold text-slate-500'

function Modal({
  title,
  onCancel,
  children,
  footer,
}: {
  title: string
  onCancel: () => void
  children: React.ReactNode
  footer: React.ReactNode
}) {
  return (
    <div
      className={modalBackdrop}
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="max-h-full w-full max-w-lg overflow-y-auto rounded-2xl bg-white p-6 text-slate-700 shadow-2xl ring-1 ring-slate-200">
        <div className="mb-5 flex items-center justify-between">
          <div className="text-base font-bold text-slate-800">{title}</div>
          <button
            type="button"
            onClick={onCancel}
            className="grid h-7 w-7 place-items-center rounded-full text-slate-400 transition hover:bg-slate-100 hover:text-slate-600"
          >
            <X className="h-4 w-4" />
          </button>
        </div>
        <div className="space-y-3">{children}</div>
        <div className="mt-6 flex justify-end gap-2">{footer}</div>
      </div>
    </div>
  )
}

function CertSelect({
  value,
  certs,
  onChange,
}: {
  value: string
  certs: Certificate[]
  onChange: (v: string) => void
}) {
  return (
    <select value={value} onChange={(e) => onChange(e.target.value)} className={inputCls}>
      <option value="0">按域名自动匹配</option>
      {certs.map((c) => (
        <option key={c.id} value={String(c.id)}>
          {c.name}
          {c.domains.length > 0 ? `（${c.domains.join('、')}）` : ''}
        </option>
      ))}
    </select>
  )
}

// ===================== 默认入口设置 =====================

function EntryModal({
  entry,
  certs,
  onCancel,
  onSubmit,
}: {
  entry: ProxyEntry
  certs: Certificate[]
  onCancel: () => void
  onSubmit: (payload: Omit<api.EntryPayload, 'enabled'>) => Promise<void>
}) {
  const [httpPort, setHttpPort] = useState(entry.httpPort ? String(entry.httpPort) : '')
  const [httpsPort, setHttpsPort] = useState(entry.httpsPort ? String(entry.httpsPort) : '')
  const [certId, setCertId] = useState(String(entry.certId || 0))
  // ?? ''：老后端不回这个字段，少了它下面每个 .trim() 都会炸
  const [baseDomain, setBaseDomain] = useState(entry.baseDomain ?? '')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const httpsOn = (Number(httpsPort) || 0) > 0

  const submit = async () => {
    setErr('')
    setBusy(true)
    try {
      await onSubmit({
        httpPort: Number(httpPort) || 0,
        httpsPort: Number(httpsPort) || 0,
        certId: httpsOn ? Number(certId) || 0 : 0,
        baseDomain: baseDomain.trim(),
      })
    } catch (e) {
      setErr(errText(e))
      setBusy(false)
    }
  }

  return (
    <Modal
      title="默认入口"
      onCancel={onCancel}
      footer={
        <>
          <button
            type="button"
            onClick={onCancel}
            className="rounded-xl px-4 py-2 text-sm text-slate-500 transition hover:bg-slate-100"
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
        </>
      }
    >
      <div className="rounded-xl bg-slate-50 px-3 py-2.5 text-[11px] leading-relaxed text-slate-500 ring-1 ring-slate-200/70">
        没单独指定端口的站点都挂在这两个端口上，靠域名区分——
        就是 nginx 里一堆 server 块共用 <span className="font-mono">listen 80</span> /{' '}
        <span className="font-mono">listen 443 ssl</span> 的那个意思
      </div>

      <div className="grid grid-cols-2 gap-3">
        <label className="block">
          <div className={labelCls}>HTTP 端口</div>
          <input
            autoFocus
            value={httpPort}
            onChange={(e) => setHttpPort(e.target.value)}
            placeholder="80"
            className={`${inputCls} font-mono`}
          />
        </label>
        <label className="block">
          <div className={labelCls}>HTTPS 端口</div>
          <input
            value={httpsPort}
            onChange={(e) => setHttpsPort(e.target.value)}
            placeholder="443"
            className={`${inputCls} font-mono`}
          />
        </label>
      </div>
      <div className="-mt-1 text-[11px] leading-relaxed text-slate-400">
        留空表示不开这个入口，两个都留空就没入口了。1024 以下的端口要管理员权限
      </div>

      {portBlocked(Number(httpPort)) && <BlockedPortWarn label="HTTP 端口" port={Number(httpPort)} />}
      {portBlocked(Number(httpsPort)) && <BlockedPortWarn label="HTTPS 端口" port={Number(httpsPort)} />}

      {httpsOn && (
        <label className="block">
          <div className={labelCls}>默认证书</div>
          <CertSelect value={certId} certs={certs} onChange={setCertId} />
          <div className="mt-1 text-[11px] leading-relaxed text-slate-400">
            自动匹配会按访问的域名挑证书。指定一张就是所有站点都用它，证书得盖住全部域名——
            <span className="font-mono">*.{baseDomain.trim() || 'example.com'}</span> 这种通配最省事
          </div>
        </label>
      )}

      <label className="block">
        <div className={labelCls}>主域名（可选）</div>
        <input
          value={baseDomain}
          onChange={(e) => setBaseDomain(e.target.value)}
          placeholder="example.com"
          className={`${inputCls} font-mono`}
        />
        <div className="mt-1 text-[11px] leading-relaxed text-slate-400">
          填了之后加站点只写前缀：<span className="font-mono">nas</span> 就是{' '}
          <span className="font-mono">nas.{baseDomain.trim() || 'example.com'}</span>。写全域名的照样认
        </div>
      </label>

      {err && <div className="rounded-xl bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>}
    </Modal>
  )
}

// ===================== 站点 =====================

interface SiteFormState {
  hosts: string[]
  pathPrefix: string
  stripPrefix: boolean
  backend: string
  backendHttps: boolean
  listenPort: string
  https: boolean
  certId: string
  enabled: boolean
  description: string
}

function SiteModal({
  initial,
  entry,
  certs,
  onCancel,
  onSubmit,
}: {
  initial?: ProxySite
  entry: ProxyEntry
  certs: Certificate[]
  onCancel: () => void
  onSubmit: (payload: api.ProxySitePayload) => Promise<void>
}) {
  const [form, setForm] = useState<SiteFormState>(() => ({
    hosts: initial?.hosts?.length ? [...initial.hosts] : [''],
    pathPrefix: initial?.pathPrefix ?? '',
    stripPrefix: initial?.stripPrefix ?? false,
    backend: initial?.backend ?? '',
    backendHttps: initial?.backendHttps ?? false,
    listenPort: initial?.listenPort ? String(initial.listenPort) : '',
    https: initial?.https ?? false,
    certId: String(initial?.certId ?? 0),
    enabled: initial?.enabled ?? true,
    description: initial?.description ?? '',
  }))
  const [busy, setBusy] = useState(false)
  const [testing, setTesting] = useState(false)
  const [probe, setProbe] = useState<{ ok: boolean; text: string } | null>(null)
  const [err, setErr] = useState('')

  const baseDomain = entry.baseDomain ?? ''
  // 要补的那截主域名，判据和后端 expandHost 一致：没有点才补
  const suffixOf = (h: string) => {
    const t = h.trim()
    return baseDomain && t && !t.includes('.') ? `.${baseDomain}` : ''
  }
  const fullHost = (h: string) => h.trim().toLowerCase() + suffixOf(h)
  const hosts = form.hosts.map((h) => h.trim()).filter(Boolean)

  const setHost = (i: number, v: string) =>
    setForm((p) => ({ ...p, hosts: p.hosts.map((h, k) => (k === i ? v : h)) }))
  const addHost = () => setForm((p) => ({ ...p, hosts: [...p.hosts, ''] }))
  const dropHost = (i: number) => setForm((p) => ({ ...p, hosts: p.hosts.filter((_, k) => k !== i) }))

  const customPort = Number(form.listenPort) || 0
  // 挂默认入口时，选 HTTPS 只是「额外挂到 HTTPS 入口」，HTTP 入口上一直都在
  const shownPort = customPort || (form.https && entry.httpsPort > 0 ? entry.httpsPort : entry.httpPort)
  const shownHttps = customPort ? form.https : form.https && entry.httpsPort > 0
  // 挂默认入口，但那个入口没开——这条配了也不会有人听
  const orphan = !customPort && shownPort === 0
  // 占了端口又不填域名 = 当端口转发使：这个端口上来的请求全收
  const portForward = hosts.length === 0 && customPort > 0
  // 没域名 + 自动匹配 + 挑不出兜底证书 = 这个端口上的 HTTPS 一次都握不成
  const noCertWithoutDomain =
    form.https && hosts.length === 0 && Number(form.certId) === 0 && autoCertNeedsSNI(certs)

  // 端口这一格到底会落在哪，一句话说清。用户最容易误解的就是
  // 「留空」和「选了 HTTPS 却在 80 上也开着」这两件事。
  const portHint = customPort
    ? `这个站点单独占 ${customPort}，只有它在上面`
    : entry.httpPort > 0 && form.https && entry.httpsPort > 0
      ? `留空 = 跟默认入口走，:${entry.httpPort} 和 :${entry.httpsPort} 上都能访问`
      : shownPort > 0
        ? `留空 = 跟默认入口走，落在 :${shownPort}`
        : '留空 = 跟默认入口走'

  const payload = (): api.ProxySitePayload => ({
    hosts: hosts.map(fullHost),
    pathPrefix: form.pathPrefix.trim(),
    stripPrefix: form.stripPrefix,
    backend: form.backend.trim(),
    backendHttps: form.backendHttps,
    listenPort: customPort,
    https: form.https,
    certId: form.https ? Number(form.certId) || 0 : 0,
    enabled: form.enabled,
    description: form.description.trim(),
  })

  const runTest = async () => {
    setTesting(true)
    setProbe(null)
    try {
      const r = await api.testProxySite(payload())
      setProbe({ ok: r.reachable && !r.mismatch, text: r.message })
      // 探到的协议和勾选不符就顺手改掉，省得用户自己回去找那个勾
      if (r.reachable && r.mismatch) setForm((p) => ({ ...p, backendHttps: r.scheme === 'https' }))
    } catch (e) {
      setProbe({ ok: false, text: errText(e) })
    } finally {
      setTesting(false)
    }
  }

  const submit = async () => {
    setErr('')
    setBusy(true)
    try {
      await onSubmit(payload())
    } catch (e) {
      setErr(errText(e))
      setBusy(false)
    }
  }

  return (
    <Modal
      title={initial ? '编辑站点' : '添加站点'}
      onCancel={onCancel}
      footer={
        <>
          <button
            type="button"
            onClick={runTest}
            disabled={testing || !form.backend.trim()}
            className="mr-auto flex items-center gap-1.5 rounded-xl px-3 py-2 text-sm text-slate-500 transition hover:bg-slate-100 disabled:opacity-40"
          >
            <Zap className={`h-3.5 w-3.5 ${testing ? 'animate-pulse' : ''}`} />
            {testing ? '测试中...' : '测试后端'}
          </button>
          <button
            type="button"
            onClick={onCancel}
            className="rounded-xl px-4 py-2 text-sm text-slate-500 transition hover:bg-slate-100"
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
        </>
      }
    >
      {/* 这条站点在干什么，一眼看完 */}
      <div className="flex items-center gap-2 rounded-xl bg-slate-50 px-3 py-2.5 text-xs ring-1 ring-slate-200/70">
        <Globe className="h-3.5 w-3.5 shrink-0 text-blue-500" />
        <span className="truncate font-mono text-slate-700">
          {shownHttps ? 'https://' : 'http://'}
          {hosts.length > 0 ? fullHost(hosts[0]) : portForward ? '任意域名' : '域名'}
          {shownPort > 0 && <span className="text-slate-400">{portText(shownHttps, shownPort)}</span>}
          {hosts.length > 1 && <span className="text-slate-400"> 等 {hosts.length} 个域名</span>}
        </span>
        <ArrowRight className="h-3.5 w-3.5 shrink-0 text-slate-300" />
        <HardDrive className="h-3.5 w-3.5 shrink-0 text-emerald-500" />
        <span className="truncate font-mono text-slate-700">
          {form.backendHttps ? 'https://' : 'http://'}
          {form.backend.trim() || '内网服务'}
        </span>
      </div>

      {/* ==== 对外这一侧：域名 + 端口 + 协议 + 证书 ==== */}
      <div>
        <div className={labelCls}>域名{customPort > 0 && <span className="font-normal">（可选）</span>}</div>
        <div className="space-y-2">
          {form.hosts.map((h, i) => (
            <div key={i} className="flex items-center gap-2">
              <div className="flex flex-1 items-center rounded-xl border border-slate-200 bg-white pr-3 focus-within:border-blue-400">
                <input
                  autoFocus={i === 0}
                  value={h}
                  onChange={(e) => setHost(i, e.target.value)}
                  placeholder={baseDomain ? 'nas' : 'nas.example.com'}
                  className="w-full bg-transparent px-3 py-2 font-mono text-sm outline-none"
                />
                {suffixOf(h) && <span className="shrink-0 font-mono text-sm text-slate-400">{suffixOf(h)}</span>}
              </div>
              {form.hosts.length > 1 && (
                <button
                  type="button"
                  onClick={() => dropHost(i)}
                  className="grid h-8 w-8 shrink-0 place-items-center rounded-lg text-slate-400 transition hover:bg-rose-50 hover:text-rose-500"
                >
                  <X className="h-4 w-4" />
                </button>
              )}
            </div>
          ))}
        </div>
        <button
          type="button"
          onClick={addHost}
          className="mt-2 flex items-center gap-1 rounded-lg px-2 py-1 text-[11px] text-blue-500 transition hover:bg-blue-50"
        >
          <Plus className="h-3 w-3" />
          添加域名
        </button>
        <div className="mt-1 text-[11px] leading-relaxed text-slate-400">
          {portForward ? (
            form.https ? (
              <>
                不填域名 = 这个端口上来的请求全收，等于 nginx 里不写{' '}
                <span className="font-mono">server_name</span> 的 server 块。
                但别指望敲 IP 能进——HTTPS 要先握手，敲 IP 不发 SNI，证书也对不上 IP，得用域名访问
              </>
            ) : (
              <>
                不填域名 = 这个端口上来的请求全收，敲 IP 也能进，就是个端口转发。
                等于 nginx 里不写 <span className="font-mono">server_name</span> 的 server 块
              </>
            )
          ) : (
            <>
              几个域名指向同一个后端就写几行，等于 nginx 的{' '}
              <span className="font-mono">server_name a.com www.a.com</span>
            </>
          )}
        </div>
      </div>

      <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
        <label className="block">
          <div className={labelCls}>端口</div>
          <input
            value={form.listenPort}
            onChange={(e) => setForm((p) => ({ ...p, listenPort: e.target.value }))}
            placeholder={shownPort > 0 ? String(shownPort) : '端口'}
            className={`${inputCls} font-mono`}
          />
        </label>
        <label className="block">
          <div className={labelCls}>协议</div>
          <select
            value={form.https ? 'https' : 'http'}
            onChange={(e) => setForm((p) => ({ ...p, https: e.target.value === 'https' }))}
            className={inputCls}
          >
            <option value="http">HTTP</option>
            <option value="https">HTTPS（含 HTTP/2）</option>
          </select>
        </label>
      </div>
      <div className="!mt-1.5 text-[11px] leading-relaxed text-slate-400">{portHint}</div>

      {portBlocked(customPort) && <BlockedPortWarn label="端口" port={customPort} />}

      {form.https && (
        <label className="block">
          <div className={labelCls}>证书</div>
          <CertSelect value={form.certId} certs={certs} onChange={(v) => setForm((p) => ({ ...p, certId: v }))} />
          <div className="mt-1 text-[11px] leading-relaxed text-slate-400">自动匹配就是按访问的域名挑，一般不用动</div>
        </label>
      )}

      {noCertWithoutDomain && (
        <Warn>
          没填域名，证书又是「自动匹配」——自动匹配是按访问用的域名挑证书的，而这条站点没有域名可挑：
          现在还没有默认证书，可用证书也不止一张，握手会直接失败，浏览器只报{' '}
          <span className="font-mono">ERR_SSL_PROTOCOL_ERROR</span>。
          上面挑一张具体证书，或者去「证书」页把一张设为默认。
          一张证书都不想管的话，「证书」页可以现签一张自签的——浏览器会拦一道警告，但 HTTPS 能通
        </Warn>
      )}

      {orphan && (
        <Warn>默认入口一个都没开，这条站点存下来也没人监听。去「默认入口」填个端口，或者在上面给它单独填一个</Warn>
      )}

      {/* ==== 后端这一侧 ==== */}
      <div className="!mt-5 border-t border-slate-100 pt-4">
        <div className={labelCls}>代理地址</div>
        <div className="flex gap-2">
          <select
            value={form.backendHttps ? 'https' : 'http'}
            onChange={(e) => setForm((p) => ({ ...p, backendHttps: e.target.value === 'https' }))}
            className={`${fieldCls} w-24 shrink-0 font-mono`}
          >
            <option value="http">http</option>
            <option value="https">https</option>
          </select>
          <input
            value={form.backend}
            onChange={(e) => setForm((p) => ({ ...p, backend: e.target.value }))}
            placeholder="192.168.1.20:5000"
            className={`${fieldCls} min-w-0 flex-1 font-mono`}
          />
        </div>
        <div className="mt-1 text-[11px] leading-relaxed text-slate-400">
          内网服务的 地址:端口。跑在本机的服务填 <span className="font-mono">127.0.0.1:端口</span>
          ；服务自己是 HTTPS（自签也算）就把左边换成 https
        </div>
      </div>

      {probe && (
        <div
          className={`rounded-xl px-3 py-2 text-[11px] leading-relaxed ring-1 ${
            probe.ok ? 'bg-emerald-50 text-emerald-700 ring-emerald-200' : 'bg-amber-50 text-amber-700 ring-amber-200'
          }`}
        >
          {probe.text}
        </div>
      )}

      <label className="!mt-5 block border-t border-slate-100 pt-4">
        <div className={labelCls}>路径（可选）</div>
        <input
          value={form.pathPrefix}
          onChange={(e) => setForm((p) => ({ ...p, pathPrefix: e.target.value }))}
          placeholder="留空 = 整站"
          className={`${inputCls} font-mono`}
        />
      </label>

      {!!form.pathPrefix.trim() && (
        <>
          <label className="flex cursor-pointer items-center gap-2 rounded-xl bg-slate-50 px-3 py-2.5 text-xs text-slate-600">
            <input
              type="checkbox"
              checked={form.stripPrefix}
              onChange={(e) => setForm((p) => ({ ...p, stripPrefix: e.target.checked }))}
              className="h-3.5 w-3.5"
            />
            转发前剥掉这个前缀
          </label>
          {form.stripPrefix && (
            <div className="flex gap-2 rounded-xl bg-amber-50 px-3 py-2.5 text-[11px] leading-relaxed text-amber-700 ring-1 ring-amber-200">
              <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <span>后端得自己支持 base path，否则页面里的资源会 404。不确定就别勾，给它单独一个域名</span>
            </div>
          )}
        </>
      )}

      <label className="block">
        <div className={labelCls}>备注（可选）</div>
        <input
          value={form.description}
          onChange={(e) => setForm((p) => ({ ...p, description: e.target.value }))}
          className={inputCls}
        />
      </label>

      <label className="flex cursor-pointer items-center gap-2 text-xs text-slate-600">
        <input
          type="checkbox"
          checked={form.enabled}
          onChange={(e) => setForm((p) => ({ ...p, enabled: e.target.checked }))}
          className="h-3.5 w-3.5"
        />
        启用
      </label>

      {err && <div className="rounded-xl bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>}
    </Modal>
  )
}

// ===================== 监听端口一览 =====================

/** 一个端口一块，对着 netstat 看到的那几行 */
function ListenerChip({ l, certName }: { l: ProxyListener; certName: string }) {
  return (
    <span
      title={l.tls ? `证书：${certName}` : undefined}
      className={`inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-[11px] ring-1 ${
        l.running ? 'bg-white text-slate-600 ring-slate-200' : 'bg-rose-50 text-rose-600 ring-rose-200'
      }`}
    >
      <span className={`h-1.5 w-1.5 shrink-0 rounded-full ${l.running ? 'bg-emerald-500' : 'bg-rose-500'}`} />
      <span className="font-mono font-semibold">:{l.port}</span>
      {l.tls ? (
        <span className="inline-flex items-center gap-0.5 font-semibold text-violet-600">
          <ShieldCheck className="h-3 w-3" />
          HTTPS
        </span>
      ) : (
        <span className="text-slate-400">HTTP</span>
      )}
      <span className="text-slate-400">{l.siteCount} 站点</span>
    </span>
  )
}

// ===================== 主页面 =====================

export function ReverseProxy() {
  const [cfg, setCfg] = useState<ProxyConfig | null>(null)
  const [certs, setCerts] = useState<Certificate[]>([])
  const [loadErr, setLoadErr] = useState('')
  const [testingId, setTestingId] = useState<number | null>(null)
  const { toasts, show: toast } = useToast()

  const [entryModal, setEntryModal] = useState(false)
  const [siteModal, setSiteModal] = useState<{ open: boolean; initial?: ProxySite }>({ open: false })

  const refresh = useCallback(async () => {
    try {
      setCfg(await api.getProxyConfig())
      setLoadErr('')
    } catch (e) {
      setLoadErr(errText(e))
    }
  }, [])

  useEffect(() => {
    refresh()
    const t = window.setInterval(refresh, 10000)
    return () => window.clearInterval(t)
  }, [refresh])

  // 证书只有弹窗用得上，拿不到不该拦住整页
  useEffect(() => {
    let alive = true
    api
      .getCertList()
      .then((l) => alive && setCerts(l))
      .catch(() => {})
    return () => {
      alive = false
    }
  }, [])

  const entry = cfg?.entry
  const listeners = useMemo(() => cfg?.listeners ?? [], [cfg])
  const sites = useMemo(() => cfg?.sites ?? [], [cfg])
  const certName = useCallback(
    (id: number) => (id === 0 ? '按域名自动匹配' : (certs.find((c) => c.id === id)?.name ?? `#${id}`)),
    [certs],
  )

  const saveEntry = async (payload: Omit<api.EntryPayload, 'enabled'>) => {
    // 改设置不该顺手把停掉的反代拉起来
    await api.saveProxyEntry({ ...payload, enabled: entry?.enabled ?? true })
    setEntryModal(false)
    toast('已保存')
    await refresh()
  }

  const toggleRunning = async () => {
    if (!entry) return
    try {
      await api.saveProxyEntry({
        enabled: !entry.enabled,
        httpPort: entry.httpPort,
        httpsPort: entry.httpsPort,
        certId: entry.certId,
        baseDomain: entry.baseDomain ?? '',
      })
      toast(entry.enabled ? '已停止' : '已启动')
    } catch (e) {
      toast(errText(e))
    }
    await refresh()
  }

  const toggleAccessLog = async () => {
    if (!entry) return
    try {
      await api.setProxyAccessLog(!entry.accessLog)
      await refresh()
    } catch (e) {
      toast(errText(e))
    }
  }

  const submitSite = async (payload: api.ProxySitePayload) => {
    if (siteModal.initial) {
      await api.updateProxySite({ ...payload, id: siteModal.initial.id })
      toast('已更新')
    } else {
      await api.addProxySite(payload)
      toast('已添加')
    }
    setSiteModal({ open: false })
    await refresh()
  }

  const removeSite = async (s: ProxySite) => {
    if (!window.confirm(`确认删除站点 ${s.hosts?.[0] ?? `:${s.listenPort}`}？`)) return
    try {
      await api.deleteProxySite(s.id)
      toast('已删除')
      await refresh()
    } catch (e) {
      toast(errText(e))
    }
  }

  const testSite = async (s: ProxySite) => {
    setTestingId(s.id)
    try {
      const probe = await api.testProxySite({
        hosts: s.hosts ?? [],
        pathPrefix: s.pathPrefix,
        stripPrefix: s.stripPrefix,
        backend: s.backend,
        backendHttps: s.backendHttps,
        listenPort: s.listenPort,
        https: s.https,
        certId: s.certId,
        enabled: s.enabled,
        description: s.description,
      })
      toast(`${s.backend}：${probe.message}`)
    } catch (e) {
      toast('测试失败：' + errText(e))
    } finally {
      setTestingId(null)
    }
  }

  if (loadErr && !cfg) {
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

  if (!cfg || !entry) {
    return (
      <Card>
        <div className="py-12 text-center text-sm text-slate-400">正在加载...</div>
      </Card>
    )
  }

  const example = entry.baseDomain ? `nas.${entry.baseDomain}` : 'nas.example.com'
  const entryPort = entry.httpPort || entry.httpsPort
  const failed = listeners.filter((l) => !l.running)

  // 状态一句话：停了 / 没端口可听 / 全起来了 / 有几个没起来
  const stateText = !entry.enabled
    ? '已停止'
    : listeners.length === 0
      ? '没有监听'
      : failed.length === 0
        ? '运行中'
        : `${failed.length} 个端口没起来`
  const stateDot = !entry.enabled
    ? 'bg-slate-300'
    : listeners.length === 0
      ? 'bg-slate-300'
      : failed.length === 0
        ? 'bg-emerald-500'
        : 'bg-amber-400'

  return (
    <div className="space-y-4">
      <div>
        <div className="text-base font-bold text-slate-800">反向代理</div>
        <div className="mt-1 text-xs text-slate-500">
          在本机监听端口，按域名把请求转给内网的机器。就是 nginx 干的活
        </div>
      </div>

      {/* 监听状态：哪些端口开着、各挂了几个站点 */}
      <Card className="!py-3.5">
        <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
          <span className={`h-2 w-2 shrink-0 rounded-full ${stateDot}`} />
          <span className="text-sm font-semibold text-slate-700">{stateText}</span>

          {entry.baseDomain && <span className="font-mono text-[11px] text-slate-400">*.{entry.baseDomain}</span>}

          <button
            onClick={toggleAccessLog}
            title="点一下开关访问日志"
            className={`rounded-md px-1.5 py-0.5 text-[11px] font-semibold transition ${
              entry.accessLog
                ? 'bg-slate-200 text-slate-600 hover:bg-slate-300'
                : 'text-slate-300 hover:bg-slate-100 hover:text-slate-500'
            }`}
          >
            访问日志
          </button>

          <div className="ml-auto flex items-center gap-1">
            <button
              onClick={toggleRunning}
              className="rounded-lg px-2.5 py-1 text-xs font-semibold text-slate-500 hover:bg-slate-100"
            >
              {entry.enabled ? '停止' : '启动'}
            </button>
            <button
              onClick={() => setEntryModal(true)}
              className="flex items-center gap-1 rounded-lg px-2.5 py-1 text-xs font-semibold text-slate-500 hover:bg-slate-100"
            >
              <Settings2 className="h-3 w-3" /> 默认入口
            </button>
          </div>
        </div>

        {listeners.length > 0 && (
          <div className="mt-2.5 flex flex-wrap gap-1.5">
            {listeners.map((l) => (
              <ListenerChip key={l.port} l={l} certName={certName(l.certId)} />
            ))}
          </div>
        )}

        {failed.map(
          (l) =>
            l.lastError && (
              <div
                key={l.port}
                className="mt-2 rounded-xl bg-rose-50 px-3 py-2 text-[11px] leading-relaxed text-rose-600"
              >
                <span className="font-mono font-semibold">:{l.port}</span> {l.lastError}
              </div>
            ),
        )}

        {entry.enabled && listeners.length === 0 && sites.length > 0 && (
          <div className="mt-2 text-[11px] text-slate-400">
            站点都停用了，或者默认入口两个端口都没填——点右上角「默认入口」看看
          </div>
        )}

        {listeners.length > 0 && failed.length === 0 && (
          <div className="mt-2 text-[11px] text-slate-400">
            要让外网也能访问：到 STUN 页给这些端口各加一条 TCP 服务
          </div>
        )}
      </Card>

      {/* 站点表 */}
      <Card className="!p-0">
        <div className="px-5 pt-4">
          <CardHeader
            title={
              <span>
                站点 <span className="ml-1 font-normal text-slate-400">{sites.length}</span>
              </span>
            }
            action={
              <button
                onClick={() => setSiteModal({ open: true })}
                className="flex items-center gap-1 rounded-md bg-blue-500 px-2.5 py-1 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 hover:bg-blue-600"
              >
                <Plus className="h-3 w-3" /> 添加站点
              </button>
            }
          />
        </div>

        {sites.length === 0 ? (
          // 空状态直接把这东西怎么工作画出来，比写一段话管用
          <div className="px-5 pb-10 pt-4">
            <div className="mx-auto flex max-w-xl flex-col items-center">
              <div className="flex w-full items-center justify-center gap-2 sm:gap-4">
                <div className="flex w-28 flex-col items-center gap-1.5">
                  <div className="grid h-11 w-11 place-items-center rounded-2xl bg-blue-50 text-blue-500">
                    <Globe className="h-5 w-5" />
                  </div>
                  <div className="break-all text-center font-mono text-[11px] text-slate-500">{example}</div>
                  <div className="text-[10px] text-slate-400">浏览器</div>
                </div>

                <ArrowRight className="h-4 w-4 shrink-0 text-slate-300" />

                <div className="flex w-28 flex-col items-center gap-1.5">
                  <div className="grid h-11 w-11 place-items-center rounded-2xl bg-slate-900 text-[10px] font-bold text-white">
                    LS
                  </div>
                  <div className="font-mono text-[11px] text-slate-500">{entryPort ? `:${entryPort}` : '未设端口'}</div>
                  <div className="text-[10px] text-slate-400">LinkStar</div>
                </div>

                <ArrowRight className="h-4 w-4 shrink-0 text-slate-300" />

                <div className="flex w-28 flex-col items-center gap-1.5">
                  <div className="grid h-11 w-11 place-items-center rounded-2xl bg-emerald-50 text-emerald-500">
                    <HardDrive className="h-5 w-5" />
                  </div>
                  <div className="break-all text-center font-mono text-[11px] text-slate-500">192.168.1.20:5000</div>
                  <div className="text-[10px] text-slate-400">内网服务</div>
                </div>
              </div>

              <button
                onClick={() => setSiteModal({ open: true })}
                className="mt-7 rounded-xl bg-blue-500 px-4 py-2 text-xs font-semibold text-white shadow-md shadow-blue-500/20 hover:bg-blue-600"
              >
                添加第一个站点
              </button>
              <div className="mt-2.5 text-[11px] text-slate-400">
                一个站点一个域名，域名解析到本机就行。要单独的端口、单独的证书也行
              </div>
            </div>
          </div>
        ) : (
          <div className="overflow-x-auto">
            {/* 手机上列太窄会把中文压成一列一个字，给张表一个下限，窄屏改成横向滚动 */}
            <table className="w-full min-w-[560px] text-sm">
              <thead>
                <tr className="bg-slate-50/60 text-xs text-slate-500">
                  <th className="px-5 py-2.5 text-left font-medium">域名</th>
                  <th className="w-8 py-2.5" />
                  <th className="px-3 py-2.5 text-left font-medium">后端</th>
                  <th className="px-5 py-2.5 text-right font-medium">操作</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100">
                {sites.map((s) => {
                  const url = siteURL(entry, s)
                  const addr = siteAddr(entry, s)
                  return (
                    <tr key={s.id} className="align-top text-slate-700 transition hover:bg-slate-50/60">
                      <td className="px-5 py-3">
                        <div className="flex flex-wrap items-center gap-1.5">
                          {url ? (
                            <a
                              href={url}
                              target="_blank"
                              rel="noopener noreferrer"
                              className="inline-flex items-center gap-1 font-mono text-[13px] font-semibold text-slate-800 hover:text-blue-600"
                            >
                              {s.hosts?.[0]}
                              {addr && <span className="text-slate-400">{portText(addr.https, addr.port)}</span>}
                              {s.pathPrefix && <span className="text-slate-400">{s.pathPrefix}</span>}
                              <ExternalLink className="h-3 w-3 text-slate-300" />
                            </a>
                          ) : (
                            <span className="font-mono text-[13px] font-semibold">
                              {s.hosts?.[0] ?? <span className="text-slate-400">任意域名</span>}
                              {addr && <span className="text-slate-400">{portText(addr.https, addr.port)}</span>}
                              {s.pathPrefix && <span className="text-slate-400">{s.pathPrefix}</span>}
                            </span>
                          )}
                          {(s.hosts?.length ?? 0) === 0 && (
                            <span
                              title="没填域名，这个端口上来的请求全收"
                              className="rounded bg-slate-100 px-1 py-0.5 text-[10px] font-bold text-slate-500"
                            >
                              端口转发
                            </span>
                          )}
                          {(s.hosts?.length ?? 0) > 1 && (
                            <span
                              title={s.hosts.slice(1).join('\n')}
                              className="rounded bg-slate-100 px-1 py-0.5 text-[10px] font-bold text-slate-500"
                            >
                              +{s.hosts.length - 1} 个域名
                            </span>
                          )}
                          {s.https && (
                            <span className="inline-flex items-center gap-0.5 rounded bg-violet-50 px-1 py-0.5 text-[10px] font-bold text-violet-600">
                              <ShieldCheck className="h-2.5 w-2.5" /> HTTPS
                            </span>
                          )}
                          {s.listenPort > 0 && (
                            <span className="rounded bg-blue-50 px-1 py-0.5 text-[10px] font-bold text-blue-600">
                              独占端口
                            </span>
                          )}
                          {addr && portBlocked(addr.port) && (
                            <span
                              title="Chrome / Firefox 拒连这个端口，报 ERR_UNSAFE_PORT。反代本身是通的，curl 也能访问，只有浏览器进不来"
                              className="rounded bg-red-50 px-1 py-0.5 text-[10px] font-bold text-red-600"
                            >
                              浏览器拒连
                            </span>
                          )}
                          {s.https && (s.hosts?.length ?? 0) === 0 && s.certId === 0 && autoCertNeedsSNI(certs) && (
                            <span
                              title="证书这格选的是「按域名自动匹配」，可这条站点没有域名，挑不出证书就一张都不出示，浏览器报 ERR_SSL_PROTOCOL_ERROR。进去指定一张证书就好了"
                              className="rounded bg-red-50 px-1 py-0.5 text-[10px] font-bold text-red-600"
                            >
                              HTTPS 连不上
                            </span>
                          )}
                          {!s.enabled && (
                            <span className="rounded bg-slate-100 px-1.5 py-0.5 text-[10px] font-bold text-slate-500">
                              已停用
                            </span>
                          )}
                          {s.stripPrefix && (
                            <span className="rounded bg-amber-50 px-1 py-0.5 text-[10px] font-bold text-amber-600">
                              剥前缀
                            </span>
                          )}
                        </div>
                        {s.description && <div className="mt-0.5 text-[11px] text-slate-400">{s.description}</div>}
                      </td>
                      <td className="py-3 text-center">
                        <ArrowRight className="mx-auto h-3.5 w-3.5 text-slate-300" />
                      </td>
                      <td className="px-3 py-3 font-mono text-xs text-slate-600">
                        {s.backend}
                        {s.backendHttps && (
                          <span className="ml-1 rounded bg-emerald-50 px-1 py-0.5 font-sans text-[10px] font-bold text-emerald-600">
                            TLS
                          </span>
                        )}
                      </td>
                      <td className="px-5 py-3 text-right">
                        <div className="inline-flex items-center gap-1">
                          <button
                            onClick={() => testSite(s)}
                            disabled={testingId === s.id}
                            className="grid h-6 w-6 place-items-center rounded-md text-slate-400 hover:bg-blue-50 hover:text-blue-600 disabled:opacity-40"
                            title="测试后端"
                          >
                            <Zap className={`h-3 w-3 ${testingId === s.id ? 'animate-pulse' : ''}`} />
                          </button>
                          <button
                            onClick={() => setSiteModal({ open: true, initial: s })}
                            className="grid h-6 w-6 place-items-center rounded-md text-slate-400 hover:bg-emerald-50 hover:text-emerald-600"
                            title="编辑"
                          >
                            <Pencil className="h-3 w-3" />
                          </button>
                          <button
                            onClick={() => removeSite(s)}
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
        )}
      </Card>

      {entryModal && (
        <EntryModal entry={entry} certs={certs} onCancel={() => setEntryModal(false)} onSubmit={saveEntry} />
      )}
      {siteModal.open && (
        <SiteModal
          initial={siteModal.initial}
          entry={entry}
          certs={certs}
          onCancel={() => setSiteModal({ open: false })}
          onSubmit={submitSite}
        />
      )}

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
