import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  AlertCircle,
  CircleCheck,
  CircleSlash,
  Clock,
  FileKey,
  FolderOpen,
  KeyRound,
  Pencil,
  Plus,
  RefreshCw,
  ShieldCheck,
  TriangleAlert,
  Upload,
  Trash2,
  X,
  type LucideIcon,
} from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import * as api from '../lib/api'
import type { AcmeProvider, Certificate, CertSource } from '../types'

// ===================== 来源元信息 =====================

interface SourceMeta {
  value: CertSource
  label: string
  hint: string
  icon: LucideIcon
  /** 走 ACME 自动签发 */
  acme?: boolean
}

const SOURCES: SourceMeta[] = [
  {
    value: 'acme-dns01',
    label: '自动签发 · DNS-01',
    hint: '用 DDNS 里已配好的服务商写 TXT 记录验证。支持通配符证书，不需要任何入站端口，CGNAT / 大内网也能用。',
    icon: ShieldCheck,
    acme: true,
  },
  {
    value: 'acme-http01',
    label: '自动签发 · HTTP-01',
    hint: '由 CA 回连你的 80 端口验证。不支持通配符域名。',
    icon: ShieldCheck,
    acme: true,
  },
  {
    value: 'upload',
    label: '手动上传',
    hint: '直接粘贴证书和私钥的 PEM 文本。到期需要自己重新上传，系统不会自动续期。',
    icon: FileKey,
  },
  {
    value: 'path',
    label: '读取本机文件',
    hint: '指向本机上已有的 PEM 文件（如 certbot / acme.sh 的输出目录）。文件变动后自动热重载，无需重启。',
    icon: FolderOpen,
  },
  {
    value: 'self-signed',
    label: '自签（本机现签）',
    hint: '不用域名、不用联网、不用等，点一下就有。浏览器会拦一道「不受信任」，要手动点继续——想要地址栏干净只能用自动签发或上传真证书。',
    icon: KeyRound,
  },
]

/** 自签证书换不来什么，得当面说清楚，别让人配完了才发现 */
const SELF_SIGNED_NOTE =
  '自签证书没有任何机构背书，浏览器一定会拦一道「您的连接不是私密连接」，点「高级 → 继续前往」才能进。它解决的只是「HTTPS 必须有证书」这一条，加密是真的，身份不是。自己内网用、或者外面还有一层（Cloudflare 等）在管证书时够用；要给别人访问、要地址栏干净，用 DNS-01 自动签发。'

const sourceMetaMap = new Map(SOURCES.map((s) => [s.value, s]))
const sourceLabel = (s: CertSource) => sourceMetaMap.get(s)?.label ?? s

/** HTTP-01 的端口是协议写死的，改不了。这个提示必须常驻，不能只在报错后才说。 */
const HTTP01_WARNING =
  'HTTP-01 需要公网 80 端口可入站，且端口由协议规定不可更改。CGNAT / 大内网环境用不了，请改用 DNS-01。'

const DIRECTORIES = [
  { value: '', label: "Let's Encrypt 正式环境" },
  { value: 'https://acme-staging-v02.api.letsencrypt.org/directory', label: "Let's Encrypt 测试环境（staging）" },
  { value: 'custom', label: '自定义 ACME 服务（ZeroSSL 等）' },
]

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
    window.setTimeout(() => setToasts((p) => p.filter((t) => t.id !== id)), 2600)
  }, [])
  return { toasts, show }
}

function fmtDate(s?: string): string {
  if (!s) return '--'
  const d = new Date(s)
  if (Number.isNaN(d.getTime()) || d.getFullYear() < 2000) return '--'
  return d.toLocaleDateString()
}

/** 域名输入：按空白或逗号分隔，顺便去重 */
function parseDomains(raw: string): string[] {
  const seen = new Set<string>()
  const out: string[] = []
  for (const part of raw.split(/[\s,，]+/)) {
    const d = part.trim().toLowerCase().replace(/\.$/, '')
    if (!d || seen.has(d)) continue
    seen.add(d)
    out.push(d)
  }
  return out
}

interface CertStatus {
  label: string
  tone: string
  icon: LucideIcon
}

