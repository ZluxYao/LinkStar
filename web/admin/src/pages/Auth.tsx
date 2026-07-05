import { useState, type FormEvent, type ReactNode } from 'react'
import { LockKeyhole, ShieldCheck } from 'lucide-react'
import { login, setupPassword, setToken } from '../lib/api'

export function Login({ onSuccess }: { onSuccess: () => void }) {
  const [password, setPassword] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (!password) {
      setError('请输入密码')
      return
    }
    setLoading(true)
    setError('')
    try {
      const { token } = await login(password)
      setToken(token)
      onSuccess()
    } catch (err) {
      setError(err instanceof Error ? err.message : '登录失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthLayout
      icon={<LockKeyhole className="h-7 w-7" />}
      title="登录 LinkStar"
      subtitle="请输入管理员密码"
    >
      <form onSubmit={submit} className="space-y-4">
        <PasswordField value={password} onChange={setPassword} placeholder="管理员密码" autoFocus />
        {error && <p className="text-sm text-rose-500">{error}</p>}
        <SubmitButton loading={loading}>登录</SubmitButton>
      </form>
    </AuthLayout>
  )
}

export function Setup({ onSuccess }: { onSuccess: () => void }) {
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [error, setError] = useState('')
  const [loading, setLoading] = useState(false)

  const submit = async (e: FormEvent) => {
    e.preventDefault()
    if (password.length < 6) {
      setError('密码至少 6 位')
      return
    }
    if (password !== confirm) {
      setError('两次输入的密码不一致')
      return
    }
    setLoading(true)
    setError('')
    try {
      const { token } = await setupPassword(password)
      setToken(token)
      onSuccess()
    } catch (err) {
      setError(err instanceof Error ? err.message : '设置失败')
    } finally {
      setLoading(false)
    }
  }

  return (
    <AuthLayout
      icon={<ShieldCheck className="h-7 w-7" />}
      title="欢迎使用 LinkStar"
      subtitle="首次使用，请设置管理员密码"
    >
      <form onSubmit={submit} className="space-y-4">
        <PasswordField value={password} onChange={setPassword} placeholder="设置密码（至少 6 位）" autoFocus />
        <PasswordField value={confirm} onChange={setConfirm} placeholder="确认密码" />
        {error && <p className="text-sm text-rose-500">{error}</p>}
        <SubmitButton loading={loading}>设置并进入</SubmitButton>
      </form>
    </AuthLayout>
  )
}

function AuthLayout({
  icon,
  title,
  subtitle,
  children,
}: {
  icon: ReactNode
  title: string
  subtitle: string
  children: ReactNode
}) {
  return (
    <div className="grid min-h-screen place-items-center bg-slate-50 px-4">
      <div className="w-full max-w-sm rounded-2xl border border-slate-200/70 bg-white p-8 shadow-sm shadow-slate-200/60">
        <div className="mb-6 flex flex-col items-center text-center">
          <div className="grid h-14 w-14 place-items-center rounded-2xl bg-indigo-50 text-indigo-500">
            {icon}
          </div>
          <h1 className="mt-4 text-lg font-bold text-slate-800">{title}</h1>
          <p className="mt-1 text-sm text-slate-400">{subtitle}</p>
        </div>
        {children}
      </div>
    </div>
  )
}

function PasswordField({
  value,
  onChange,
  placeholder,
  autoFocus,
}: {
  value: string
  onChange: (v: string) => void
  placeholder: string
  autoFocus?: boolean
}) {
  return (
    <input
      type="password"
      value={value}
      autoFocus={autoFocus}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      className="w-full rounded-xl border border-slate-200 bg-slate-50 px-4 py-2.5 text-sm text-slate-800 outline-none transition focus:border-indigo-400 focus:bg-white focus:ring-2 focus:ring-indigo-100"
    />
  )
}

function SubmitButton({ loading, children }: { loading: boolean; children: ReactNode }) {
  return (
    <button
      type="submit"
      disabled={loading}
      className="w-full rounded-xl bg-indigo-500 px-4 py-2.5 text-sm font-bold text-white transition hover:bg-indigo-600 disabled:cursor-not-allowed disabled:opacity-60"
    >
      {loading ? '处理中…' : children}
    </button>
  )
}
