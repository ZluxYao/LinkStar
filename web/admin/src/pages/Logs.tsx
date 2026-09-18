import { useCallback, useEffect, useState } from 'react'
import { RotateCw, Search } from 'lucide-react'
import { Card, CardHeader } from '../components/Card'
import * as api from '../lib/api'
import type { LogEntry } from '../types'

const levelStyle: Record<string, string> = {
  error: 'bg-rose-50 text-rose-600',
  fatal: 'bg-rose-50 text-rose-600',
  panic: 'bg-rose-50 text-rose-600',
  warning: 'bg-amber-50 text-amber-600',
  warn: 'bg-amber-50 text-amber-600',
  info: 'bg-blue-50 text-blue-600',
  debug: 'bg-slate-100 text-slate-500',
  trace: 'bg-slate-100 text-slate-500',
}

const levelFilters = [
  { value: '', label: '全部' },
  { value: 'error', label: '错误' },
  { value: 'warning', label: '警告' },
  { value: 'info', label: '信息' },
  { value: 'debug', label: '调试' },
]

export function Logs() {
  const [days, setDays] = useState<string[]>([])
  const [day, setDay] = useState('')
  const [level, setLevel] = useState('')
  const [keyword, setKeyword] = useState('')
  // 输入框和真正发出去的关键字分开，不然每敲一个字都打一次接口
  const [search, setSearch] = useState('')
  const [rows, setRows] = useState<LogEntry[]>([])
  const [truncated, setTruncated] = useState(false)
  const [loading, setLoading] = useState(false)
  const [err, setErr] = useState('')

  useEffect(() => {
    api
      .getLogDays()
      .then((d) => {
        setDays(d)
        setDay((cur) => cur || d[0] || '')
      })
      .catch((e) => setErr(e instanceof Error ? e.message : String(e)))
  }, [])

  const refresh = useCallback(async () => {
    if (!day) return
    setLoading(true)
    try {
      const r = await api.getLogs({ day, level, keyword: search, limit: 500 })
      setRows(r.list || [])
      setTruncated(r.truncated)
      setErr('')
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e))
      setRows([])
    } finally {
      setLoading(false)
    }
  }, [day, level, search])

  useEffect(() => {
    refresh()
  }, [refresh])

  return (
    <div className="space-y-4">
      <Card>
        <CardHeader
          title="运行日志"
          action={
            <button
              onClick={refresh}
              disabled={loading}
              className="flex items-center gap-1 rounded-lg border border-slate-200 px-2.5 py-1 text-xs font-semibold text-slate-600 transition hover:bg-slate-50 disabled:opacity-50"
            >
              <RotateCw className={`h-3.5 w-3.5 ${loading ? 'animate-spin' : ''}`} /> 刷新
            </button>
          }
        />

        <div className="flex flex-wrap items-center gap-2">
          <select
            value={day}
            onChange={(e) => setDay(e.target.value)}
            className="h-9 rounded-lg border border-slate-200 bg-white px-2.5 text-sm text-slate-700 outline-none focus:border-blue-400"
          >
            {days.length === 0 && <option value="">暂无日志</option>}
            {days.map((d) => (
              <option key={d} value={d}>
                {d}
              </option>
            ))}
          </select>

          <div className="flex overflow-hidden rounded-lg border border-slate-200">
            {levelFilters.map((f) => (
              <button
                key={f.value}
                onClick={() => setLevel(f.value)}
                className={`px-3 py-1.5 text-xs font-semibold transition ${
                  level === f.value ? 'bg-blue-500 text-white' : 'bg-white text-slate-600 hover:bg-slate-50'
                }`}
              >
                {f.label}
              </button>
            ))}
          </div>

          <div className="relative min-w-[14rem] flex-1">
            <Search className="pointer-events-none absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-slate-400" />
            <input
              value={keyword}
              onChange={(e) => setKeyword(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === 'Enter') setSearch(keyword.trim())
              }}
              onBlur={() => setSearch(keyword.trim())}
              placeholder="搜索日志内容，回车生效"
              className="h-9 w-full rounded-lg border border-slate-200 bg-white pl-9 pr-3 text-sm text-slate-600 outline-none transition focus:border-blue-400"
            />
          </div>
        </div>

        {err && (
          <div className="mt-3 rounded-lg bg-rose-50 px-3 py-2 text-xs text-rose-600">{err}</div>
        )}
      </Card>

      <Card className="p-0">
        <div className="flex items-center justify-between px-5 py-3 text-xs text-slate-400">
          <span>最新 {rows.length} 条在最上面</span>
          {/* 只从文件尾部读了一段，别让用户以为一整天就这些 */}
          {truncated && <span>日志文件较大，只读了最近的一段</span>}
        </div>

        {rows.length === 0 ? (
          <div className="px-5 pb-6 text-sm text-slate-400">{loading ? '读取中…' : '没有匹配的日志'}</div>
        ) : (
          <ul className="divide-y divide-slate-100 border-t border-slate-100">
            {rows.map((r, i) => (
              <li key={`${r.time}-${i}`} className="flex gap-3 px-5 py-2 hover:bg-slate-50/70">
                <span className="shrink-0 pt-0.5 font-mono text-[11px] text-slate-400">{r.time || '—'}</span>
                <span
                  className={`h-fit shrink-0 rounded px-1.5 py-0.5 text-[10px] font-bold uppercase ${
                    levelStyle[r.level] || 'bg-slate-100 text-slate-500'
                  }`}
                >
                  {r.level || 'log'}
                </span>
                <span className="min-w-0 flex-1">
                  <span className="block whitespace-pre-wrap break-all text-xs text-slate-700">{r.message}</span>
                  {r.caller && (
                    <span className="mt-0.5 block break-all font-mono text-[10px] text-slate-400">{r.caller}</span>
                  )}
                </span>
              </li>
            ))}
          </ul>
        )}
      </Card>
    </div>
  )
}