function certStatus(c: Certificate): CertStatus {
  if (!c.enabled) return { label: '已禁用', tone: 'bg-slate-100 text-slate-500', icon: CircleSlash }
  if (c.expired) return { label: '已过期', tone: 'bg-rose-50 text-rose-500', icon: TriangleAlert }
  if (c.lastError) return { label: '有错误', tone: 'bg-rose-50 text-rose-500', icon: TriangleAlert }
  if (!c.loaded) {
    // ACME 证书新建后还没签下来、自签的还没生成，都属于正常的中间态，别吓唬用户
    const pending =
      c.source === 'acme-dns01' || c.source === 'acme-http01' || c.source === 'self-signed'
    return pending
      ? { label: '等待签发', tone: 'bg-amber-50 text-amber-600', icon: RefreshCw }
      : { label: '未加载', tone: 'bg-amber-50 text-amber-600', icon: Clock }
  }
  if (c.daysLeft <= 7) return { label: '即将到期', tone: 'bg-amber-50 text-amber-600', icon: Clock }
  return { label: '生效中', tone: 'bg-emerald-50 text-emerald-600', icon: CircleCheck }
}

// ===================== 新建 / 编辑弹窗 =====================

interface CertFormState {
  name: string
  source: CertSource
  domainsRaw: string
  enabled: boolean
  isDefault: boolean
  certFile: string
  keyFile: string
  directoryChoice: string
  directoryCustom: string
  email: string
  providerId: string
  httpPort: string
  eabKeyId: string
  eabHmac: string
  renewDays: string
}

function initialForm(initial?: Certificate, providers: AcmeProvider[] = []): CertFormState {
  if (!initial) {
    const firstUsable = providers.find((p) => p.supportsDns01)
    return {
      name: '',
      source: 'acme-dns01',
      domainsRaw: '',
      enabled: true,
      isDefault: false,
      certFile: '',
      keyFile: '',
      directoryChoice: '',
      directoryCustom: '',
      email: '',
      providerId: firstUsable ? String(firstUsable.id) : '',
      httpPort: '80',
      eabKeyId: '',
      eabHmac: '',
      renewDays: '30',
    }
  }
  const known = DIRECTORIES.some((d) => d.value === initial.acme.directory)
  return {
    name: initial.name,
    source: initial.source,
    domainsRaw: initial.domains.join('\n'),
    enabled: initial.enabled,
    isDefault: initial.isDefault,
    certFile: initial.certFile,
    keyFile: initial.keyFile,
    directoryChoice: known ? initial.acme.directory : initial.acme.directory ? 'custom' : '',
    directoryCustom: known ? '' : initial.acme.directory,
    email: initial.acme.email,
    providerId: initial.acme.providerId ? String(initial.acme.providerId) : '',
    httpPort: String(initial.acme.httpPort || 80),
    eabKeyId: initial.acme.eabKeyId,
    eabHmac: '',
    renewDays: String(initial.acme.renewDays || 30),
  }
}

