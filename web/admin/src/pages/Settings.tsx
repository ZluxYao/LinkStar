import { useCallback, useEffect, useState, type FormEvent } from 'react'
import { KeyRound, Network, Save } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import {
  MIN_PASSWORD_LENGTH,
  changePassword,
  getStunNetwork,
  setToken,
  updateStunNetwork,
} from '../lib/api'
import type { NetworkMode, StunNetwork } from '../types'

const fieldCls =
  'w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none transition focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400'
const labelCls = 'mb-1 text-xs font-semibold text-slate-500'

export function Settings() {
  return (
    <div className="space-y-4">
      <ChangePassword />
      <NetworkSettings />
    </div>
  )
}

function ChangePassword() {
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState('')
  const [done, setDone] = useState('')

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setDone('')
    if (!oldPassword) {
      setErr('请输入当前密码')
      return
    }
    if (newPassword.length < MIN_PASSWORD_LENGTH) {
      setErr(`新密码至少 ${MIN_PASSWORD_LENGTH} 位`)
      return
    }
    if (newPassword === oldPassword) {
      setErr('新密码和当前密码一样')
      return
    }
    if (newPassword !== confirm) {
      setErr('两次输入的新密码不一致')
      return
    }
    setLoading(true)
    setErr('')
    try {
      // 后端改完密码会换掉签名密钥，旧 token 立刻作废，
      // 顺手回一个新的——不存下来这一页下一个请求就变成未登录了
      const { token } = await changePassword(oldPassword, newPassword)
      setToken(token)
      setOldPassword('')
      setNewPassword('')
      setConfirm('')
      setDone('密码已改好。其他设备上原来登着的都掉线了，要用新密码重新登录')
    } catch (e) {
      setErr(e instanceof Error ? e.message : '修改失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <Card>
      <CardHeader title="修改密码" />

      <div className="mb-4 rounded-xl bg-slate-50 px-3 py-2.5 text-[11px] leading-relaxed text-slate-500 ring-1 ring-slate-200/70">
        LinkStar 只有一个管理员，没有用户名，登录就是这一串密码，所以它得够长。
        <br />
        连续错 13 次会锁 3 分钟，挡得住别人一直试；但这个锁不分来源，别人也能拿它把你自己关在外面，
        真正靠得住的还是密码本身。
      </div>

      <form onSubmit={submit} className="max-w-sm space-y-3">
        <label className="block">
          <div className={labelCls}>当前密码</div>
          <input
            type="password"
            autoComplete="current-password"
            value={oldPassword}
            onChange={(e) => setOldPassword(e.target.value)}
            className={fieldCls}
          />
        </label>
        <label className="block">
          <div className={labelCls}>新密码</div>
          <input
            type="password"
            autoComplete="new-password"
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
            placeholder={`至少 ${MIN_PASSWORD_LENGTH} 位`}
            className={fieldCls}
          />
        </label>
        <label className="block">
          <div className={labelCls}>确认新密码</div>
          <input
            type="password"
            autoComplete="new-password"
            value={confirm}
            onChange={(e) => setConfirm(e.target.value)}
            className={fieldCls}
          />
        </label>

        {err && <div className="rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>}
        {done && (
          <div className="rounded-lg bg-emerald-50 px-3 py-2 text-xs text-emerald-600">{done}</div>
        )}

        <button
          type="submit"
          disabled={loading}
          className="flex items-center gap-1.5 rounded-xl bg-blue-500 px-4 py-2 text-sm font-bold text-white transition hover:bg-blue-600 disabled:cursor-not-allowed disabled:opacity-60"
        >
          <KeyRound className="h-4 w-4" />
          {loading ? '处理中…' : '保存新密码'}
        </button>
      </form>
    </Card>
  )
}

// ModeSwitch 三选一的分段按钮
function ModeSwitch({
  value,
  options,
  onChange,
}: {
  value: NetworkMode
  options: { value: NetworkMode; label: string }[]
  onChange: (v: NetworkMode) => void
}) {
  return (
    <div className="inline-flex flex-wrap gap-1 rounded-xl bg-slate-100 p-1">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          onClick={() => onChange(o.value)}
          className={`rounded-lg px-3 py-1.5 text-xs font-semibold transition ${
            value === o.value ? 'bg-white text-blue-600 shadow-sm' : 'text-slate-500 hover:text-slate-700'
          }`}
        >
          {o.label}
        </button>
      ))}
    </div>
  )
}

