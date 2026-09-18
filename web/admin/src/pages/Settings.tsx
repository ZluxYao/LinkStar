import { useState, type FormEvent } from 'react'
import { KeyRound } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import { MIN_PASSWORD_LENGTH, changePassword, setToken } from '../lib/api'

const fieldCls =
  'w-full rounded-xl border border-slate-200 bg-white px-3 py-2 text-sm outline-none transition focus:border-blue-400 disabled:bg-slate-50 disabled:text-slate-400'
const labelCls = 'mb-1 text-xs font-semibold text-slate-500'

export function Settings() {
  return (
    <div className="space-y-4">
      <ChangePassword />
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