function CertModal({
  initial,
  providers,
  onCancel,
  onSubmit,
}: {
  initial?: Certificate
  providers: AcmeProvider[]
  onCancel: () => void
  onSubmit: (payload: api.CertPayload) => Promise<void>
}) {
  const [form, setForm] = useState<CertFormState>(() => initialForm(initial, providers))
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const meta = sourceMetaMap.get(form.source) ?? SOURCES[0]
  const isDns01 = form.source === 'acme-dns01'
  const isHttp01 = form.source === 'acme-http01'
  const isAcme = !!meta.acme
  const isPath = form.source === 'path'
  const isSelfSigned = form.source === 'self-signed'

  const domains = useMemo(() => parseDomains(form.domainsRaw), [form.domainsRaw])
  const hasWildcard = domains.some((d) => d.startsWith('*.'))

  const submit = async () => {
    if (!form.name.trim()) {
      setErr('请填写证书名称')
      return
    }
    if (isPath && (!form.certFile.trim() || !form.keyFile.trim())) {
      setErr('请填写证书和私钥的文件路径')
      return
    }
    if (isAcme && domains.length === 0) {
      setErr('自动签发至少需要填写一个域名')
      return
    }
    if (isDns01 && !form.providerId) {
      setErr('DNS-01 需要选择一个 DNS 服务商')
      return
    }
    if (isHttp01 && hasWildcard) {
      setErr('HTTP-01 不支持通配符域名，通配符证书请改用 DNS-01')
      return
    }

    const directory = form.directoryChoice === 'custom' ? form.directoryCustom.trim() : form.directoryChoice

    setErr('')
    setBusy(true)
    try {
      await onSubmit({
        name: form.name.trim(),
        source: form.source,
        domains,
        enabled: form.enabled,
        isDefault: form.isDefault,
        certFile: form.certFile.trim(),
        keyFile: form.keyFile.trim(),
        acme: {
          directory,
          email: form.email.trim(),
          providerId: isDns01 ? Number(form.providerId) || 0 : 0,
          httpPort: isHttp01 ? Number(form.httpPort) || 80 : 0,
          eabKeyId: form.eabKeyId.trim(),
          eabHmac: form.eabHmac.trim(),
          renewDays: Number(form.renewDays) || 30,
        },
      })
    } catch (e) {
      setErr(e instanceof Error ? e.message : '操作失败')
      setBusy(false)
    }
  }

  const input =
    'w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400'
  const labelText = 'mb-1 text-xs font-semibold text-slate-500'

  return (
    <div
      className="fixed inset-0 z-50 grid place-items-center bg-black/30 px-4 py-6 backdrop-blur-sm"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="max-h-full w-full max-w-lg overflow-y-auto rounded-2xl bg-white p-6 text-slate-700 shadow-2xl ring-1 ring-slate-200">
        <div className="mb-5 flex items-center justify-between">
          <div className="text-base font-bold text-slate-800">{initial ? '编辑证书' : '添加证书'}</div>
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
            <div className={labelText}>证书名称</div>
            <input
              autoFocus
              value={form.name}
              onChange={(e) => setForm((p) => ({ ...p, name: e.target.value }))}
              placeholder="如 主域名通配符"
              className={input}
            />
          </label>

          <label className="block">
            <div className={labelText}>证书来源</div>
            <select
              value={form.source}
              onChange={(e) => {
                // 换来源要顺手清掉报错：上一条错误是针对旧来源的，
                // 留在那里会变成「已经改用 DNS-01 了还在说 HTTP-01 不支持通配符」
                setErr('')
                setForm((p) => ({ ...p, source: e.target.value as CertSource }))
              }}
              className={input}
            >
              {SOURCES.map((s) => (
                <option key={s.value} value={s.value}>
                  {s.label}
                </option>
              ))}
            </select>
            <div className="mt-1.5 text-[11px] leading-relaxed text-slate-400">{meta.hint}</div>
          </label>

          {/* HTTP-01 的限制是协议层面的，常驻展示，不等报错才说 */}
          {isHttp01 && (
            <div className="flex gap-2 rounded-xl bg-amber-50 px-3 py-2.5 text-[11px] leading-relaxed text-amber-700 ring-1 ring-amber-200">
              <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <span>{HTTP01_WARNING}</span>
            </div>
          )}

          {/* 自签换不来什么，配之前就得知道，别配完了才发现地址栏还是红的 */}
          {isSelfSigned && (
            <div className="flex gap-2 rounded-xl bg-amber-50 px-3 py-2.5 text-[11px] leading-relaxed text-amber-700 ring-1 ring-amber-200">
              <TriangleAlert className="mt-0.5 h-3.5 w-3.5 shrink-0" />
              <span>{SELF_SIGNED_NOTE}</span>
            </div>
          )}

          <label className="block">
            <div className={labelText}>
              覆盖域名
              {isAcme ? '' : isSelfSigned ? '（可以不填）' : '（留空则从证书里自动读取）'}
            </div>
            <textarea
              rows={3}
              value={form.domainsRaw}
              onChange={(e) => setForm((p) => ({ ...p, domainsRaw: e.target.value }))}
              placeholder={'一行一个，如\nexample.com\n*.example.com'}
              className={`${input} font-mono`}
            />
            <div className="mt-1 text-[11px] text-slate-400">
              {domains.length > 0
                ? `将覆盖 ${domains.length} 个域名：${domains.join('、')}`
                : isSelfSigned
                  ? '不填也行——本机的 IP 会自动写进证书，敲 IP 访问时名字对得上'
                  : '用换行或逗号分隔'}
            </div>
            {hasWildcard && (
              <div className="mt-1 text-[11px] text-slate-400">
                通配符只覆盖一层：<span className="font-mono">*.example.com</span> 能覆盖{' '}
                <span className="font-mono">a.example.com</span>，但不覆盖{' '}
                <span className="font-mono">example.com</span> 和{' '}
                <span className="font-mono">a.b.example.com</span>
              </div>
            )}
          </label>

          {isPath && (
            <>
              <label className="block">
                <div className={labelText}>证书文件路径（fullchain.pem）</div>
                <input
                  value={form.certFile}
                  onChange={(e) => setForm((p) => ({ ...p, certFile: e.target.value }))}
                  placeholder="/etc/letsencrypt/live/example.com/fullchain.pem"
                  className={`${input} font-mono`}
                />
              </label>
              <label className="block">
                <div className={labelText}>私钥文件路径（privkey.pem）</div>
                <input
                  value={form.keyFile}
                  onChange={(e) => setForm((p) => ({ ...p, keyFile: e.target.value }))}
                  placeholder="/etc/letsencrypt/live/example.com/privkey.pem"
                  className={`${input} font-mono`}
                />
              </label>
            </>
          )}

          {isAcme && (
            <>
              <label className="block">
                <div className={labelText}>ACME 服务</div>
                <select
                  value={form.directoryChoice}
                  onChange={(e) => setForm((p) => ({ ...p, directoryChoice: e.target.value }))}
                  className={input}
                >
                  {DIRECTORIES.map((d) => (
                    <option key={d.value} value={d.value}>
                      {d.label}
                    </option>
                  ))}
                </select>
                {form.directoryChoice === 'custom' && (
                  <input
                    value={form.directoryCustom}
                    onChange={(e) => setForm((p) => ({ ...p, directoryCustom: e.target.value }))}
                    placeholder="https://acme.zerossl.com/v2/DV90"
                    className={`${input} mt-2 font-mono`}
                  />
                )}
                {form.directoryChoice.includes('staging') && (
                  <div className="mt-1 text-[11px] text-slate-400">
                    测试环境签出的证书浏览器不信任，仅用于验证流程是否跑通
                  </div>
                )}
              </label>

              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <label className="block">
                  <div className={labelText}>联系邮箱（可选）</div>
                  <input
                    value={form.email}
                    onChange={(e) => setForm((p) => ({ ...p, email: e.target.value }))}
                    placeholder="you@example.com"
                    className={input}
                  />
                </label>
                <label className="block">
                  <div className={labelText}>提前续期天数</div>
                  <input
                    type="number"
                    min={1}
                    max={89}
                    value={form.renewDays}
                    onChange={(e) => setForm((p) => ({ ...p, renewDays: e.target.value }))}
                    className={input}
                  />
                </label>
              </div>

              {isDns01 && (
                <label className="block">
                  <div className={labelText}>DNS 服务商</div>
                  <select
                    value={form.providerId}
                    onChange={(e) => setForm((p) => ({ ...p, providerId: e.target.value }))}
                    className={input}
                  >
                    {providers.length === 0 && <option value="">请先在 DDNS 页面添加服务商</option>}
                    {providers.map((p) => (
                      <option key={p.id} value={String(p.id)} disabled={!p.supportsDns01}>
                        {p.name}
                        {p.supportsDns01 ? '' : '（暂不支持 DNS-01）'}
                      </option>
                    ))}
                  </select>
                  <div className="mt-1 text-[11px] text-slate-400">
                    复用 DDNS 里已配好的凭证，签发时自动写入 TXT 记录，验证完成后自动清理
                  </div>
                </label>
              )}

              {isHttp01 && (
                <label className="block">
                  <div className={labelText}>验证监听端口</div>
                  <input
                    type="number"
                    value={form.httpPort}
                    onChange={(e) => setForm((p) => ({ ...p, httpPort: e.target.value }))}
                    className={input}
                  />
                  <div className="mt-1 text-[11px] text-slate-400">
                    CA 始终从 80 端口发起验证。这里只在你用端口转发把外部 80 转到本机其它端口时才需要改。
                  </div>
                </label>
              )}

              <details className="rounded-xl bg-slate-50 px-3 py-2">
                <summary className="cursor-pointer text-xs font-semibold text-slate-500">
                  外部账户绑定 EAB（ZeroSSL 等需要）
                </summary>
                <div className="mt-2 space-y-2">
                  <input
                    value={form.eabKeyId}
                    onChange={(e) => setForm((p) => ({ ...p, eabKeyId: e.target.value }))}
                    placeholder="EAB Key ID"
                    className={`${input} font-mono`}
                  />
                  <input
                    type="password"
                    value={form.eabHmac}
                    onChange={(e) => setForm((p) => ({ ...p, eabHmac: e.target.value }))}
                    placeholder={initial?.acme.hasEabHmac ? '••••••••（已配置，留空不修改）' : 'EAB HMAC Key'}
                    className={`${input} font-mono`}
                  />
                </div>
              </details>
            </>
          )}

          {form.source === 'upload' && !initial && (
            <div className="rounded-xl bg-blue-50 px-3 py-2.5 text-[11px] leading-relaxed text-blue-700 ring-1 ring-blue-200">
              保存后会出现在列表里，再点「上传 PEM」把证书和私钥贴进去。
            </div>
          )}

          <div className="grid grid-cols-2 gap-2">
            <label className="flex cursor-pointer items-center gap-2 rounded-xl bg-slate-50 px-3 py-2 text-xs font-medium text-slate-600">
              <input
                type="checkbox"
                checked={form.enabled}
                onChange={(e) => setForm((p) => ({ ...p, enabled: e.target.checked }))}
                className="h-3.5 w-3.5"
              />
              启用
            </label>
            <label className="flex cursor-pointer items-center gap-2 rounded-xl bg-slate-50 px-3 py-2 text-xs font-medium text-slate-600">
              <input
                type="checkbox"
                checked={form.isDefault}
                onChange={(e) => setForm((p) => ({ ...p, isDefault: e.target.checked }))}
                className="h-3.5 w-3.5"
              />
              设为默认证书
            </label>
          </div>
          <div className="text-[11px] text-slate-400">
            默认证书用于客户端没带 SNI（比如直接用 IP 访问）或没有域名匹配时兜底，全局只能有一张。
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

// ===================== 上传 PEM 弹窗 =====================

function UploadModal({
  cert,
  onCancel,
  onSubmit,
}: {
  cert: Certificate
  onCancel: () => void
  onSubmit: (certPem: string, keyPem: string) => Promise<void>
}) {
  const [certPem, setCertPem] = useState('')
  const [keyPem, setKeyPem] = useState('')
  const [busy, setBusy] = useState(false)
  const [err, setErr] = useState('')

  const submit = async () => {
    if (!certPem.trim() || !keyPem.trim()) {
      setErr('证书和私钥都要填')
      return
    }
    setErr('')
    setBusy(true)
    try {
      await onSubmit(certPem, keyPem)
    } catch (e) {
      setErr(e instanceof Error ? e.message : '上传失败')
      setBusy(false)
    }
  }

  const box =
    'w-full rounded-xl border border-slate-200 bg-white px-3 py-2 font-mono text-[11px] leading-relaxed outline-none focus:border-blue-400'

  return (
    <div
      className="fixed inset-0 z-50 grid place-items-center bg-black/30 px-4 py-6 backdrop-blur-sm"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget) onCancel()
      }}
    >
      <div className="max-h-full w-full max-w-2xl overflow-y-auto rounded-2xl bg-white p-6 text-slate-700 shadow-2xl ring-1 ring-slate-200">
        <div className="mb-5 flex items-center justify-between">
          <div className="text-base font-bold text-slate-800">上传 PEM — {cert.name}</div>
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
            <div className="mb-1 text-xs font-semibold text-slate-500">
              证书链（fullchain）
              <span className="ml-1 font-normal text-slate-400">含中间证书，否则部分客户端会报证书链不完整</span>
            </div>
            <textarea
              rows={8}
              value={certPem}
              onChange={(e) => setCertPem(e.target.value)}
              placeholder="-----BEGIN CERTIFICATE-----"
              className={box}
            />
          </label>
          <label className="block">
            <div className="mb-1 text-xs font-semibold text-slate-500">私钥</div>
            <textarea
              rows={6}
              value={keyPem}
              onChange={(e) => setKeyPem(e.target.value)}
              placeholder="-----BEGIN PRIVATE KEY-----"
              className={box}
            />
          </label>
          <div className="text-[11px] text-slate-400">
            私钥只写到本机 data/cert/ 目录（权限 0600），不会出现在任何接口返回里。
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
            {busy ? '校验中...' : '上传'}
          </button>
        </div>
      </div>
    </div>
  )
}