function NetworkSettings() {
  const [net, setNet] = useState<StunNetwork | null>(null)
  const [ifaceMode, setIfaceMode] = useState<NetworkMode>('')
  const [iface, setIface] = useState('')
  const [dnsMode, setDnsMode] = useState<NetworkMode>('')
  const [dns, setDns] = useState('')
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState('')
  const [done, setDone] = useState('')

  const apply = useCallback((n: StunNetwork) => {
    setNet(n)
    setIfaceMode(n.config.ifaceMode)
    setIface(n.config.iface)
    setDnsMode(n.config.dnsMode)
    setDns(n.config.dns.join('\n'))
  }, [])

  useEffect(() => {
    getStunNetwork()
      .then(apply)
      .catch((e) => setErr(e instanceof Error ? e.message : '读取网络设置失败'))
  }, [apply])

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    setErr('')
    setDone('')
    if (ifaceMode === 'custom' && !iface) {
      setErr('选一张网卡')
      return
    }
    const dnsList = dns
      .split(/[\s,，]+/)
      .map((s) => s.trim())
      .filter(Boolean)
    if (dnsMode === 'custom' && dnsList.length === 0) {
      setErr('至少填一个 DNS 地址')
      return
    }
    setLoading(true)
    try {
      const { msg } = await updateStunNetwork({ ifaceMode, iface, dnsMode, dns: dnsList })
      setDone(msg)
      apply(await getStunNetwork())
    } catch (e) {
      setErr(e instanceof Error ? e.message : '保存失败')
    } finally {
      setLoading(false)
    }
  }

  if (!net) {
    return (
      <Card>
        <CardHeader title="网络" />
        <div className="py-6 text-center text-sm text-slate-400">{err || '正在加载…'}</div>
      </Card>
    )
  }

  const autoIface = net.ifaces.find((i) => i.auto)
  const pickedIface = net.ifaces.find((i) => i.name === iface)

  return (
    <Card>
      <CardHeader title="网络" />

      <div className="mb-4 rounded-xl bg-slate-50 px-3 py-2.5 text-[11px] leading-relaxed text-slate-500 ring-1 ring-slate-200/70">
        只管打洞这一块：查公网 IP、STUN、UPnP、NAT 检测。DDNS、证书、Webhook 照常走系统网络。
        <br />
        开着 Clash / sing-box 的 TUN 时，不指定出口的连接会被代理接走，查到的就是代理节点的 IP。保持「自动」就能绕开。
      </div>

      <form onSubmit={submit} className="max-w-xl space-y-5">
        {/* 出口网卡 */}
        <div>
          <div className={labelCls}>出口网卡</div>
          <ModeSwitch
            value={ifaceMode}
            onChange={setIfaceMode}
            options={[
              { value: '', label: '自动' },
              { value: 'system', label: '跟随系统' },
              { value: 'custom', label: '指定网卡' },
            ]}
          />
          <div className="mt-2 text-[11px] leading-relaxed text-slate-500">
            {ifaceMode === '' &&
              (autoIface ? (
                <>
                  挑关掉代理后系统本来会走的那张物理网卡，换了网络自动跟着换。现在会选{' '}
                  <span className="font-mono text-slate-700">{autoIface.name}</span>（{autoIface.localIP}）。
                </>
              ) : (
                <span className="text-rose-500">挑关掉代理后系统本来会走的那张物理网卡。现在一张能用的都没有。</span>
              ))}
            {ifaceMode === 'system' && (
              <span className="text-amber-600">
                不绑网卡，按系统路由走。开着代理 TUN 时打洞会从代理出去，公网 IP 也会变成代理节点的。
              </span>
            )}
            {ifaceMode === 'custom' && '两条宽带时用来选线路。这张卡断了打洞会停下来等它，不会自己换到别的卡。'}
          </div>

          {ifaceMode === 'custom' && (
            <div className="mt-2 space-y-1.5">
              {net.ifaces.map((i) => (
                <label
                  key={i.name}
                  className={`flex cursor-pointer flex-wrap items-center gap-x-3 gap-y-1 rounded-xl border px-3 py-2 text-xs transition ${
                    iface === i.name ? 'border-blue-300 bg-blue-50/60' : 'border-slate-200 hover:border-slate-300'
                  }`}
                >
                  <input
                    type="radio"
                    name="iface"
                    checked={iface === i.name}
                    onChange={() => setIface(i.name)}
                    className="accent-blue-500"
                  />
                  <span className="font-semibold text-slate-700">{i.name}</span>
                  <span className="font-mono text-slate-500">{i.localIP}</span>
                  {i.gateway && <span className="text-slate-400">网关 {i.gateway}</span>}
                  {i.auto && <span className="rounded bg-emerald-50 px-1.5 py-0.5 text-emerald-600">自动会选</span>}
                  {i.tunnel && <span className="rounded bg-amber-50 px-1.5 py-0.5 text-amber-600">隧道</span>}
                </label>
              ))}
              {/* 存的网卡现在不在列表里（拔了 / 没连上），也得让人看见选的是哪张 */}
              {iface && !pickedIface && (
                <div className="rounded-xl border border-rose-200 bg-rose-50 px-3 py-2 text-xs text-rose-600">
                  之前选的 {iface} 现在没连上
                </div>
              )}
            </div>
          )}
        </div>

        {/* DNS */}
        <div>
          <div className={labelCls}>解析 STUN 服务器用的 DNS</div>
          <ModeSwitch
            value={dnsMode}
            onChange={setDnsMode}
            options={[
              { value: '', label: '内置' },
              { value: 'system', label: '跟随系统' },
              { value: 'custom', label: '自定义' },
            ]}
          />
          <div className="mt-2 text-[11px] leading-relaxed text-slate-500">
            {dnsMode === '' && (
              <>
                从出口网卡直接问 <span className="font-mono text-slate-700">{net.defaultDns.join('、')}</span>
                ，绕开代理的 fake-ip。
              </>
            )}
            {dnsMode === 'system' && (
              <span className="text-amber-600">代理开着 fake-ip 时，STUN 服务器会被解析成 198.18.x.x，从物理网卡连不上。</span>
            )}
            {dnsMode === 'custom' && '一行一个，只认 IPv4，可以带端口，比如路由器的 192.168.1.1 或 192.168.1.1:5353。'}
          </div>
          {dnsMode === 'custom' && (
            <textarea
              value={dns}
              onChange={(e) => setDns(e.target.value)}
              rows={3}
              placeholder={net.defaultDns.join('\n')}
              className={`${fieldCls} mt-2 font-mono`}
            />
          )}
        </div>

        <div className="flex flex-wrap items-center gap-x-1.5 text-[11px] text-slate-500">
          <Network className="h-3.5 w-3.5 text-slate-400" />
          现在从
          <span className="font-mono text-slate-700">
            {net.current.localIP ? `${net.current.name || '系统路由'}（${net.current.localIP}）` : '——'}
          </span>
          出去
        </div>

        {err && <div className="rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>}
        {done && <div className="rounded-lg bg-emerald-50 px-3 py-2 text-xs text-emerald-600">{done}</div>}

        <div className="flex flex-wrap items-center gap-3">
          <button
            type="submit"
            disabled={loading}
            className="flex items-center gap-1.5 rounded-xl bg-blue-500 px-4 py-2 text-sm font-bold text-white transition hover:bg-blue-600 disabled:cursor-not-allowed disabled:opacity-60"
          >
            <Save className="h-4 w-4" />
            {loading ? '保存中…' : '保存'}
          </button>
          <span className="text-[11px] text-slate-400">保存后立刻生效，出口变了会把所有打洞服务重启一遍。</span>
        </div>
      </form>
    </Card>
  )
}