// ===================== 主页面 =====================

export function Cert() {
  const [certs, setCerts] = useState<Certificate[] | null>(null)
  const [providers, setProviders] = useState<AcmeProvider[]>([])
  const [loadErr, setLoadErr] = useState('')
  const [issuingId, setIssuingId] = useState<number | null>(null)
  const { toasts, show: toast } = useToast()

  const [editModal, setEditModal] = useState<{ open: boolean; initial?: Certificate }>({ open: false })
  const [uploadModal, setUploadModal] = useState<Certificate | null>(null)

  const refresh = useCallback(async () => {
    try {
      const [list, provs] = await Promise.all([api.getCertList(), api.getCertProviders()])
      setCerts(list)
      setProviders(provs)
      setLoadErr('')
    } catch (e) {
      setLoadErr(e instanceof Error ? e.message : String(e))
    }
  }, [])

  useEffect(() => {
    refresh()
    // 签发是异步的，靠轮询把 notAfter / lastError 的变化捞回来
    const t = window.setInterval(refresh, 10000)
    return () => window.clearInterval(t)
  }, [refresh])

  const list = certs ?? []
  const active = list.filter((c) => c.enabled && c.loaded && !c.expired).length
  const expiring = list.filter((c) => c.loaded && !c.expired && c.daysLeft <= 15).length
  const problem = list.filter((c) => c.enabled && (c.expired || !!c.lastError)).length

  const submitCert = async (payload: api.CertPayload) => {
    if (editModal.initial) {
      await api.updateCert({ ...payload, id: editModal.initial.id })
      toast('证书已更新')
    } else {
      await api.addCert(payload)
      toast(payload.source === 'upload' ? '已创建，接着上传 PEM' : '证书已添加')
    }
    setEditModal({ open: false })
    await refresh()
  }

  const submitUpload = async (certPem: string, keyPem: string) => {
    if (!uploadModal) return
    await api.uploadCertPEM(uploadModal.id, certPem, keyPem)
    setUploadModal(null)
    toast('证书已上传并生效')
    await refresh()
  }

  const removeCert = async (c: Certificate) => {
    if (!window.confirm(`确认删除证书「${c.name}」？\n本机保存的 PEM 也会一并清除。`)) return
    try {
      await api.deleteCert(c.id)
      toast('证书已删除')
      await refresh()
    } catch (e) {
      toast('删除失败: ' + (e instanceof Error ? e.message : ''))
    }
  }

  const issue = async (c: Certificate) => {
    setIssuingId(c.id)
    try {
      await api.issueCert(c.id)
      // 自签是本机算的，毫秒级就好了；ACME 要跟 CA 来回跑，说个时间免得用户以为卡住
      toast(c.source === 'self-signed' ? '已重新生成' : '已开始签发，全程约需 1–3 分钟')
      await refresh()
    } catch (e) {
      toast('签发失败: ' + (e instanceof Error ? e.message : ''))
    } finally {
      setIssuingId(null)
    }
  }

  if (loadErr && !certs) {
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

  if (!certs) {
    return (
      <Card>
        <div className="py-12 text-center text-sm text-slate-400">正在加载...</div>
      </Card>
    )
  }

  return (
    <div className="space-y-4">
      <div className="flex items-center justify-between">
        <div className="text-base font-bold text-slate-800">证书管理</div>
      </div>

      {/* 这页存在的意义，一句话说清楚 */}
      <Card>
        <div className="flex gap-3 text-xs leading-relaxed text-slate-500">
          <ShieldCheck className="mt-0.5 h-4 w-4 shrink-0 text-blue-500" />
          <div>
            这里的证书用于在 <span className="font-semibold text-slate-700">STUN 洞口终结 TLS</span>
            ：开启后外部访问是 <span className="font-mono">https://</span>，而不是明文 HTTP。
            在 STUN 页面为每个服务填好对外域名并打开「洞口终结 TLS」即可使用，
            证书续期时只热替换内存中的指针，<span className="font-semibold text-slate-700">不会断开已有连接，也不需要重启穿透</span>。
          </div>
        </div>
      </Card>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 lg:grid-cols-4">
        <Card>
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-blue-50 text-blue-500">
              <ShieldCheck className="h-5 w-5" />
            </div>
            <div>
              <div className="text-xs text-slate-500">证书总数</div>
              <div className="text-2xl font-bold text-slate-800">{list.length}</div>
            </div>
          </div>
        </Card>
        <Card>
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-emerald-50 text-emerald-500">
              <CircleCheck className="h-5 w-5" />
            </div>
            <div>
              <div className="text-xs text-slate-500">生效中</div>
              <div className="text-2xl font-bold text-slate-800">{active}</div>
            </div>
          </div>
        </Card>
        <Card>
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-amber-50 text-amber-500">
              <Clock className="h-5 w-5" />
            </div>
            <div>
              <div className="text-xs text-slate-500">15 天内到期</div>
              <div className="text-2xl font-bold text-slate-800">{expiring}</div>
            </div>
          </div>
        </Card>
        <Card>
          <div className="flex items-center gap-3">
            <div className="grid h-10 w-10 place-items-center rounded-xl bg-rose-50 text-rose-500">
              <TriangleAlert className="h-5 w-5" />
            </div>
            <div>
              <div className="text-xs text-slate-500">过期 / 异常</div>
              <div className="text-2xl font-bold text-slate-800">{problem}</div>
            </div>
          </div>
        </Card>
      </div>

      <Card className="!p-0">
        <div className="px-5 pt-4">
          <CardHeader
            title="证书列表"
            action={
              <button
                onClick={() => setEditModal({ open: true })}
                className="flex items-center gap-1 rounded-md bg-blue-500 px-2.5 py-1 text-xs font-semibold text-white shadow-sm shadow-blue-500/20 hover:bg-blue-600"
              >
                <Plus className="h-3 w-3" /> 添加证书
              </button>
            }
          />
        </div>

        {/* 手机上列太窄会把中文压成一列一个字，给张表一个下限，窄屏改成横向滚动 */}
        <div className="overflow-x-auto">
          <table className="w-full min-w-[820px] text-sm">
            <thead>
              <tr className="bg-slate-50/60 text-xs text-slate-500">
                <th className="px-5 py-2.5 text-left font-medium">名称</th>
                <th className="px-3 py-2.5 text-left font-medium">来源</th>
                <th className="px-3 py-2.5 text-left font-medium">覆盖域名</th>
                <th className="px-3 py-2.5 text-left font-medium">状态</th>
                <th className="px-3 py-2.5 text-left font-medium">有效期至</th>
                <th className="px-3 py-2.5 text-left font-medium">签发者</th>
                <th className="px-5 py-2.5 text-right font-medium">操作</th>
              </tr>
            </thead>
            <tbody className="divide-y divide-slate-100">
              {list.length === 0 && (
                <tr>
                  <td colSpan={7} className="px-5 py-8 text-center text-sm text-slate-400">
                    还没有证书，点右上角「添加证书」开始
                  </td>
                </tr>
              )}
              {list.map((c) => {
                const s = certStatus(c)
                const Icon = s.icon
                const busy = issuingId === c.id
                // 自签也能点这个按钮：换了网段 / DHCP 换了地址之后，
                // 得重签一张把新 IP 写进证书里
                const selfSigned = c.source === 'self-signed'
                const canReissue = selfSigned || c.source === 'acme-dns01' || c.source === 'acme-http01'
                return (
                  <tr key={c.id} className="align-top text-slate-700 transition hover:bg-slate-50/60">
                    <td className="px-5 py-3">
                      <div className="flex items-center gap-1.5 font-semibold">
                        {c.name}
                        {c.isDefault && (
                          <span className="rounded bg-violet-50 px-1.5 py-0.5 text-[10px] font-bold text-violet-600">
                            默认
                          </span>
                        )}
                      </div>
                      {c.lastError && (
                        <div className="mt-0.5 max-w-xs text-[11px] leading-relaxed text-rose-500" title={c.lastError}>
                          {c.lastError}
                        </div>
                      )}
                    </td>
                    <td className="px-3 py-3 text-xs text-slate-500">{sourceLabel(c.source)}</td>
                    <td className="px-3 py-3">
                      <div className="flex max-w-xs flex-wrap gap-1">
                        {c.domains.length === 0 && <span className="text-xs text-slate-400">--</span>}
                        {c.domains.map((d) => (
                          <span key={d} className="rounded bg-slate-100 px-1.5 py-0.5 font-mono text-[11px] text-slate-600">
                            {d}
                          </span>
                        ))}
                      </div>
                    </td>
                    <td className="px-3 py-3">
                      <span className={`inline-flex items-center gap-1 rounded-md px-1.5 py-0.5 text-[11px] font-semibold ${s.tone}`}>
                        <Icon className={`h-3 w-3 ${s.label === '等待签发' ? 'animate-spin' : ''}`} />
                        {s.label}
                      </span>
                    </td>
                    <td className="px-3 py-3 text-xs">
                      <div className="text-slate-600">{fmtDate(c.notAfter)}</div>
                      {c.notAfter && !c.expired && (
                        <div className={c.daysLeft <= 15 ? 'text-amber-600' : 'text-slate-400'}>剩 {c.daysLeft} 天</div>
                      )}
                    </td>
                    {/* 自签证书的签发者就是它自己，直接显示会是「nas.home.lan」，
                        看着像某个叫这名字的 CA。说清楚是谁签的更有用 */}
                    <td className="px-3 py-3 text-xs text-slate-500">
                      {selfSigned ? 'LinkStar 自签' : c.issuer || '--'}
                    </td>
                    <td className="px-5 py-3 text-right">
                      <div className="inline-flex items-center gap-1">
                        {canReissue && (
                          <button
                            onClick={() => issue(c)}
                            disabled={busy || !c.enabled}
                            className="grid h-6 w-6 place-items-center rounded-md text-slate-400 hover:bg-blue-50 hover:text-blue-600 disabled:opacity-40"
                            title={
                              !c.enabled
                                ? '证书已禁用'
                                : selfSigned
                                  ? '重新生成（换了网段、IP 变了之后点一下，把新 IP 写进证书）'
                                  : '立即签发 / 续期'
                            }
                          >
                            <RefreshCw className={`h-3 w-3 ${busy ? 'animate-spin' : ''}`} />
                          </button>
                        )}
                        {c.source === 'upload' && (
                          <button
                            onClick={() => setUploadModal(c)}
                            className="grid h-6 w-6 place-items-center rounded-md text-slate-400 hover:bg-blue-50 hover:text-blue-600"
                            title="上传 PEM"
                          >
                            <Upload className="h-3 w-3" />
                          </button>
                        )}
                        <button
                          onClick={() => setEditModal({ open: true, initial: c })}
                          className="grid h-6 w-6 place-items-center rounded-md text-slate-400 hover:bg-emerald-50 hover:text-emerald-600"
                          title="编辑"
                        >
                          <Pencil className="h-3 w-3" />
                        </button>
                        <button
                          onClick={() => removeCert(c)}
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

        <div className="border-t border-slate-100 px-5 py-3 text-xs text-slate-400">共 {list.length} 张证书</div>
      </Card>

      {editModal.open && (
        <CertModal
          initial={editModal.initial}
          providers={providers}
          onCancel={() => setEditModal({ open: false })}
          onSubmit={submitCert}
        />
      )}
      {uploadModal && (
        <UploadModal cert={uploadModal} onCancel={() => setUploadModal(null)} onSubmit={submitUpload} />
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
